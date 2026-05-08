package dynamodb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dberrors "golang.nuinfra.api-server/pkg/db"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
	awstesting "golang.nuinfra.net/commons/pkg/aws/testing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sandboxesSchemaPath = "../../../../../dynamodb-tables/sandboxes.json"

// sharedDynamoDBEndpoint is the http endpoint of the package-shared
// DynamoDB Local container started by TestMain. Tests use it via
// setupDB; per-test isolation comes from CreateTable's random table
// suffix rather than a fresh container per call.
var sharedDynamoDBEndpoint string

func TestMain(m *testing.M) {
	endpoint, cleanup, err := awstesting.StartSharedDynamoDB(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: start dynamodb:", err)
		os.Exit(1)
	}
	sharedDynamoDBEndpoint = endpoint

	code := m.Run()

	if err := cleanup(); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: terminate dynamodb:", err)
	}
	os.Exit(code)
}

func setupDB(t *testing.T) (*dynamoDB, context.Context) {
	t.Helper()

	tableName := awstesting.CreateTable(t, sharedDynamoDBEndpoint, sandboxesSchemaPath)

	ctx := t.Context()
	awsCfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, awsCfg)
	client := dynamodb.New(ctx, dynamodb.Config{Endpoint: sharedDynamoDBEndpoint})

	return &dynamoDB{client: client, tableName: tableName}, ctx
}

func newFixture(opts ...func(*cpv1.Sandbox)) *cpv1.Sandbox {
	now := time.Date(2026, 4, 27, 10, 30, 45, 123456789, time.UTC)
	// Version is zero by default — the DB stamps initialVersion in Create.
	sb := &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{
			Id:             "sb-001",
			Namespace:      "team-alpha",
			CreatedAt:      timestamppb.New(now),
			LastModifiedAt: timestamppb.New(now),
			Labels:         map[string]string{"app": "demo", "env": "test"},
		},
		Resources: &cpv1.Resources{
			VcpuCount: 2,
			MemoryMib: 1024,
		},
		Intent: &cpv1.Intent{
			Phase: cpv1.SandboxStatus_PHASE_RUNNING,
		},
		Status: &cpv1.SandboxStatus{
			Phase: cpv1.SandboxStatus_PHASE_PENDING,
		},
	}
	for _, opt := range opts {
		opt(sb)
	}
	return sb
}

func withID(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.Id = id }
}

func withNode(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Node = &cpv1.NodeRef{Id: id} }
}

func withVCPU(n uint32) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Resources.VcpuCount = n }
}

func withLastModified(t time.Time) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.LastModifiedAt = timestamppb.New(t) }
}

func withVersion(v int64) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.Version = v }
}

func withSavedPhase(p cpv1.SandboxStatus_Phase) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Status.Phase = p }
}

func withIntentPhase(p cpv1.SandboxStatus_Phase) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Intent.Phase = p }
}

func getItem(ctx context.Context, t *testing.T, db *dynamoDB, id string) map[string]types.AttributeValue {
	t.Helper()
	out, err := db.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(db.tableName),
		Key:            map[string]types.AttributeValue{attrSandboxID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Item, "expected sandbox %q to be present", id)
	return out.Item
}

func attrS(t *testing.T, item map[string]types.AttributeValue, name string) string {
	t.Helper()
	av, ok := item[name]
	require.True(t, ok, "attribute %q is missing", name)
	s, ok := av.(*types.AttributeValueMemberS)
	require.True(t, ok, "attribute %q is not a string", name)
	return s.Value
}

// attrVersion reads the numeric sandbox_version attribute. It is the only
// numeric column on the sandboxes table, so a dedicated reader is clearer
// than a generic attrN helper at the call sites.
func attrVersion(t *testing.T, item map[string]types.AttributeValue) string {
	t.Helper()
	av, ok := item[attrSandboxVersion]
	require.True(t, ok, "attribute %q is missing", attrSandboxVersion)
	n, ok := av.(*types.AttributeValueMemberN)
	require.True(t, ok, "attribute %q is not a number", attrSandboxVersion)
	return n.Value
}

func attrB(t *testing.T, item map[string]types.AttributeValue, name string) []byte {
	t.Helper()
	av, ok := item[name]
	require.True(t, ok, "attribute %q is missing", name)
	b, ok := av.(*types.AttributeValueMemberB)
	require.True(t, ok, "attribute %q is not bytes", name)
	return b.Value
}

