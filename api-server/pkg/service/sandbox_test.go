package service_test

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.nuinfra.api-server/pkg/config"
	nodedb "golang.nuinfra.api-server/pkg/db/node"
	nodedynamodb "golang.nuinfra.api-server/pkg/db/node/dynamodb"
	"golang.nuinfra.api-server/pkg/db/sandbox/dynamodb"
	"golang.nuinfra.api-server/pkg/service"
	"golang.nuinfra.api-server/pkg/watch"
	"golang.nuinfra.api-server/pkg/watch/factory"
	redistesting "golang.nuinfra.api-server/pkg/watch/providers/redis/testing"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	awsdynamodb "golang.nuinfra.net/commons/pkg/aws/dynamodb"
	awstesting "golang.nuinfra.net/commons/pkg/aws/testing"
	commonsconfig "golang.nuinfra.net/commons/pkg/config"
	commonsserver "golang.nuinfra.net/commons/pkg/server"
	servertesting "golang.nuinfra.net/commons/pkg/server/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sandboxesSchemaPath = "../../../dynamodb-tables/sandboxes.json"
const nodesSchemaPath = "../../../dynamodb-tables/nodes.json"
const snapshotsSchemaPath = "../../../dynamodb-tables/snapshots.json"

// sharedDynamoDBEndpoint is the http endpoint of the package-shared
// DynamoDB Local container started by TestMain. The redis container
// for the WatchableStream is still spun up per-test (only ~5 tests
// use it), so it is not shared here.
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

// validNamespace satisfies the post-fix regex
// `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`.
const validNamespace = "team-alpha"

type harness struct {
	client    cpv1.SandboxServiceClient
	cluster   cpv1.ClusterServiceClient
	snapshot  cpv1.SnapshotServiceClient
	ddb       awsdynamodb.Client
	tableName string

	// nodeDB lets a test seed nodes directly (e.g. for GetNode and
	// ListNodes coverage), since no node-write RPC exists yet. It
	// targets the same nodes table the api-server is running against.
	nodeDB nodedb.DB

	// Populated when startService is invoked with withEventStream(); empty
	// otherwise. db lets a test simulate an internal write (e.g. an
	// imaginary scheduler stamping Sandbox.Node) and have the resulting
	// Event flow to whatever EstablishSession watchers the apiServer holds
	// — both this WatchableDB and the apiServer's share the same redis
	// stream and the same dynamodb table.
	redisAddr string
	db        *service.WatchableDB
	stream    watch.WatchableStream[*cpv1.Event]
}

type harnessConfig struct {
	withEventStream bool
	skipDefaultNode bool
}

type harnessOption func(*harnessConfig)

// withEventStream enables a redis-backed WatchableStream on the harness so
// EstablishSession can register watchers and the test can publish events via
// harness.db.Update(...).
func withEventStream() harnessOption { //nolint:unused // used by cluster_test.go
	return func(c *harnessConfig) { c.withEventStream = true }
}

// withoutDefaultNode disables the harness's default seeded healthy node.
// Use this in tests that assert on the exact set of nodes returned by
// ListNodes / GetNode, or that need to exercise the no-healthy-nodes
// scheduling path. Default behaviour seeds one healthy node so
// CreateSandbox tests have somewhere to schedule to.
func withoutDefaultNode() harnessOption { //nolint:unused // used across cluster_node_test.go and the no-healthy-nodes test
	return func(c *harnessConfig) { c.skipDefaultNode = true }
}

// defaultHarnessNodeID is the id of the healthy node startService seeds
// by default. CreateSandbox tests can assert that scheduled sandboxes
// land on this node without coupling to test ordering.
const defaultHarnessNodeID = "harness-node"

// setSavedPhase rewrites the stored Status.Phase out-of-band so Pause/Resume
// tests can simulate the data-plane reconciler advancing PENDING → RUNNING or
// RUNNING → PAUSED. CreateSandbox always seeds PENDING and there is no
// reconciler in this test process.
//
// TODO: delete this helper once the data-plane reconciler exists in the test
// fixture and can drive these transitions through the public surface.
func (h *harness) setSavedPhase(ctx context.Context, t *testing.T, id string, phase cpv1.SandboxStatus_Phase) {
	t.Helper()

	out, err := h.ddb.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName: awssdk.String(h.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"sandbox_id": &ddbtypes.AttributeValueMemberS{Value: id},
		},
		ConsistentRead: awssdk.Bool(true),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Item, "sandbox %q not found", id)

	pb := out.Item["sandbox_pb"].(*ddbtypes.AttributeValueMemberB).Value
	sb := &cpv1.Sandbox{}
	require.NoError(t, proto.Unmarshal(pb, sb))
	sb.Status = &cpv1.SandboxStatus{Phase: phase}

	updatedPB, err := proto.Marshal(sb)
	require.NoError(t, err)

	// Keep gsi_namespace_phase_hk consistent with the new phase so the
	// row migrates between partitions of by_status_phase_index the same
	// way a real Update would; otherwise list-by-phase queries miss
	// rows mutated through this back-door.
	gsiPhaseHK := sb.GetMetadata().GetNamespace() + "#" + phase.String()

	_, err = h.ddb.UpdateItem(ctx, &dynamodbservice.UpdateItemInput{
		TableName: awssdk.String(h.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"sandbox_id": &ddbtypes.AttributeValueMemberS{Value: id},
		},
		UpdateExpression: awssdk.String("SET #p = :p, #pb = :pb, #hk = :hk"),
		ExpressionAttributeNames: map[string]string{
			"#p":  "sandbox_status_phase",
			"#pb": "sandbox_pb",
			"#hk": "gsi_namespace_phase_hk",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":p":  &ddbtypes.AttributeValueMemberS{Value: phase.String()},
			":pb": &ddbtypes.AttributeValueMemberB{Value: updatedPB},
			":hk": &ddbtypes.AttributeValueMemberS{Value: gsiPhaseHK},
		},
	})
	require.NoError(t, err)
}

