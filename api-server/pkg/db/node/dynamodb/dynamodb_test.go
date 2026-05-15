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

	dberrors "github.com/alan-ghelardi/yaghan/api-server/pkg/db"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	"github.com/alan-ghelardi/yaghan/commons/pkg/aws/dynamodb"
	awstesting "github.com/alan-ghelardi/yaghan/commons/pkg/aws/testing"
	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const nodesSchemaPath = "../../../../../dynamodb-tables/nodes.json"

// sharedDynamoDBEndpoint is the http endpoint of the package-shared
// DynamoDB Local container. Per-test isolation comes from CreateTable's
// random table suffix.
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

	tableName := awstesting.CreateTable(t, sharedDynamoDBEndpoint, nodesSchemaPath)

	ctx := t.Context()
	awsCfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, awsCfg)
	client := dynamodb.New(ctx, dynamodb.Config{Endpoint: sharedDynamoDBEndpoint})

	return &dynamoDB{client: client, tableName: tableName}, ctx
}

func newFixture(opts ...func(*cpv1.Node)) *cpv1.Node {
	now := time.Date(2026, 5, 6, 10, 30, 45, 123456789, time.UTC)
	// Version is zero by default — the DB stamps initialVersion on the
	// create branch of Put.
	n := &cpv1.Node{
		Metadata: &cpv1.NodeMeta{
			Id:             "node-001",
			CreatedAt:      timestamppb.New(now),
			LastModifiedAt: timestamppb.New(now),
		},
		Resources: &cpv1.NodeResources{
			CpuCapacityMillicores: 8000,
			MemoryCapacityBytes:   16 * 1024 * 1024 * 1024,
			DiskCapacityBytes:     100 * 1024 * 1024 * 1024,
		},
		Status: &cpv1.NodeStatus{
			Phase: cpv1.NodeStatus_PHASE_HEALTHY,
		},
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

func withID(id string) func(*cpv1.Node) {
	return func(n *cpv1.Node) { n.Metadata.Id = id }
}

func withVersion(v int64) func(*cpv1.Node) {
	return func(n *cpv1.Node) { n.Metadata.Version = v }
}

func withCPUMillicores(c uint32) func(*cpv1.Node) {
	return func(n *cpv1.Node) { n.Resources.CpuCapacityMillicores = c }
}

func withPhase(p cpv1.NodeStatus_Phase) func(*cpv1.Node) {
	return func(n *cpv1.Node) { n.Status.Phase = p }
}

func withLastModified(ts time.Time) func(*cpv1.Node) {
	return func(n *cpv1.Node) { n.Metadata.LastModifiedAt = timestamppb.New(ts) }
}

func getItem(ctx context.Context, t *testing.T, db *dynamoDB, id string) map[string]types.AttributeValue {
	t.Helper()
	out, err := db.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(db.tableName),
		Key:            map[string]types.AttributeValue{attrNodeID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Item, "expected node %q to be present", id)
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

func attrVersion(t *testing.T, item map[string]types.AttributeValue) string {
	t.Helper()
	av, ok := item[attrNodeVersion]
	require.True(t, ok, "attribute %q is missing", attrNodeVersion)
	n, ok := av.(*types.AttributeValueMemberN)
	require.True(t, ok, "attribute %q is not a number", attrNodeVersion)
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
// `<rfc3339nano-timestamp>#<id>` shape and that the embedded timestamp
// is within a few seconds of now. The DB layer owns LastModifiedAt, so
// tests cannot assert against a literal caller-supplied value.
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

func TestPut_CreateHappyPath(t *testing.T) {
	db, ctx := setupDB(t)
	n := newFixture()

	require.NoError(t, db.Put(ctx, n))

	item := getItem(ctx, t, db, n.Metadata.Id)
	assert.Equal(t, "node-001", attrS(t, item, attrNodeID))
	assert.Equal(t, "PHASE_HEALTHY", attrS(t, item, attrNodeStatusPhase))
	assert.Equal(t, allNodesPartition, attrS(t, item, attrGSIConstHK),
		"every row must share the all-nodes partition key")
	assertGSISortKey(t, item, "node-001")
	assert.Equal(t, "1", attrVersion(t, item),
		"create branch must stamp initialVersion")

	// Input proto reflects the stored state on success.
	assert.Equal(t, int64(1), n.Metadata.Version)
	assert.NotNil(t, n.Metadata.CreatedAt)
	assert.NotNil(t, n.Metadata.LastModifiedAt)
	assert.WithinDuration(t, time.Now(), n.Metadata.CreatedAt.AsTime(), 5*time.Second,
		"server must stamp CreatedAt on create")

	storedDigest := attrB(t, item, attrNodeContentDigest)
	assert.Len(t, storedDigest, 32)
	expected, err := contentDigest(n)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expected, storedDigest))

	storedPB := attrB(t, item, attrNodePB)
	roundTripped := &cpv1.Node{}
	require.NoError(t, proto.Unmarshal(storedPB, roundTripped))
	assert.True(t, proto.Equal(n, roundTripped),
		"stored node_pb must round-trip to the input")
}

func TestPut_CreateConflictOnDifferentBody(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("node-conflict"), withCPUMillicores(4000))
	require.NoError(t, db.Put(ctx, original))

	conflicting := newFixture(withID("node-conflict"), withCPUMillicores(8000))
	err := db.Put(ctx, conflicting)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrAlreadyExists),
		"different body for same id on create branch must wrap ErrAlreadyExists, got: %v", err)
}