// assertGSISortKey asserts the shared GSI sort key has the canonical
// `<rfc3339nano-timestamp>#<id>` shape and that the embedded timestamp is
// within a few seconds of now. The DB layer owns LastModifiedAt, so tests
// cannot assert against a literal caller-supplied value.
func assertGSISortKey(t *testing.T, item map[string]types.AttributeValue, id string) time.Time {
	t.Helper()
	sk := attrS(t, item, attrGSILmtSK)
	suffix := "#" + id
	require.True(t, strings.HasSuffix(sk, suffix),
		"GSI sort key %q must end with %q", sk, suffix)
	tsPart := strings.TrimSuffix(sk, suffix)
	ts, err := time.Parse(time.RFC3339Nano, tsPart)
	require.NoError(t, err, "GSI timestamp segment %q must parse as RFC3339Nano", tsPart)
	assert.WithinDuration(t, time.Now(), ts, 5*time.Second,
		"GSI timestamp segment must be a recent server-stamped time")
	return ts
}

func TestCreate_HappyPath(t *testing.T) {
	db, ctx := setupDB(t)
	sb := newFixture()

	require.NoError(t, db.Create(ctx, sb))

	item := getItem(ctx, t, db, sb.Metadata.Id)

	assert.Equal(t, "sb-001", attrS(t, item, attrSandboxID))
	assert.Equal(t, "team-alpha", attrS(t, item, attrSandboxNamespace))
	assert.Equal(t, "PHASE_PENDING", attrS(t, item, attrSandboxStatusPhase))
	assert.Equal(t, "1", attrVersion(t, item))
	// The DB stamps LastModifiedAt itself, so the GSI sort key is built
	// from a server-recent timestamp; assert the shape and freshness.
	assertGSISortKey(t, item, "sb-001")

	assert.Equal(t, "team-alpha#PHASE_PENDING", attrS(t, item, attrGSINamespacePhaseHK),
		"gsi_namespace_phase_hk must be {namespace}#{phase}")

	assert.NotContains(t, item, attrNodeRefID, "node_ref_id must be absent when Node is unset (sparse GSI)")
	assert.NotContains(t, item, attrGSINamespaceNodeHK,
		"gsi_namespace_node_hk must be absent when Node is unset (sparse GSI)")
	assert.NotContains(t, item, attrGSINodePhaseHK,
		"gsi_node_phase_hk must be absent when Node is unset (sparse GSI)")
	assert.NotContains(t, item, attrGSINamespaceNodePhaseHK,
		"gsi_namespace_node_phase_hk must be absent when Node is unset (sparse GSI)")

	storedDigest := attrB(t, item, attrSandboxContentDigest)
	assert.Len(t, storedDigest, 32)
	expected, err := contentDigest(sb)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expected, storedDigest), "digest stored on the row must equal the digest of the input")

	storedPB := attrB(t, item, attrSandboxPB)
	roundTripped := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, roundTripped))
	assert.True(t, proto.Equal(sb, roundTripped), "stored sandbox_pb must round-trip back to the input")
}

func TestCreate_IncludesNodeRefIDWhenSet(t *testing.T) {
	db, ctx := setupDB(t)
	// Sandbox status defaults to PHASE_PENDING in the fixture, so the
	// node-phase synthetic columns interpolate that.
	sb := newFixture(withID("sb-002"), withNode("node-1"))

	require.NoError(t, db.Create(ctx, sb))

	item := getItem(ctx, t, db, sb.Metadata.Id)
	assert.Equal(t, "node-1", attrS(t, item, attrNodeRefID))
	assert.Equal(t, "team-alpha#node-1", attrS(t, item, attrGSINamespaceNodeHK),
		"gsi_namespace_node_hk must be {namespace}#{node_id} when Node is set")
	assert.Equal(t, "node-1#PHASE_PENDING", attrS(t, item, attrGSINodePhaseHK),
		"gsi_node_phase_hk must be {node_id}#{phase} when Node is set")
	assert.Equal(t, "team-alpha#node-1#PHASE_PENDING", attrS(t, item, attrGSINamespaceNodePhaseHK),
		"gsi_namespace_node_phase_hk must be {namespace}#{node_id}#{phase} when Node is set")
}