func startService(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()

	cfg := harnessConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	endpoint := sharedDynamoDBEndpoint
	tableName := awstesting.CreateTable(t, endpoint, sandboxesSchemaPath)
	nodesTableName := awstesting.CreateTable(t, endpoint, nodesSchemaPath)
	snapshotsTableName := awstesting.CreateTable(t, endpoint, snapshotsSchemaPath)

	// Pre-bind the gRPC listener and pass it to server.Start via context.
	// This eliminates the close→relisten race that exists when picking a
	// free port via :0 and reopening it later.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	bundle := &config.Bundle{
		Base: commonsconfig.Base{
			GRPCPort: uint(port),
			KeepAlive: &commonsconfig.KeepAlive{
				MinTime:  commonsconfig.DefaultKeepAliveMinTime,
				Interval: commonsconfig.DefaultKeepAliveInterval,
				Timeout:  commonsconfig.DefaultKeepAliveTimeout,
			},
			MaxConcurrentStreams: math.MaxUint32,
		},
		Database: &config.Database{
			AWS: &config.AWS{
				SandboxesTableName: tableName,
				NodesTableName:     nodesTableName,
				SnapshotsTableName: snapshotsTableName,
				EndpointURL:        endpoint,
			},
		},
	}

	var redisAddr string
	if cfg.withEventStream {
		redisAddr = redistesting.WithRedis(t)
		bundle.WatchStream = &config.WatchStream{
			EventDeliveryTimeout: 5 * time.Second,
			Redis: &config.Redis{
				Address:                redisAddr,
				StreamName:             "test",
				StreamKey:              "test",
				KeyTTL:                 5 * time.Minute,
				Timeout:                15 * time.Second,
				StreamReadinessTimeout: 5 * time.Second,
			},
		}
	}

	// Compose the context apiServer.Setup expects: AWS config + dynamodb
	// client. Mirrors what a future api-server main.go would do.
	ctx := t.Context()
	ctx = awsconfig.With(ctx, awsconfig.New(ctx))
	ddbClient := awsdynamodb.New(ctx, awsdynamodb.Config{Endpoint: endpoint})
	ctx = awsdynamodb.With(ctx, ddbClient)

	ctx = commonsserver.WithListener(ctx, listener)
	servertesting.StartServer(ctx, t, service.New(bundle))

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	h := &harness{
		client:    cpv1.NewSandboxServiceClient(conn),
		cluster:   cpv1.NewClusterServiceClient(conn),
		snapshot:  cpv1.NewSnapshotServiceClient(conn),
		ddb:       ddbClient,
		tableName: tableName,
		nodeDB:    nodedynamodb.New(ctx, bundle),
		redisAddr: redisAddr,
	}

	if cfg.withEventStream {
		// Build a parallel WatchableDB that targets the same redis stream
		// and dynamodb table the apiServer is using. Tests use h.db.Update
		// to simulate an internal write (e.g. a future scheduler stamping
		// Sandbox.Node) and observe the resulting Event arrive via the
		// EstablishSession stream. h.stream is a separate client to the
		// same redis instance — handy for assertions that don't need to
		// go through the gRPC server.
		stream, err := factory.NewEventStream(ctx, bundle, sandboxEventIDForTest)
		require.NoError(t, err)
		t.Cleanup(func() { /* no Close() on WatchableStream interface */ })

		rawDB := dynamodb.New(ctx, bundle)
		h.stream = stream
		h.db = service.NewWatchableDB(rawDB, stream)
	}

	if !cfg.skipDefaultNode {
		// Seed one healthy node so CreateSandbox tests have somewhere to
		// schedule to. Tests that assert on the exact set of nodes
		// returned by ListNodes / GetNode opt out via withoutDefaultNode.
		require.NoError(t, h.nodeDB.Put(ctx, &cpv1.Node{
			Metadata: &cpv1.NodeMeta{Id: defaultHarnessNodeID},
			Resources: &cpv1.NodeResources{
				CpuCapacityMillicores: 8000,
				MemoryCapacityBytes:   16 * 1024 * 1024 * 1024,
				DiskCapacityBytes:     100 * 1024 * 1024 * 1024,
			},
			Status: &cpv1.NodeStatus{Phase: cpv1.NodeStatus_PHASE_HEALTHY},
		}))
	}

	return h
}

// sandboxEventIDForTest mirrors the SetEventIDFunc the apiServer wires up
// internally; the redis provider stamps each delivered Event with its
// stream message id.
func sandboxEventIDForTest(event *cpv1.Event, eventID string) {
	event.Id = eventID
}