func TestPut_CreateIdempotentOnEquivalentRetry(t *testing.T) {
	db, ctx := setupDB(t)

	first := newFixture(withID("node-idem-create"))
	require.NoError(t, db.Put(ctx, first))

	// Simulate a retry of the same logical create. Put stamps
	// LastModifiedAt (and CreatedAt on this branch) internally on every
	// call, so the timestamps differ — but the digest excludes them, so
	// the call is a no-op success and the stored row is preserved.
	retry := newFixture(withID("node-idem-create"))
	require.NoError(t, db.Put(ctx, retry))

	item := getItem(ctx, t, db, "node-idem-create")
	assert.Equal(t, "1", attrVersion(t, item),
		"idempotent retry must not advance the version")
	storedPB := attrB(t, item, attrNodePB)
	stored := &cpv1.Node{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t,
		first.Metadata.LastModifiedAt.AsTime().Equal(stored.Metadata.LastModifiedAt.AsTime()),
		"idempotent retry must not overwrite the existing row")
}

func TestPut_UpdateHappyPath(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("node-upd-1"), withCPUMillicores(4000))
	require.NoError(t, db.Put(ctx, original))
	createdLmt := original.Metadata.LastModifiedAt.AsTime()
	createdAt := original.Metadata.CreatedAt.AsTime()

	updated := proto.CloneOf(original)
	updated.Resources.CpuCapacityMillicores = 16000
	// Sleep enough to guarantee a strictly later LMT.
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Put(ctx, updated))

	item := getItem(ctx, t, db, "node-upd-1")
	assert.Equal(t, "2", attrVersion(t, item),
		"successful update must increment the version")

	storedPB := attrB(t, item, attrNodePB)
	stored := &cpv1.Node{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.Equal(t, uint32(16000), stored.Resources.CpuCapacityMillicores,
		"stored row must reflect the updated body")
	assert.Equal(t, int64(2), stored.Metadata.Version)
	assert.True(t, createdAt.Equal(stored.Metadata.CreatedAt.AsTime()),
		"update must not mutate CreatedAt")

	updatedLmt := assertGSISortKey(t, item, "node-upd-1")
	assert.True(t, updatedLmt.After(createdLmt),
		"successful update must advance LastModifiedAt")
}