func TestCreate_IdempotentOnEquivalentRetry(t *testing.T) {
	db, ctx := setupDB(t)

	first := newFixture(withID("sb-003"))
	require.NoError(t, db.Create(ctx, first))

	// Simulate a retry. Create stamps LastModifiedAt internally on every
	// call, so the retry's stamped timestamp differs from the first
	// call's — but the digest excludes wall-clock fields, so the digest
	// still matches the stored row and the call is a no-op success.
	retry := proto.Clone(first).(*cpv1.Sandbox)
	require.NoError(t, db.Create(ctx, retry))

	// The stored row must reflect the FIRST write — no overwrite occurred.
	item := getItem(ctx, t, db, "sb-003")
	storedPB := attrB(t, item, attrSandboxPB)
	stored := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t,
		first.Metadata.LastModifiedAt.AsTime().Equal(stored.Metadata.LastModifiedAt.AsTime()),
		"idempotent retry must not overwrite the existing row",
	)
}

func TestCreate_IdempotentAcrossDifferentScheduledNodes(t *testing.T) {
	// The random scheduler may pick a different node on retry, which used
	// to surface as ErrAlreadyExists because Node was part of the digest.
	// Now Node is server-managed and excluded from the digest, so a retry
	// with a different Node is still a no-op success.
	db, ctx := setupDB(t)

	first := newFixture(withID("sb-sched-idem"), withNode("node-A"))
	require.NoError(t, db.Create(ctx, first))

	retry := proto.Clone(first).(*cpv1.Sandbox)
	withNode("node-B")(retry)
	require.NoError(t, db.Create(ctx, retry),
		"a retry with a different scheduled node must be a no-op success")

	item := getItem(ctx, t, db, "sb-sched-idem")
	storedPB := attrB(t, item, attrSandboxPB)
	stored := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.Equal(t, "node-A", stored.GetNode().GetId(),
		"the stored Node must reflect the FIRST attempt's choice, not the retry's")
	assert.Equal(t, "node-A", attrS(t, item, attrNodeRefID),
		"the GSI sparse column must also reflect the first-write Node")
}

func TestCreate_ConflictOnDifferentBody(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-004"), withVCPU(2))
	require.NoError(t, db.Create(ctx, original))

	conflicting := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(4)(conflicting)
	err := db.Create(ctx, conflicting)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrAlreadyExists),
		"different body for same id must wrap dberrors.ErrAlreadyExists, got: %v", err)
}

func TestCreate_ConflictAfterPhaseTransition(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-005"))
	require.NoError(t, db.Create(ctx, original))

	// Simulate the next iteration's Update method: phase moves PENDING ->
	// RUNNING and the digest is recomputed/overwritten. We just write a
	// different digest directly, since the test only cares that the digest
	// no longer matches the input's.
	_, err := db.client.UpdateItem(ctx, &dynamodbservice.UpdateItemInput{
		TableName:        aws.String(db.tableName),
		Key:              map[string]types.AttributeValue{attrSandboxID: &types.AttributeValueMemberS{Value: "sb-005"}},
		UpdateExpression: aws.String("SET #d = :d, #p = :p"),
		ExpressionAttributeNames: map[string]string{
			"#d": attrSandboxContentDigest,
			"#p": attrSandboxStatusPhase,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":d": &types.AttributeValueMemberB{Value: bytes.Repeat([]byte{0xAA}, 32)},
			":p": &types.AttributeValueMemberS{Value: cpv1.SandboxStatus_PHASE_RUNNING.String()},
		},
	})
	require.NoError(t, err)

	retry := proto.Clone(original).(*cpv1.Sandbox)
	err = db.Create(ctx, retry)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrAlreadyExists),
		"retry after a server-side phase transition must surface as conflict, got: %v", err)
}

func TestContentDigest_IsDeterministic(t *testing.T) {
	// Map iteration order is randomised in Go; running the digest many
	// times exercises that randomness and guards against future drift if
	// someone removes the Deterministic marshal option.
	sb := newFixture(withID("sb-det"))
	want, err := contentDigest(sb)
	require.NoError(t, err)
	require.Len(t, want, 32)

	for i := 0; i < 100; i++ {
		got, err := contentDigest(sb)
		require.NoError(t, err)
		require.True(t, bytes.Equal(want, got), "digest must be stable across calls (iteration %d)", i)
	}
}