func newCreateRequest(opts ...func(*cpv1.Sandbox)) *cpv1.CreateSandboxRequest {
	sb := &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{
			Id:        "sb-001",
			Namespace: validNamespace,
			Labels:    map[string]string{"app": "demo"},
		},
		Resources: &cpv1.Resources{
			VcpuCount: 2,
			MemoryMib: 1024,
		},
	}
	for _, opt := range opts {
		opt(sb)
	}
	return &cpv1.CreateSandboxRequest{Sandbox: sb}
}

func withID(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.Id = id }
}

func withNamespace(ns string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.Namespace = ns }
}

func withVCPU(n uint32) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Resources.VcpuCount = n }
}

func withMemory(m uint64) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Resources.MemoryMib = m }
}

func withLabel(k, v string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.Labels[k] = v }
}

func withClientStatus(p cpv1.SandboxStatus_Phase) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Status = &cpv1.SandboxStatus{Phase: p} }
}

func withClientIntent(p cpv1.SandboxStatus_Phase) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Intent = &cpv1.Intent{Phase: p} }
}

func withClientNode(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Node = &cpv1.NodeRef{Id: id} }
}

func withClientCreatedAt(ts time.Time) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) { sb.Metadata.CreatedAt = timestamppb.New(ts) }
}

func withSnapshotSource(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) {
		sb.Metadata.Source = &cpv1.SandboxSource{
			Reference: &cpv1.SandboxSource_SnapshotId{SnapshotId: id},
		}
	}
}

func withImageSource(id string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) {
		sb.Metadata.Source = &cpv1.SandboxSource{
			Reference: &cpv1.SandboxSource_ImageId{ImageId: id},
		}
	}
}

// seedSnapshot persists a snapshot via the public RPC so the DB row
// looks identical to one the daemon would have produced. namespace
// must satisfy SnapshotMeta's pattern; sandboxID is the ref id the
// snapshot is "from" (only needs to be non-empty for validation).
func seedSnapshot(ctx context.Context, t *testing.T, h *harness, id, namespace, sandboxID string) {
	t.Helper()
	_, err := h.snapshot.CreateSnapshot(ctx, &cpv1.CreateSnapshotRequest{
		Snapshot: &cpv1.Snapshot{
			Metadata: &cpv1.SnapshotMeta{Id: id, Namespace: namespace, Description: "test"},
			Sandbox:  &cpv1.SandboxRef{Id: sandboxID},
		},
	})
	require.NoError(t, err)
}

func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "error is not a gRPC status: %v", err)
	assert.Equal(t, want, st.Code(), "message: %s", st.Message())
}

func TestCreateSandbox_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	resp, err := h.client.CreateSandbox(ctx, newCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, resp.GetSandbox())

	sb := resp.GetSandbox()
	assert.Equal(t, int64(1), sb.GetMetadata().GetVersion(),
		"the DB stamps the initial version on Create")
	require.NotNil(t, sb.GetMetadata().GetCreatedAt())
	require.NotNil(t, sb.GetMetadata().GetLastModifiedAt())
	assert.WithinDuration(t, time.Now(), sb.GetMetadata().GetCreatedAt().AsTime(), 5*time.Second)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PENDING, sb.GetStatus().GetPhase())
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sb.GetIntent().GetPhase())
	assert.Equal(t, defaultHarnessNodeID, sb.GetNode().GetId(),
		"the scheduler must assign the only seeded healthy node")

	// Read-back via GetSandbox confirms persistence.
	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-001"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(sb, got.GetSandbox()),
		"GetSandbox must round-trip the response from CreateSandbox")
}

func TestCreateSandbox_StatusForcedToPending(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	req := newCreateRequest(
		withID("sb-status"),
		withClientStatus(cpv1.SandboxStatus_PHASE_RUNNING),
	)
	resp, err := h.client.CreateSandbox(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, cpv1.SandboxStatus_PHASE_PENDING, resp.GetSandbox().GetStatus().GetPhase(),
		"server must overwrite client-supplied Status with PHASE_PENDING")
}

func TestCreateSandbox_IntentForcedToRunning(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	cases := []struct {
		name string
		opts []func(*cpv1.Sandbox)
	}{
		{"unset", nil},
		{"client sent PHASE_PAUSED", []func(*cpv1.Sandbox){withClientIntent(cpv1.SandboxStatus_PHASE_PAUSED)}},
		{"client sent PHASE_DELETING", []func(*cpv1.Sandbox){withClientIntent(cpv1.SandboxStatus_PHASE_DELETING)}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]func(*cpv1.Sandbox){withID(fmt.Sprintf("sb-intent-%d", i))}, tc.opts...)
			resp, err := h.client.CreateSandbox(ctx, newCreateRequest(opts...))
			require.NoError(t, err)
			assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, resp.GetSandbox().GetIntent().GetPhase(),
				"server must overwrite Intent.Phase with PHASE_RUNNING regardless of client input")
		})
	}
}