func TestPut_UpdateVersionConflict(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("node-upd-conflict"))
	require.NoError(t, db.Put(ctx, original))

	// First update bumps version 1 → 2.
	first := proto.CloneOf(original)
	first.Resources.CpuCapacityMillicores = 8000
	require.NoError(t, db.Put(ctx, first))

	// Second update reuses the stale version 1 with a different body.
	stale := proto.CloneOf(original)
	stale.Resources.CpuCapacityMillicores = 32000
	err := db.Put(ctx, stale)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrVersionConflict),
		"stale version with different body must wrap ErrVersionConflict, got: %v", err)
}

func TestPut_UpdateNotFound(t *testing.T) {
	db, ctx := setupDB(t)

	missing := newFixture(withID("node-missing"), withVersion(1))
	err := db.Put(ctx, missing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"update on missing node must wrap ErrNotFound, got: %v", err)
}

func TestPut_UpdateIdempotentOnEquivalentRetry(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("node-idem-upd"), withCPUMillicores(4000))
	require.NoError(t, db.Put(ctx, original))

	first := proto.CloneOf(original)
	first.Resources.CpuCapacityMillicores = 8000

	// Clone the request body BEFORE the first Put — Put mutates
	// Metadata.Version on success, and the retry must replay the version
	// the caller originally read.
	retry := proto.CloneOf(first)

	require.NoError(t, db.Put(ctx, first))
	firstWriteLmt := first.Metadata.LastModifiedAt.AsTime()

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Put(ctx, retry))

	item := getItem(ctx, t, db, "node-idem-upd")
	assert.Equal(t, "2", attrVersion(t, item),
		"idempotent retry must not bump version a second time")

	storedPB := attrB(t, item, attrNodePB)
	stored := &cpv1.Node{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t,
		firstWriteLmt.Equal(stored.Metadata.LastModifiedAt.AsTime()),
		"idempotent retry must not overwrite LastModifiedAt on the stored row")
}

func TestPut_StampsLastModifiedAtRegardlessOfInput(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("node-lmt"))
	require.NoError(t, db.Put(ctx, original))

	updated := proto.CloneOf(original)
	updated.Resources.CpuCapacityMillicores = 12000
	stale := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	withLastModified(stale)(updated)

	require.NoError(t, db.Put(ctx, updated))

	item := getItem(ctx, t, db, "node-lmt")
	gotLmt := assertGSISortKey(t, item, "node-lmt")
	assert.True(t, gotLmt.After(stale),
		"stored LMT must advance past the caller-supplied stale value")
}

func TestContentDigest_IsDeterministic(t *testing.T) {
	n := newFixture(withID("node-det"))
	want, err := contentDigest(n)
	require.NoError(t, err)
	require.Len(t, want, 32)

	for i := 0; i < 100; i++ {
		got, err := contentDigest(n)
		require.NoError(t, err)
		require.True(t, bytes.Equal(want, got),
			"digest must be stable across calls (iteration %d)", i)
	}
}

func TestContentDigest_IgnoresWallClockAndVersion(t *testing.T) {
	a := newFixture(withID("node-cd"))
	b := proto.CloneOf(a)
	withLastModified(a.Metadata.LastModifiedAt.AsTime().Add(1 * time.Hour))(b)
	b.Metadata.CreatedAt = timestamppb.New(b.Metadata.CreatedAt.AsTime().Add(2 * time.Hour))
	b.Metadata.Version = 99

	da, err := contentDigest(a)
	require.NoError(t, err)
	dbDigest, err := contentDigest(b)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(da, dbDigest),
		"digest must ignore CreatedAt, LastModifiedAt and Version")
}

func TestGet_HappyPath(t *testing.T) {
	db, ctx := setupDB(t)
	n := newFixture(withID("node-get"))
	require.NoError(t, db.Put(ctx, n))

	got, err := db.Get(ctx, "node-get")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, proto.Equal(n, got), "Get must round-trip the stored node")
}

func TestGet_NotFound(t *testing.T) {
	db, ctx := setupDB(t)

	got, err := db.Get(ctx, "no-such-node")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"missing node must wrap ErrNotFound, got: %v", err)
}