func TestContentDigest_IgnoresWallClockFields(t *testing.T) {
	a := newFixture(withID("sb-clock"))
	b := proto.Clone(a).(*cpv1.Sandbox)
	withLastModified(a.Metadata.LastModifiedAt.AsTime().Add(1 * time.Hour))(b)
	b.Metadata.CreatedAt = timestamppb.New(b.Metadata.CreatedAt.AsTime().Add(2 * time.Hour))

	da, err := contentDigest(a)
	require.NoError(t, err)
	db, err := contentDigest(b)
	require.NoError(t, err)

	assert.True(t, bytes.Equal(da, db), "digest must ignore CreatedAt and LastModifiedAt")
}

func TestContentDigest_IgnoresVersion(t *testing.T) {
	// Update idempotency depends on the digest being stable across the
	// version bump that a successful Update applies to the stored row.
	a := newFixture(withID("sb-ver"), withVersion(1))
	b := proto.Clone(a).(*cpv1.Sandbox)
	withVersion(2)(b)

	da, err := contentDigest(a)
	require.NoError(t, err)
	db, err := contentDigest(b)
	require.NoError(t, err)

	assert.True(t, bytes.Equal(da, db), "digest must ignore Metadata.Version")
}

func TestContentDigest_IgnoresNode(t *testing.T) {
	// Create idempotency under a non-deterministic scheduler depends on
	// the digest being stable across Node changes — a retry that picks a
	// different node must still hit the idempotent path.
	a := newFixture(withID("sb-node-digest"), withNode("node-A"))
	b := proto.Clone(a).(*cpv1.Sandbox)
	withNode("node-B")(b)

	da, err := contentDigest(a)
	require.NoError(t, err)
	dbDigest, err := contentDigest(b)
	require.NoError(t, err)

	assert.True(t, bytes.Equal(da, dbDigest), "digest must ignore Sandbox.Node")
}

func TestGet_HappyPath(t *testing.T) {
	db, ctx := setupDB(t)
	sb := newFixture(withID("sb-get-1"), withNode("node-7"))
	require.NoError(t, db.Create(ctx, sb))

	got, err := db.Get(ctx, "sb-get-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, proto.Equal(sb, got), "Get must round-trip the stored sandbox")
}

func TestGet_NotFound(t *testing.T) {
	db, ctx := setupDB(t)

	got, err := db.Get(ctx, "no-such-sandbox")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"missing sandbox must wrap dberrors.ErrNotFound, got: %v", err)
}

func TestUpdate_HappyPath(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-upd-1"), withVCPU(2))
	require.NoError(t, db.Create(ctx, original))
	createdLmt := original.Metadata.LastModifiedAt.AsTime()

	updated := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(4)(updated)
	// Sleep enough to guarantee a strictly later timestamp on Update —
	// the DB stamps LastModifiedAt with wall-clock precision, so two
	// calls in the same nanosecond would tie.
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Update(ctx, updated))

	item := getItem(ctx, t, db, "sb-upd-1")
	assert.Equal(t, "2", attrVersion(t, item),
		"successful Update must increment the stored version")

	storedPB := attrB(t, item, attrSandboxPB)
	stored := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.Equal(t, uint32(4), stored.Resources.VcpuCount,
		"stored row must reflect the updated body")
	assert.Equal(t, int64(2), stored.Metadata.Version,
		"sandbox_pb's version must agree with the sandbox_version column")

	updatedLmt := assertGSISortKey(t, item, "sb-upd-1")
	assert.True(t, updatedLmt.After(createdLmt),
		"a successful Update must advance the LastModifiedAt timestamp")
}

func TestUpdate_VersionConflict(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-upd-conflict"))
	require.NoError(t, db.Create(ctx, original))

	// First update bumps stored version 1 -> 2.
	first := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(8)(first)
	require.NoError(t, db.Update(ctx, first))

	// Second update reuses the stale version 1 with a *different* body, so
	// the digest cannot rescue it as an idempotent retry.
	stale := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(16)(stale)
	err := db.Update(ctx, stale)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrVersionConflict),
		"stale version with a different body must wrap ErrVersionConflict, got: %v", err)
}