func TestCreateSandbox_ClientNodeReplacedByScheduler(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	req := newCreateRequest(
		withID("sb-node"),
		withClientNode("client-supplied-node"),
	)
	resp, err := h.client.CreateSandbox(ctx, req)
	require.NoError(t, err)
	// The client-supplied node id is discarded; the scheduler picks
	// the seeded harness node instead.
	assert.Equal(t, defaultHarnessNodeID, resp.GetSandbox().GetNode().GetId(),
		"server must replace client-supplied Node with the scheduler's choice")

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-node"})
	require.NoError(t, err)
	assert.Equal(t, defaultHarnessNodeID, got.GetSandbox().GetNode().GetId(),
		"stored row must carry the scheduler-chosen node, not the client-supplied one")
}

func TestCreateSandbox_NoHealthyNodes_ReturnsFailedPrecondition(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-no-nodes")))
	assertCode(t, err, codes.FailedPrecondition)
}

func TestCreateSandbox_TimestampsServerStamped(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	req := newCreateRequest(
		withID("sb-ts"),
		withClientCreatedAt(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
	)
	resp, err := h.client.CreateSandbox(ctx, req)
	require.NoError(t, err)

	createdAt := resp.GetSandbox().GetMetadata().GetCreatedAt().AsTime()
	assert.WithinDuration(t, time.Now(), createdAt, 5*time.Second,
		"server must overwrite client-supplied CreatedAt with the current time")
}

func TestCreateSandbox_IdempotentRetry(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	first, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-idem")))
	require.NoError(t, err)

	// Same logical request again. The DB layer recognises the matching
	// content digest and returns success without overwriting.
	second, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-idem")))
	require.NoError(t, err, "idempotent retry must succeed")

	assert.Equal(t, int64(1), first.GetSandbox().GetMetadata().GetVersion())
	assert.Equal(t, int64(1), second.GetSandbox().GetMetadata().GetVersion(),
		"version must not advance on idempotent retry")

	// The stored row reflects the FIRST write — not the retry's clock.
	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-idem"})
	require.NoError(t, err)
	assert.True(t,
		first.GetSandbox().GetMetadata().GetCreatedAt().AsTime().
			Equal(got.GetSandbox().GetMetadata().GetCreatedAt().AsTime()),
		"stored CreatedAt must come from the first Create, not the retry")
}

// TestCreateSandbox_IdempotentAcrossDifferentScheduledNodes pins the
// end-to-end behaviour of the digest fix: with two healthy candidates,
// the random scheduler may pick different nodes across two calls with
// the same sandbox id, but the second call must still be a no-op
// (idempotent return) and the stored Node must reflect the first
// attempt's choice.
func TestCreateSandbox_IdempotentAcrossDifferentScheduledNodes(t *testing.T) {
	h := startService(t) // also seeds defaultHarnessNodeID
	ctx := t.Context()

	// Add a second healthy node so the scheduler has a real choice
	// between two candidates on each call.
	require.NoError(t, h.nodeDB.Put(ctx, &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: "harness-node-2"},
		Resources: &cpv1.NodeResources{
			CpuCapacityMillicores: 8000,
			MemoryCapacityBytes:   16 * 1024 * 1024 * 1024,
			DiskCapacityBytes:     100 * 1024 * 1024 * 1024,
		},
		Status: &cpv1.NodeStatus{Phase: cpv1.NodeStatus_PHASE_HEALTHY},
	}))

	first, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-idem-multi")))
	require.NoError(t, err)
	firstNodeID := first.GetSandbox().GetNode().GetId()
	require.NotEmpty(t, firstNodeID, "first attempt must have a scheduled node")

	// The retry must not error. (Before the digest fix it would surface
	// as AlreadyExists whenever the scheduler picked a different node.)
	_, err = h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-idem-multi")))
	require.NoError(t, err,
		"a retry must succeed even when the scheduler picks a different node")

	// Authoritative assertion: the stored row reflects the FIRST attempt's
	// scheduling choice, not the retry's. Read back through GetSandbox
	// rather than the retry's response — the response carries the
	// handler's in-memory sb (post-scheduling on the retry), which is a
	// pre-existing inconsistency the digest fix does not change.
	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-idem-multi"})
	require.NoError(t, err)
	assert.Equal(t, firstNodeID, got.GetSandbox().GetNode().GetId(),
		"stored Node must reflect the first attempt's scheduled node")
	assert.Equal(t, int64(1), got.GetSandbox().GetMetadata().GetVersion(),
		"idempotent retry must not advance the stored version")
}

func TestCreateSandbox_ConflictOnDifferentBody(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-conflict"), withVCPU(2)))
	require.NoError(t, err)

	_, err = h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-conflict"), withVCPU(4)))
	assertCode(t, err, codes.AlreadyExists)
}

func TestCreateSandbox_ValidatesMissingSandbox(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx, &cpv1.CreateSandboxRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

func TestCreateSandbox_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("")))
	assertCode(t, err, codes.InvalidArgument)
}

func TestCreateSandbox_ValidatesNamespacePattern(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Uppercase letters are not allowed by the namespace regex.
	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withNamespace("Bad-NS")))
	assertCode(t, err, codes.InvalidArgument)
}

