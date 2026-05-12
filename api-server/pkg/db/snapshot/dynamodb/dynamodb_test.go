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
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
	awstesting "golang.nuinfra.net/commons/pkg/aws/testing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const snapshotsSchemaPath = "../../../../../dynamodb-tables/snapshots.json"

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

	tableName := awstesting.CreateTable(t, sharedDynamoDBEndpoint, snapshotsSchemaPath)

	ctx := t.Context()
	awsCfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, awsCfg)
	client := dynamodb.New(ctx, dynamodb.Config{Endpoint: sharedDynamoDBEndpoint})

	return &dynamoDB{client: client, tableName: tableName}, ctx
}

func newFixture(opts ...func(*cpv1.Snapshot)) *cpv1.Snapshot {
	sn := &cpv1.Snapshot{
		Metadata: &cpv1.SnapshotMeta{
			Id:          "snap-001",
			Namespace:   "team-alpha",
			Description: "nightly checkpoint",
		},
		Sandbox: &cpv1.SandboxRef{Id: "sb-001"},
	}
	for _, opt := range opts {
		opt(sn)
	}
	return sn
}

func withID(id string) func(*cpv1.Snapshot) {
	return func(sn *cpv1.Snapshot) { sn.Metadata.Id = id }
}

func withNamespace(ns string) func(*cpv1.Snapshot) {
	return func(sn *cpv1.Snapshot) { sn.Metadata.Namespace = ns }
}

func withSandbox(id string) func(*cpv1.Snapshot) {
	return func(sn *cpv1.Snapshot) { sn.Sandbox = &cpv1.SandboxRef{Id: id} }
}

func withDescription(d string) func(*cpv1.Snapshot) {
	return func(sn *cpv1.Snapshot) { sn.Metadata.Description = d }
}

func getItem(ctx context.Context, t *testing.T, db *dynamoDB, id string) map[string]types.AttributeValue {
	t.Helper()
	out, err := db.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(db.tableName),
		Key:            map[string]types.AttributeValue{attrSnapshotID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Item, "expected snapshot %q to be present", id)
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
// is within a few seconds of now. The DB layer owns CreatedAt, so tests
// cannot assert against a literal caller-supplied value.
func assertGSISortKey(t *testing.T, item map[string]types.AttributeValue, id string) time.Time {
	t.Helper()
	sk := attrS(t, item, attrGSICtSK)
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
	sn := newFixture()

	require.NoError(t, db.Create(ctx, sn))

	item := getItem(ctx, t, db, sn.Metadata.Id)

	assert.Equal(t, "snap-001", attrS(t, item, attrSnapshotID))
	assert.Equal(t, "team-alpha", attrS(t, item, attrSnapshotNamespace))
	assert.Equal(t, "sb-001", attrS(t, item, attrSandboxRefID))
	assertGSISortKey(t, item, "snap-001")

	storedDigest := attrB(t, item, attrSnapshotContentDigest)
	assert.Len(t, storedDigest, 32)
	expected, err := contentDigest(sn)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(expected, storedDigest), "digest stored on the row must equal the digest of the input")

	storedPB := attrB(t, item, attrSnapshotPB)
	roundTripped := &cpv1.Snapshot{}
	require.NoError(t, proto.Unmarshal(storedPB, roundTripped))
	assert.True(t, proto.Equal(sn, roundTripped), "stored snapshot_pb must round-trip back to the input")

	require.NotNil(t, sn.Metadata.CreatedAt,
		"Create must stamp Metadata.CreatedAt on the input proto")
	assert.WithinDuration(t, time.Now(), sn.Metadata.CreatedAt.AsTime(), 5*time.Second)
}