func TestUpdate_NotFound(t *testing.T) {
	db, ctx := setupDB(t)

	sb := newFixture(withID("sb-missing"))
	err := db.Update(ctx, sb)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"update on missing sandbox must wrap ErrNotFound, got: %v", err)
}

func TestUpdate_IdempotentOnEquivalentRetry(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-upd-idem"), withVCPU(2))
	require.NoError(t, db.Create(ctx, original))

	first := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(4)(first)

	// Clone the request body BEFORE the first Update — Update mutates
	// Metadata.Version on success, and the retry must replay the version
	// the caller originally read (the version the gRPC client would
	// re-send on a network error), not the bumped value.
	retry := proto.Clone(first).(*cpv1.Sandbox)

	require.NoError(t, db.Update(ctx, first))
	firstWriteLmt := first.Metadata.LastModifiedAt.AsTime()

	// Retry the same logical Update. The DB stamps a fresh LMT
	// internally on the retry, so the retry's `retry.LastModifiedAt`
	// will differ from the first call's — but the digest excludes
	// wall-clock fields, so the digest still matches the stored row
	// and the call must be a no-op success. The stored LMT therefore
	// stays at firstWriteLmt.
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Update(ctx, retry))

	// Stored version must remain 2 — no second mutation occurred.
	item := getItem(ctx, t, db, "sb-upd-idem")
	assert.Equal(t, "2", attrVersion(t, item),
		"idempotent retry must not bump the version a second time")

	storedPB := attrB(t, item, attrSandboxPB)
	stored := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t,
		firstWriteLmt.Equal(stored.Metadata.LastModifiedAt.AsTime()),
		"idempotent retry must not overwrite LastModifiedAt on the stored row")
}

func TestUpdate_PauseRequiresRunning_OK(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(
		withID("sb-pause-ok"),
		withSavedPhase(cpv1.SandboxStatus_PHASE_RUNNING),
		withIntentPhase(cpv1.SandboxStatus_PHASE_RUNNING),
	)
	require.NoError(t, db.Create(ctx, original))

	intent := proto.Clone(original).(*cpv1.Sandbox)
	// Pause writes both the in-progress marker (status=PAUSING) and the
	// terminal target (intent=PAUSED) atomically.
	withSavedPhase(cpv1.SandboxStatus_PHASE_PAUSING)(intent)
	withIntentPhase(cpv1.SandboxStatus_PHASE_PAUSED)(intent)
	require.NoError(t, db.Update(ctx, intent),
		"pausing from saved phase RUNNING must succeed")

	item := getItem(ctx, t, db, "sb-pause-ok")
	assert.Equal(t, "2", attrVersion(t, item))
}

// TestUpdate_RecomputesNamespacePhaseHKOnPhaseTransition asserts that
// gsi_namespace_phase_hk is recomputed on every Update so that the row is
// always in the by_status_phase_index partition that matches its current
// Status.Phase — list-by-phase queries must reflect the current state.
func TestUpdate_RecomputesNamespacePhaseHKOnPhaseTransition(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(
		withID("sb-phase-hk"),
		withSavedPhase(cpv1.SandboxStatus_PHASE_RUNNING),
		withIntentPhase(cpv1.SandboxStatus_PHASE_RUNNING),
	)
	require.NoError(t, db.Create(ctx, original))

	item := getItem(ctx, t, db, "sb-phase-hk")
	assert.Equal(t, "team-alpha#PHASE_RUNNING", attrS(t, item, attrGSINamespacePhaseHK),
		"create must seed gsi_namespace_phase_hk from the initial phase")

	intent := proto.Clone(original).(*cpv1.Sandbox)
	withSavedPhase(cpv1.SandboxStatus_PHASE_PAUSING)(intent)
	withIntentPhase(cpv1.SandboxStatus_PHASE_PAUSED)(intent)
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Update(ctx, intent))

	item = getItem(ctx, t, db, "sb-phase-hk")
	assert.Equal(t, "team-alpha#PHASE_PAUSING", attrS(t, item, attrGSINamespacePhaseHK),
		"update must recompute gsi_namespace_phase_hk when the saved phase changes")
}