func TestCreateSandbox_ValidatesVcpuCountOutOfRange(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	cases := []struct {
		name string
		vcpu uint32
	}{
		{"too low", 0},
		{"too high", 33},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.client.CreateSandbox(ctx, newCreateRequest(
				withID("sb-vcpu-"+tc.name),
				withVCPU(tc.vcpu),
			))
			assertCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestCreateSandbox_ValidatesMemoryOutOfRange(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	cases := []struct {
		name   string
		memory uint64
	}{
		{"too low", 64},
		{"too high", 200000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.client.CreateSandbox(ctx, newCreateRequest(
				withID("sb-mem-"+tc.name),
				withMemory(tc.memory),
			))
			assertCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestCreateSandbox_ValidatesLabelKeyPattern(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Spaces and uppercase are not allowed in label keys.
	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withLabel("Bad Key", "v")))
	assertCode(t, err, codes.InvalidArgument)
}

func TestCreateSandbox_Source_SnapshotResolves(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	seedSnapshot(ctx, t, h, "snap-src-ok", validNamespace, "sb-src-origin")

	resp, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-from-snap"), withSnapshotSource("snap-src-ok")))
	require.NoError(t, err)

	assert.Equal(t, "snap-src-ok", resp.GetSandbox().GetMetadata().GetSource().GetSnapshotId(),
		"the response must round-trip the source reference unchanged")

	// Re-read via GetSandbox confirms the source persisted.
	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-from-snap"})
	require.NoError(t, err)
	assert.Equal(t, "snap-src-ok", got.GetSandbox().GetMetadata().GetSource().GetSnapshotId())
}

func TestCreateSandbox_Source_SnapshotMissing(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-from-missing"), withSnapshotSource("snap-missing")))
	assertCode(t, err, codes.NotFound)

	st, _ := status.FromError(err)
	assert.Contains(t, st.Message(), "snap-missing",
		"error must name the missing snapshot id")
	assert.Contains(t, st.Message(), "sandbox.metadata.source",
		"error must name the field that referenced the snapshot")
}

func TestCreateSandbox_Source_SnapshotNamespaceMismatch(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Seed the snapshot in a different namespace than the sandbox will
	// be created in. SnapshotMeta's namespace pattern matches the
	// sandbox pattern, so any valid namespace works as the "other" one.
	const otherNamespace = "team-beta"
	seedSnapshot(ctx, t, h, "snap-cross", otherNamespace, "sb-src-cross")

	_, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-cross"), withSnapshotSource("snap-cross")))
	assertCode(t, err, codes.FailedPrecondition)

	st, _ := status.FromError(err)
	assert.Contains(t, st.Message(), otherNamespace,
		"error must surface the snapshot's namespace")
	assert.Contains(t, st.Message(), validNamespace,
		"error must surface the sandbox's namespace")
}

func TestCreateSandbox_Source_ImageIdRejectedAsUnimplemented(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-image"), withImageSource("img-1")))
	assertCode(t, err, codes.Unimplemented)

	st, _ := status.FromError(err)
	assert.Contains(t, st.Message(), "img-1",
		"error must name the image id the client supplied")
}

// TestCreateSandbox_Source_EmptyReferenceIsTreatedAsUnsourced pins the
// behaviour when the caller serialises a SandboxSource with no oneof
// arm set. There is nothing to validate; the request must succeed and
// the (empty) source is preserved on the stored proto so a subsequent
// proto round-trip stays equivalent.
func TestCreateSandbox_Source_EmptyReferenceIsTreatedAsUnsourced(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	resp, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-empty-source"), func(sb *cpv1.Sandbox) {
			sb.Metadata.Source = &cpv1.SandboxSource{}
		}))
	require.NoError(t, err)
	assert.Nil(t, resp.GetSandbox().GetMetadata().GetSource().GetReference(),
		"an empty SandboxSource must round-trip with no reference set")
}

func TestCreateSandbox_Source_IdempotentRetry(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	seedSnapshot(ctx, t, h, "snap-idem", validNamespace, "sb-src-idem")

	first, err := h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-idem-src"), withSnapshotSource("snap-idem")))
	require.NoError(t, err)
	firstLMT := first.GetSandbox().GetMetadata().GetLastModifiedAt().AsTime()

	// Sleep so a non-idempotent server would stamp a strictly later
	// LastModifiedAt — if the retry still matches, we know the digest
	// folded into the existing row instead of writing a new one.
	time.Sleep(2 * time.Millisecond)

	_, err = h.client.CreateSandbox(ctx,
		newCreateRequest(withID("sb-idem-src"), withSnapshotSource("snap-idem")))
	require.NoError(t, err)

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-idem-src"})
	require.NoError(t, err)
	assert.True(t,
		firstLMT.Equal(got.GetSandbox().GetMetadata().GetLastModifiedAt().AsTime()),
		"idempotent retry with the same source must not advance LastModifiedAt")
}

func TestGetSandbox_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	created, err := h.client.CreateSandbox(ctx, newCreateRequest(withID("sb-get-1")))
	require.NoError(t, err)

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-get-1"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(created.GetSandbox(), got.GetSandbox()),
		"GetSandbox must round-trip the proto returned by CreateSandbox")
}

func TestGetSandbox_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "no-such-sandbox"})
	assertCode(t, err, codes.NotFound)
}

func TestGetSandbox_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

