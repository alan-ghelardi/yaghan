package service_test

import (
	"context"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.nuinfra.api-server/pkg/config"
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

// validNamespace satisfies the post-fix regex
// `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`.
const validNamespace = "team-alpha"

type harness struct {
	client    cpv1.SandboxServiceClient
	cluster   cpv1.ClusterServiceClient
	ddb       awsdynamodb.Client
	tableName string

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
}

type harnessOption func(*harnessConfig)

// withEventStream enables a redis-backed WatchableStream on the harness so
// EstablishSession can register watchers and the test can publish events via
// harness.db.Update(...).
func withEventStream() harnessOption { //nolint:unused // used by cluster_test.go
	return func(c *harnessConfig) { c.withEventStream = true }
}

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

	_, err = h.ddb.UpdateItem(ctx, &dynamodbservice.UpdateItemInput{
		TableName: awssdk.String(h.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"sandbox_id": &ddbtypes.AttributeValueMemberS{Value: id},
		},
		UpdateExpression: awssdk.String("SET #p = :p, #pb = :pb"),
		ExpressionAttributeNames: map[string]string{
			"#p":  "sandbox_status_phase",
			"#pb": "sandbox_pb",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":p":  &ddbtypes.AttributeValueMemberS{Value: phase.String()},
			":pb": &ddbtypes.AttributeValueMemberB{Value: updatedPB},
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

	endpoint := awstesting.StartEmulator(t)
	tableName := awstesting.CreateTable(t, endpoint, sandboxesSchemaPath)

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
		ddb:       ddbClient,
		tableName: tableName,
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
	assert.Nil(t, sb.GetNode())

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

func TestCreateSandbox_NodeZeroedOnCreate(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	req := newCreateRequest(
		withID("sb-node"),
		withClientNode("node-1"),
	)
	resp, err := h.client.CreateSandbox(ctx, req)
	require.NoError(t, err)
	assert.Nil(t, resp.GetSandbox().GetNode(), "Node must be zeroed on Create")

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-node"})
	require.NoError(t, err)
	assert.Nil(t, got.GetSandbox().GetNode(), "stored row must not carry the client-supplied Node")
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
	require.NotNil(t, resp)

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-pause-ok"})
	require.NoError(t, err)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSING, got.GetSandbox().GetStatus().GetPhase(),
		"Pause must set Status.Phase to PHASE_PAUSING (in-progress marker)")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, got.GetSandbox().GetIntent().GetPhase(),
		"Pause must set Intent.Phase to PHASE_PAUSED")
	assert.Equal(t, int64(2), got.GetSandbox().GetMetadata().GetVersion(),
		"successful Update must increment the stored version")
	assert.True(t,
		got.GetSandbox().GetMetadata().GetLastModifiedAt().AsTime().After(originalLastModified),
		"transition must advance Metadata.LastModifiedAt")
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
	require.NotNil(t, resp)

	got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: "sb-resume-ok"})
	require.NoError(t, err)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RESUMING, got.GetSandbox().GetStatus().GetPhase(),
		"Resume must set Status.Phase to PHASE_RESUMING (in-progress marker)")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, got.GetSandbox().GetIntent().GetPhase(),
		"Resume must set Intent.Phase to PHASE_RUNNING (PHASE_RESUMED no longer exists)")
	assert.Equal(t, int64(2), got.GetSandbox().GetMetadata().GetVersion())
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
			require.NotNil(t, resp)

			got, err := h.client.GetSandbox(ctx, &cpv1.GetSandboxRequest{SandboxId: id})
			require.NoError(t, err)
			assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETING, got.GetSandbox().GetStatus().GetPhase(),
				"Delete must set Status.Phase to PHASE_DELETING (in-progress marker)")
			assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETED, got.GetSandbox().GetIntent().GetPhase(),
				"Delete must set Intent.Phase to PHASE_DELETED (terminal target)")
			assert.Equal(t, int64(2), got.GetSandbox().GetMetadata().GetVersion())
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