func TestUpdate_PauseRequiresRunning_Fails(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(
		withID("sb-pause-bad"),
		withSavedPhase(cpv1.SandboxStatus_PHASE_PENDING),
	)
	require.NoError(t, db.Create(ctx, original))

	intent := proto.Clone(original).(*cpv1.Sandbox)
	withSavedPhase(cpv1.SandboxStatus_PHASE_PAUSING)(intent)
	withIntentPhase(cpv1.SandboxStatus_PHASE_PAUSED)(intent)
	err := db.Update(ctx, intent)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sandboxdb.ErrInvalidPhaseTransition),
		"pausing from non-running must wrap ErrInvalidPhaseTransition, got: %v", err)
}

func TestUpdate_ResumeRequiresPaused_OK(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(
		withID("sb-resume-ok"),
		withSavedPhase(cpv1.SandboxStatus_PHASE_PAUSED),
		withIntentPhase(cpv1.SandboxStatus_PHASE_PAUSED),
	)
	require.NoError(t, db.Create(ctx, original))

	intent := proto.Clone(original).(*cpv1.Sandbox)
	// Resume writes status=RESUMING + intent=RUNNING. There is no separate
	// PHASE_RESUMED — once the reconciler closes the loop, status converges
	// to RUNNING.
	withSavedPhase(cpv1.SandboxStatus_PHASE_RESUMING)(intent)
	withIntentPhase(cpv1.SandboxStatus_PHASE_RUNNING)(intent)
	require.NoError(t, db.Update(ctx, intent),
		"resuming from saved phase PAUSED must succeed")
}

func TestUpdate_ResumeRequiresPaused_Fails(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(
		withID("sb-resume-bad"),
		withSavedPhase(cpv1.SandboxStatus_PHASE_RUNNING),
		withIntentPhase(cpv1.SandboxStatus_PHASE_RUNNING),
	)
	require.NoError(t, db.Create(ctx, original))

	intent := proto.Clone(original).(*cpv1.Sandbox)
	withSavedPhase(cpv1.SandboxStatus_PHASE_RESUMING)(intent)
	withIntentPhase(cpv1.SandboxStatus_PHASE_RUNNING)(intent)
	err := db.Update(ctx, intent)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sandboxdb.ErrInvalidPhaseTransition),
		"resuming from non-paused must wrap ErrInvalidPhaseTransition, got: %v", err)
}

// TestUpdate_StampsLastModifiedAtRegardlessOfInput is the regression
// guard for the original EstablishSession bug: a daemon-side caller
// that forwards a stale (or zeroed) LastModifiedAt must still see the
// stored row's timestamp advance to a recent server time. The DB owns
// the field, so the caller's value is irrelevant.
func TestUpdate_StampsLastModifiedAtRegardlessOfInput(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-lmt-bug"))
	require.NoError(t, db.Create(ctx, original))

	updated := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(4)(updated)
	// Force a stale timestamp on the input. The bug being guarded
	// against is "DB doesn't stamp; stored LMT keeps this stale value."
	stale := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	withLastModified(stale)(updated)

	require.NoError(t, db.Update(ctx, updated))

	item := getItem(ctx, t, db, "sb-lmt-bug")
	gotLmt := assertGSISortKey(t, item, "sb-lmt-bug")
	assert.True(t, gotLmt.After(stale),
		"stored LMT must advance past the caller-supplied stale value")

	storedPB := attrB(t, item, attrSandboxPB)
	stored := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t, stored.Metadata.LastModifiedAt.AsTime().After(stale),
		"stored Metadata.LastModifiedAt must advance past the caller's stale value")
}

// TestUpdate_PreservesCreatedAt locks in the invariant that Update
// never touches CreatedAt — only LastModifiedAt. CreatedAt belongs to
// Create.
func TestUpdate_PreservesCreatedAt(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("sb-created-at"))
	require.NoError(t, db.Create(ctx, original))
	createdAt := original.Metadata.CreatedAt.AsTime()

	updated := proto.Clone(original).(*cpv1.Sandbox)
	withVCPU(4)(updated)
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Update(ctx, updated))

	stored, err := db.Get(ctx, "sb-created-at")
	require.NoError(t, err)
	assert.True(t, createdAt.Equal(stored.Metadata.CreatedAt.AsTime()),
		"Update must not mutate CreatedAt — got %v, want %v",
		stored.Metadata.CreatedAt.AsTime(), createdAt)
	assert.True(t,
		stored.Metadata.LastModifiedAt.AsTime().After(createdAt),
		"LastModifiedAt should have advanced past CreatedAt")
}