// createForTransition creates a sandbox and forces its saved phase, returning
// the version the caller should send on the subsequent Pause/Resume request.
func createForTransition(ctx context.Context, t *testing.T, h *harness, id string, savedPhase cpv1.SandboxStatus_Phase) int64 {
	t.Helper()
	resp, err := h.client.CreateSandbox(ctx, newCreateRequest(withID(id)))
	require.NoError(t, err)
	if savedPhase != cpv1.SandboxStatus_PHASE_PENDING {
		h.setSavedPhase(ctx, t, id, savedPhase)
	}
	return resp.GetSandbox().GetMetadata().GetVersion()
}

func TestPauseSandbox_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-pause-ok", cpv1.SandboxStatus_PHASE_RUNNING)

	createdAt, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-pause-ok"})
	require.NoError(t, err)
	originalLastModified := createdAt.GetSandbox().GetMetadata().GetLastModifiedAt().AsTime()

	resp, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{
		SandboxId: "sb-pause-ok",
		Version:   version,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSandbox(),
		"PauseSandboxResponse must carry the updated Sandbox")

	respSb := resp.GetSandbox()
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSING, respSb.GetStatus().GetPhase(),
		"Pause must set Status.Phase to PHASE_PAUSING (in-progress marker)")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, respSb.GetIntent().GetPhase(),
		"Pause must set Intent.Phase to PHASE_PAUSED")
	assert.Equal(t, int64(2), respSb.GetMetadata().GetVersion(),
		"successful Update must increment the stored version")
	assert.True(t,
		respSb.GetMetadata().GetLastModifiedAt().AsTime().After(originalLastModified),
		"transition must advance Metadata.LastModifiedAt")

	// Cross-check: response Sandbox matches what's stored. Kept on Pause
	// only (the cheapest of the three) as a guard against the in-process
	// db.Update mutation diverging from the row written to DynamoDB.
	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-pause-ok"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(respSb, got.GetSandbox()),
		"response Sandbox must round-trip through GetSandbox")
}

func TestPauseSandbox_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{
		SandboxId: "no-such-sandbox",
		Version:   1,
	})
	assertCode(t, err, codes.NotFound)
}

func TestPauseSandbox_VersionConflict(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	createForTransition(ctx, t, h, "sb-pause-stale", cpv1.SandboxStatus_PHASE_RUNNING)

	_, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{
		SandboxId: "sb-pause-stale",
		Version:   99, // stale
	})
	assertCode(t, err, codes.Aborted)
}

func TestPauseSandbox_InvalidPhaseTransition(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Saved phase remains PENDING — Pause requires RUNNING.
	version := createForTransition(ctx, t, h, "sb-pause-bad", cpv1.SandboxStatus_PHASE_PENDING)

	_, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{
		SandboxId: "sb-pause-bad",
		Version:   version,
	})
	assertCode(t, err, codes.FailedPrecondition)
}

func TestPauseSandbox_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{Version: 1})
	assertCode(t, err, codes.InvalidArgument)
}

func TestPauseSandbox_ValidatesMissingVersion(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{SandboxId: "sb-x"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestResumeSandbox_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-resume-ok", cpv1.SandboxStatus_PHASE_PAUSED)

	resp, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{
		SandboxId: "sb-resume-ok",
		Version:   version,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSandbox(),
		"ResumeSandboxResponse must carry the updated Sandbox")

	respSb := resp.GetSandbox()
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RESUMING, respSb.GetStatus().GetPhase(),
		"Resume must set Status.Phase to PHASE_RESUMING (in-progress marker)")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, respSb.GetIntent().GetPhase(),
		"Resume must set Intent.Phase to PHASE_RUNNING (PHASE_RESUMED no longer exists)")
	assert.Equal(t, int64(2), respSb.GetMetadata().GetVersion())
}

func TestResumeSandbox_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{
		SandboxId: "no-such-sandbox",
		Version:   1,
	})
	assertCode(t, err, codes.NotFound)
}

func TestResumeSandbox_VersionConflict(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	createForTransition(ctx, t, h, "sb-resume-stale", cpv1.SandboxStatus_PHASE_PAUSED)

	_, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{
		SandboxId: "sb-resume-stale",
		Version:   99,
	})
	assertCode(t, err, codes.Aborted)
}

func TestResumeSandbox_InvalidPhaseTransition(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Saved phase is PENDING — Resume requires PAUSED.
	version := createForTransition(ctx, t, h, "sb-resume-bad", cpv1.SandboxStatus_PHASE_PENDING)

	_, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{
		SandboxId: "sb-resume-bad",
		Version:   version,
	})
	assertCode(t, err, codes.FailedPrecondition)
}

func TestResumeSandbox_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{Version: 1})
	assertCode(t, err, codes.InvalidArgument)
}