func TestCreate_IdempotentOnEquivalentRetry(t *testing.T) {
	db, ctx := setupDB(t)

	first := newFixture(withID("snap-idem"))
	require.NoError(t, db.Create(ctx, first))
	firstCreatedAt := first.Metadata.CreatedAt.AsTime()

	// Simulate a retry. Create stamps CreatedAt internally on every call,
	// so the retry's stamped timestamp differs from the first call's —
	// but the digest excludes CreatedAt, so the digest still matches the
	// stored row and the call is a no-op success.
	retry := proto.CloneOf(first)
	retry.Metadata.CreatedAt = nil
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, db.Create(ctx, retry))

	// The stored row must reflect the FIRST write — no overwrite occurred.
	item := getItem(ctx, t, db, "snap-idem")
	storedPB := attrB(t, item, attrSnapshotPB)
	stored := &cpv1.Snapshot{}
	require.NoError(t, proto.Unmarshal(storedPB, stored))
	assert.True(t,
		firstCreatedAt.Equal(stored.Metadata.CreatedAt.AsTime()),
		"idempotent retry must not overwrite the existing row's CreatedAt",
	)
}

func TestCreate_ConflictOnDifferentBody(t *testing.T) {
	db, ctx := setupDB(t)

	original := newFixture(withID("snap-conflict"), withDescription("first"))
	require.NoError(t, db.Create(ctx, original))

	conflicting := proto.CloneOf(original)
	withDescription("second")(conflicting)
	conflicting.Metadata.CreatedAt = nil

	err := db.Create(ctx, conflicting)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrAlreadyExists),
		"different body for same id must wrap ErrAlreadyExists, got: %v", err)
}

func TestContentDigest_IsDeterministic(t *testing.T) {
	// Map iteration order is randomised in Go; running the digest many
	// times exercises that randomness and guards against future drift if
	// someone removes the Deterministic marshal option.
	sn := newFixture(withID("snap-det"))
	want, err := contentDigest(sn)
	require.NoError(t, err)
	require.Len(t, want, 32)

	for i := 0; i < 100; i++ {
		got, err := contentDigest(sn)
		require.NoError(t, err)
		require.True(t, bytes.Equal(want, got), "digest must be stable across calls (iteration %d)", i)
	}
}

func TestContentDigest_IgnoresCreatedAt(t *testing.T) {
	a := newFixture(withID("snap-clock"))
	b := proto.CloneOf(a)
	// Stamp two different CreatedAt values; the digest must agree.
	a.Metadata.CreatedAt = timestamppb.Now()
	b.Metadata.CreatedAt = timestamppb.New(time.Now().Add(1 * time.Hour))

	da, err := contentDigest(a)
	require.NoError(t, err)
	dbDigest, err := contentDigest(b)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(da, dbDigest), "digest must ignore CreatedAt")
}

func TestGet_HappyPath(t *testing.T) {
	db, ctx := setupDB(t)
	sn := newFixture(withID("snap-get-1"))
	require.NoError(t, db.Create(ctx, sn))

	got, err := db.Get(ctx, "snap-get-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, proto.Equal(sn, got), "Get must round-trip the stored snapshot")
}

func TestGet_NotFound(t *testing.T) {
	db, ctx := setupDB(t)

	got, err := db.Get(ctx, "no-such-snapshot")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"missing snapshot must wrap ErrNotFound, got: %v", err)
}

func TestDelete_RemovesExistingRow(t *testing.T) {
	db, ctx := setupDB(t)

	sn := newFixture(withID("snap-del-1"))
	require.NoError(t, db.Create(ctx, sn))

	require.NoError(t, db.Delete(ctx, "snap-del-1"))

	_, err := db.Get(ctx, "snap-del-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrNotFound),
		"snapshot must be gone after Delete, got: %v", err)
}

func TestDelete_IdempotentOnMissingRow(t *testing.T) {
	db, ctx := setupDB(t)

	// Snapshots are immutable; the contract is "row no longer exists
	// after this call returns". A Delete on a never-created id must
	// succeed so retries (after partial network failures) are safe.
	require.NoError(t, db.Delete(ctx, "snap-never-existed"))
}

func TestDelete_DoubleDeleteIsIdempotent(t *testing.T) {
	db, ctx := setupDB(t)

	sn := newFixture(withID("snap-del-twice"))
	require.NoError(t, db.Create(ctx, sn))
	require.NoError(t, db.Delete(ctx, "snap-del-twice"))
	require.NoError(t, db.Delete(ctx, "snap-del-twice"),
		"a second Delete on an already-deleted snapshot must succeed")
}