func TestResumeSandbox_ValidatesMissingVersion(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ResumeSandbox(ctx, &cpv1.ResumeSandboxRequest{SandboxId: "sb-x"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestDeleteSandbox_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Delete is allowed from any saved phase. Cover the three alive ones to
	// confirm there's no precondition rejecting them.
	cases := []struct {
		name       string
		savedPhase cpv1.SandboxStatus_Phase
	}{
		{"from PENDING", cpv1.SandboxStatus_PHASE_PENDING},
		{"from RUNNING", cpv1.SandboxStatus_PHASE_RUNNING},
		{"from PAUSED", cpv1.SandboxStatus_PHASE_PAUSED},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("sb-delete-%d", i)
			version := createForTransition(ctx, t, h, id, tc.savedPhase)

			resp, err := h.client.DeleteSandbox(ctx, &cpv1.DeleteSandboxRequest{
				SandboxId: id,
				Version:   version,
			})
			require.NoError(t, err)
			require.NotNil(t, resp.GetSandbox(),
				"DeleteSandboxResponse must carry the updated Sandbox")

			respSb := resp.GetSandbox()
			assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETING, respSb.GetStatus().GetPhase(),
				"Delete must set Status.Phase to PHASE_DELETING (in-progress marker)")
			assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETED, respSb.GetIntent().GetPhase(),
				"Delete must set Intent.Phase to PHASE_DELETED (terminal target)")
			assert.Equal(t, int64(2), respSb.GetMetadata().GetVersion())
		})
	}
}

func TestDeleteSandbox_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.DeleteSandbox(ctx, &cpv1.DeleteSandboxRequest{
		SandboxId: "no-such-sandbox",
		Version:   1,
	})
	assertCode(t, err, codes.NotFound)
}

func TestDeleteSandbox_VersionConflict(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	createForTransition(ctx, t, h, "sb-delete-stale", cpv1.SandboxStatus_PHASE_RUNNING)

	_, err := h.client.DeleteSandbox(ctx, &cpv1.DeleteSandboxRequest{
		SandboxId: "sb-delete-stale",
		Version:   99,
	})
	assertCode(t, err, codes.Aborted)
}

func TestDeleteSandbox_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.DeleteSandbox(ctx, &cpv1.DeleteSandboxRequest{Version: 1})
	assertCode(t, err, codes.InvalidArgument)
}

func TestDeleteSandbox_ValidatesMissingVersion(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.DeleteSandbox(ctx, &cpv1.DeleteSandboxRequest{SandboxId: "sb-x"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestStartSnapshot_HappyPathFromRunning(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-snap-ok", cpv1.SandboxStatus_PHASE_RUNNING)

	created, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-snap-ok"})
	require.NoError(t, err)
	originalLastModified := created.GetSandbox().GetMetadata().GetLastModifiedAt().AsTime()

	resp, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId:   "sb-snap-ok",
		Version:     version,
		Description: "before-deploy",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSandbox(),
		"StartSnapshotResponse must carry the updated Sandbox")

	respSb := resp.GetSandbox()
	require.NotNil(t, respSb.GetIntent().GetStartSnapshot(),
		"Intent.StartSnapshot must be set so the reconciler picks it up")
	assert.Equal(t, "before-deploy", respSb.GetIntent().GetStartSnapshot().GetDescription())
	assert.Equal(t, cpv1.SandboxStatus_PHASE_SNAPSHOTTING, respSb.GetStatus().GetPhase(),
		"Status.Phase must flip to PHASE_SNAPSHOTTING (in-progress marker)")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, respSb.GetIntent().GetPhase(),
		"Intent.Phase must record the saved phase the reconciler should restore")
	assert.Equal(t, int64(2), respSb.GetMetadata().GetVersion(),
		"successful Update must increment the stored version")
	assert.True(t,
		respSb.GetMetadata().GetLastModifiedAt().AsTime().After(originalLastModified),
		"StartSnapshot must advance Metadata.LastModifiedAt")

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-snap-ok"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(respSb, got.GetSandbox()),
		"response Sandbox must round-trip through GetSandbox")
}

func TestStartSnapshot_HappyPathFromPaused(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-snap-paused", cpv1.SandboxStatus_PHASE_PAUSED)

	resp, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId: "sb-snap-paused",
		Version:   version,
	})
	require.NoError(t, err)
	respSb := resp.GetSandbox()

	require.NotNil(t, respSb.GetIntent().GetStartSnapshot())
	assert.Equal(t, cpv1.SandboxStatus_PHASE_SNAPSHOTTING, respSb.GetStatus().GetPhase())
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, respSb.GetIntent().GetPhase(),
		"snapshot of a paused sandbox must record PAUSED as the restore target")
}

func TestStartSnapshot_RejectsFromPending(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-snap-pending", cpv1.SandboxStatus_PHASE_PENDING)

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId: "sb-snap-pending",
		Version:   version,
	})
	assertCode(t, err, codes.FailedPrecondition)
}

// TestStartSnapshot_RejectsFromPausing covers the case a snapshot
// request arrives while a Pause is mid-flight (saved phase is the
// transient PAUSING). The DB state-machine guard accepts only
// RUNNING/PAUSED as priors for SNAPSHOTTING.
func TestStartSnapshot_RejectsFromPausing(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	version := createForTransition(ctx, t, h, "sb-snap-inflight", cpv1.SandboxStatus_PHASE_RUNNING)

	pauseResp, err := h.client.PauseSandbox(ctx, &cpv1.PauseSandboxRequest{
		SandboxId: "sb-snap-inflight",
		Version:   version,
	})
	require.NoError(t, err)

	_, err = h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId: "sb-snap-inflight",
		Version:   pauseResp.GetSandbox().GetMetadata().GetVersion(),
	})
	assertCode(t, err, codes.FailedPrecondition)
}

func TestStartSnapshot_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId: "no-such-sandbox",
		Version:   1,
	})
	assertCode(t, err, codes.NotFound)
}

func TestStartSnapshot_VersionConflict(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	createForTransition(ctx, t, h, "sb-snap-stale", cpv1.SandboxStatus_PHASE_RUNNING)

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId: "sb-snap-stale",
		Version:   99,
	})
	assertCode(t, err, codes.Aborted)
}

func TestStartSnapshot_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{Version: 1})
	assertCode(t, err, codes.InvalidArgument)
}

func TestStartSnapshot_ValidatesMissingVersion(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{SandboxId: "sb-x"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestStartSnapshot_ValidatesDescriptionTooLong(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.StartSnapshot(ctx, &cpv1.StartSnapshotRequest{
		SandboxId:   "sb-x",
		Version:     1,
		Description: strings.Repeat("a", 257), // exceeds buf.validate max_len = 256
	})
	assertCode(t, err, codes.InvalidArgument)
}

// listSandboxIDs creates a sandbox in the supplied namespace via the gRPC
// surface and returns its id. It exists so List tests can build fixtures
// through the same code path callers do.
func listFixtureViaGRPC(ctx context.Context, t *testing.T, h *harness, id, namespace string) {
	t.Helper()
	_, err := h.client.CreateSandbox(ctx, newCreateRequest(withID(id), withNamespace(namespace)))
	require.NoError(t, err)
}

func responseSandboxIDs(resp *cpv1.ListSandboxesResponse) []string {
	out := make([]string, len(resp.GetSandboxes()))
	for i, sb := range resp.GetSandboxes() {
		out[i] = sb.GetMetadata().GetId()
	}
	return out
}

func TestListSandboxes_HappyPath(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	listFixtureViaGRPC(ctx, t, h, "sb-list-a", "list-ns-happy")
	listFixtureViaGRPC(ctx, t, h, "sb-list-b", "list-ns-happy")
	// Different namespace — must NOT appear.
	listFixtureViaGRPC(ctx, t, h, "sb-list-other", "list-ns-other")

	resp, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace: "list-ns-happy",
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"sb-list-a", "sb-list-b"}, responseSandboxIDs(resp),
		"namespace-bounded list must include only the namespace's sandboxes")
}

func TestListSandboxes_AppliesDefaults(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	listFixtureViaGRPC(ctx, t, h, "sb-list-defaults", "list-ns-defaults")

	// Both PageSize and SortOrder unset; the handler must substitute the
	// documented defaults (30, NEWEST_FIRST) before calling the DB.
	resp, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace: "list-ns-defaults",
	})
	require.NoError(t, err, "unset PageSize/SortOrder must succeed via handler defaulting")
	assert.NotEmpty(t, resp.GetSandboxes())
}

func TestListSandboxes_ValidatesMissingFilters(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	// Neither namespace nor node_id set — the CEL message constraint must
	// reject this at the validation interceptor.
	_, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListSandboxes_ValidatesNamespacePattern(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace: "Bad-NS",
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	assertCode(t, err, codes.InvalidArgument)
}

// TestListSandboxes_NodeOnlyFilterAcceptsEmptyNamespace pins the fix for
// the controller's resync request, which omits namespace entirely and
// filters by node_id alone. Previously the field-level pattern fired on
// the empty string and rejected the request before it reached the
// handler; the namespace pattern now lives in a conditional message-level
// CEL rule that only enforces when the field is non-empty.
func TestListSandboxes_NodeOnlyFilterAcceptsEmptyNamespace(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	resp, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		NodeId:    "harness-node",
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestListSandboxes_ValidatesPageSizeUpperBound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace: "list-ns-pagesize",
		PageSize:  10_001,
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListSandboxes_RejectsGarbledContinuationToken(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace:         "list-ns-token",
		ContinuationToken: "this-is-not-base64-json",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListSandboxes_FiltersByPhase(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	listFixtureViaGRPC(ctx, t, h, "sb-list-phase-1", "list-ns-phase")
	listFixtureViaGRPC(ctx, t, h, "sb-list-phase-2", "list-ns-phase")
	// Force one of them to RUNNING via the test back-door so the phase
	// filter has something to discriminate. The CreateSandbox handler
	// always seeds PENDING, so we can't construct mixed phases through
	// the public surface alone.
	h.setSavedPhase(ctx, t, "sb-list-phase-2", cpv1.SandboxStatus_PHASE_RUNNING)

	resp, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace:   "list-ns-phase",
		StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"sb-list-phase-2"}, responseSandboxIDs(resp),
		"phase filter via the namespace+phase index must drop non-RUNNING rows")
}

func TestListSandboxes_EmptyResultIsOK(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	resp, err := h.client.ListSandboxes(ctx, &cpv1.ListSandboxesRequest{
		Namespace: "list-ns-empty",
	})
	require.NoError(t, err)
	assert.Empty(t, resp.GetSandboxes())
	assert.Equal(t, "", resp.GetContinuationToken())
}
