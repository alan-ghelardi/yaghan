package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// seedNode persists a node directly through the harness's nodeDB. The DB
// stamps version, created_at, and last_modified_at on success.
func seedNode(ctx context.Context, t *testing.T, h *harness, id string, phase cpv1.NodeStatus_Phase) *cpv1.Node {
	t.Helper()
	n := &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: id},
		Resources: &cpv1.NodeResources{
			CpuCapacityMillicores: 8000,
			MemoryCapacityBytes:   16 * 1024 * 1024 * 1024,
			DiskCapacityBytes:     100 * 1024 * 1024 * 1024,
		},
		Status: &cpv1.NodeStatus{Phase: phase},
	}
	require.NoError(t, h.nodeDB.Put(ctx, n))
	return n
}

func nodeIDs(resp *cpv1.ListNodesResponse) []string {
	out := make([]string, len(resp.GetNodes()))
	for i, n := range resp.GetNodes() {
		out[i] = n.GetMetadata().GetId()
	}
	return out
}

func TestGetNode_HappyPath(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	seeded := seedNode(ctx, t, h, "node-get-1", cpv1.NodeStatus_PHASE_HEALTHY)

	resp, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{NodeId: "node-get-1"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(seeded, resp.GetNode()),
		"GetNode must round-trip the stored node")
}

func TestGetNode_NotFound(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{NodeId: "no-such-node"})
	assertCode(t, err, codes.NotFound)
}

func TestGetNode_ValidatesMissingId(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListNodes_HappyPath(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	seedNode(ctx, t, h, "node-list-1", cpv1.NodeStatus_PHASE_HEALTHY)
	seedNode(ctx, t, h, "node-list-2", cpv1.NodeStatus_PHASE_HEALTHY)
	seedNode(ctx, t, h, "node-list-3", cpv1.NodeStatus_PHASE_HEALTHY)

	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{})
	require.NoError(t, err)
	// Default sort is newest-first; the third node was written last.
	assert.ElementsMatch(t,
		[]string{"node-list-1", "node-list-2", "node-list-3"},
		nodeIDs(resp),
		"ListNodes must include every node when no filter is set")
}

func TestListNodes_AppliesDefaults(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	seedNode(ctx, t, h, "node-defaults", cpv1.NodeStatus_PHASE_HEALTHY)

	// Both PageSize and SortOrder unset; the handler must substitute the
	// documented defaults (30, NEWEST_FIRST) before calling the DB.
	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{})
	require.NoError(t, err, "unset PageSize/SortOrder must succeed via handler defaulting")
	assert.NotEmpty(t, resp.GetNodes())
}

func TestListNodes_FiltersByPhase(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	seedNode(ctx, t, h, "node-h", cpv1.NodeStatus_PHASE_HEALTHY)
	seedNode(ctx, t, h, "node-u", cpv1.NodeStatus_PHASE_UNHEALTHY)
	seedNode(ctx, t, h, "node-l", cpv1.NodeStatus_PHASE_LOST)

	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
		StatusPhase: cpv1.NodeStatus_PHASE_UNHEALTHY,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"node-u"}, nodeIDs(resp),
		"phase filter must restrict results to the requested phase")
}

func TestListNodes_PaginatesAcrossPages(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	const total = 5
	want := make([]string, 0, total)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("node-pg-%02d", i)
		want = append(want, id)
		seedNode(ctx, t, h, id, cpv1.NodeStatus_PHASE_HEALTHY)
	}

	var collected []string
	var token string
	for page := 0; page < total+2; page++ { // safety bound
		resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
			PageSize:          2,
			SortOrder:         cpv1.ListNodesRequest_ORDER_OLDEST_FIRST,
			ContinuationToken: token,
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(resp.GetNodes()), 2, "page must not exceed PageSize")
		collected = append(collected, nodeIDs(resp)...)
		if resp.GetContinuationToken() == "" {
			break
		}
		token = resp.GetContinuationToken()
	}
	assert.Equal(t, want, collected,
		"pagination must traverse rows oldest-first with no duplicates or gaps")
}

func TestListNodes_EmptyResultIsOK(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetNodes())
	assert.Equal(t, "", resp.GetContinuationToken())
}

func TestListNodes_ValidatesPageSizeUpperBound(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
		PageSize: 10_001,
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListNodes_RejectsGarbledContinuationToken(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
		ContinuationToken: "this-is-not-a-valid-token!!!",
	})
	assertCode(t, err, codes.InvalidArgument)
}

// connectWithNode opens an EstablishSession stream, sends a ConnectionRequest
// carrying the supplied node, and waits for the acknowledgement. Useful when
// tests need to control the full Node payload (resources, status, metrics).
func connectWithNode(ctx context.Context, t *testing.T, h *harness, node *cpv1.Node) (cpv1.ClusterService_EstablishSessionClient, int64) {
	t.Helper()
	stream, err := h.cluster.EstablishSession(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&cpv1.EstablishSessionRequest{
		Operation: &cpv1.EstablishSessionRequest_Connect{
			Connect: &cpv1.ConnectionRequest{Node: node},
		},
	}))

	resp, err := stream.Recv()
	require.NoError(t, err, "expected ConnectionResponse acknowledgement")
	ack := resp.GetAcknowledge()
	require.NotNil(t, ack, "first response must be an acknowledgement, got %T", resp.GetMessage())
	require.Positive(t, ack.GetSessionId())
	return stream, ack.GetSessionId()
}

// closeAndDrain half-closes the client side of the stream and reads until the
// server flushes any in-flight responses and closes its end. After this
// returns, all server-side request handlers have completed and the persisted
// state is safe to assert against.
func closeAndDrain(t *testing.T, stream cpv1.ClusterService_EstablishSessionClient) {
	t.Helper()
	require.NoError(t, stream.CloseSend())
	for {
		_, err := stream.Recv()
		if err != nil {
			require.True(t, errors.Is(err, io.EOF) || isCancelled(err),
				"stream should close cleanly after CloseSend, got %v", err)
			return
		}
	}
}

func TestEstablishSession_RegisterNodeCreatesOnFirstConnect(t *testing.T) {
	h := startService(t, withEventStream())
	ctx := t.Context()

	stream, _ := connectAs(ctx, t, h, "node-fresh", 0)
	defer func() { _ = stream.CloseSend() }()

	got, err := h.nodeDB.Get(ctx, "node-fresh")
	require.NoError(t, err, "the row must be persisted by the time the ack returns")

	assert.Equal(t, "node-fresh", got.GetMetadata().GetId())
	assert.Equal(t, int64(1), got.GetMetadata().GetVersion(),
		"first registration must land at version 1")
	assert.NotNil(t, got.GetMetadata().GetCreatedAt(), "DB must stamp created_at on create")
	assert.NotNil(t, got.GetMetadata().GetLastModifiedAt())

	// Resources should mirror what connectAs sent.
	assert.Equal(t, uint32(4000), got.GetResources().GetCpuCapacityMillicores())
	assert.Equal(t, uint64(8*1024*1024*1024), got.GetResources().GetMemoryCapacityBytes())
	assert.Equal(t, uint64(100*1024*1024*1024), got.GetResources().GetDiskCapacityBytes())
}

func TestEstablishSession_RegisterNodeOverlaysOnReconnect(t *testing.T) {
	h := startService(t, withEventStream())
	ctx := t.Context()

	// Pre-seed via the DB; this stamps version=1 and created_at.
	seeded := seedNode(ctx, t, h, "node-reconnect", cpv1.NodeStatus_PHASE_HEALTHY)
	originalCreatedAt := seeded.GetMetadata().GetCreatedAt().AsTime()
	require.False(t, originalCreatedAt.IsZero(), "fixture must have a stamped created_at")

	// Connect with a payload that differs from the seed in every
	// daemon-owned field. The server must overlay these and bump version,
	// while preserving created_at.
	reconnectPayload := &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: "node-reconnect"},
		Resources: &cpv1.NodeResources{
			CpuCapacityMillicores: 16000,
			MemoryCapacityBytes:   32 * 1024 * 1024 * 1024,
			DiskCapacityBytes:     200 * 1024 * 1024 * 1024,
		},
		Status: &cpv1.NodeStatus{
			Phase:   cpv1.NodeStatus_PHASE_UNHEALTHY,
			Message: "test impairment",
		},
		Metrics: &cpv1.NodeMetrics{
			SampledAt:          timestamppb.Now(),
			ActiveSandboxCount: 7,
			CpuUsedMillicores:  1234,
			MemoryUsedBytes:    5 * 1024 * 1024 * 1024,
			DiskUsedBytes:      10 * 1024 * 1024 * 1024,
		},
		ProviderMetadata: &cpv1.Node_AwsEc2{AwsEc2: &cpv1.EC2InstanceMeta{
			InstanceId: "i-1234567890abcdef",
			Region:     "us-east-1",
		}},
	}

	stream, _ := connectWithNode(ctx, t, h, reconnectPayload)
	defer func() { _ = stream.CloseSend() }()

	got, err := h.nodeDB.Get(ctx, "node-reconnect")
	require.NoError(t, err)

	assert.Equal(t, int64(2), got.GetMetadata().GetVersion(),
		"reconnect must bump version exactly once")
	assert.True(t, originalCreatedAt.Equal(got.GetMetadata().GetCreatedAt().AsTime()),
		"created_at must be preserved across reconnects")

	// All daemon-owned fields are overlaid.
	assert.Equal(t, uint32(16000), got.GetResources().GetCpuCapacityMillicores())
	assert.Equal(t, uint64(32*1024*1024*1024), got.GetResources().GetMemoryCapacityBytes())
	assert.Equal(t, cpv1.NodeStatus_PHASE_UNHEALTHY, got.GetStatus().GetPhase())
	assert.Equal(t, "test impairment", got.GetStatus().GetMessage())
	assert.Equal(t, uint32(7), got.GetMetrics().GetActiveSandboxCount())
	assert.Equal(t, "i-1234567890abcdef", got.GetAwsEc2().GetInstanceId())
}

func TestEstablishSession_PatchNodeMetricsUpdatesOnlyMetrics(t *testing.T) {
	h := startService(t, withEventStream())
	ctx := t.Context()

	stream, _ := connectAs(ctx, t, h, "node-patch-metrics", 0)
	created, err := h.nodeDB.Get(ctx, "node-patch-metrics")
	require.NoError(t, err)
	priorVersion := created.GetMetadata().GetVersion()

	patchedMetrics := &cpv1.NodeMetrics{
		SampledAt:          timestamppb.Now(),
		ActiveSandboxCount: 3,
		CpuUsedMillicores:  2500,
		MemoryUsedBytes:    2 * 1024 * 1024 * 1024,
		DiskUsedBytes:      4 * 1024 * 1024 * 1024,
	}
	require.NoError(t, stream.Send(&cpv1.EstablishSessionRequest{
		Operation: &cpv1.EstablishSessionRequest_PatchNode{
			PatchNode: &cpv1.PatchNodeRequest{
				Patch: &cpv1.PatchNodeRequest_NodeMetrics{NodeMetrics: patchedMetrics},
			},
		},
	}))

	closeAndDrain(t, stream)

	got, err := h.nodeDB.Get(ctx, "node-patch-metrics")
	require.NoError(t, err)
	assert.Equal(t, priorVersion+1, got.GetMetadata().GetVersion(),
		"patch must bump version exactly once")
	assert.True(t, proto.Equal(patchedMetrics, got.GetMetrics()),
		"persisted metrics must equal the patch payload")
	assert.True(t, proto.Equal(created.GetResources(), got.GetResources()),
		"resources must remain untouched by a metrics-only patch")
}

func TestEstablishSession_PatchNodeStatusUpdatesOnlyStatus(t *testing.T) {
	h := startService(t, withEventStream())
	ctx := t.Context()

	stream, _ := connectAs(ctx, t, h, "node-patch-status", 0)
	created, err := h.nodeDB.Get(ctx, "node-patch-status")
	require.NoError(t, err)
	priorVersion := created.GetMetadata().GetVersion()

	patchedStatus := &cpv1.NodeStatus{
		Phase:   cpv1.NodeStatus_PHASE_UNHEALTHY,
		Message: "ebs check failed",
	}
	require.NoError(t, stream.Send(&cpv1.EstablishSessionRequest{
		Operation: &cpv1.EstablishSessionRequest_PatchNode{
			PatchNode: &cpv1.PatchNodeRequest{
				Patch: &cpv1.PatchNodeRequest_NodeStatus{NodeStatus: patchedStatus},
			},
		},
	}))

	closeAndDrain(t, stream)

	got, err := h.nodeDB.Get(ctx, "node-patch-status")
	require.NoError(t, err)
	assert.Equal(t, priorVersion+1, got.GetMetadata().GetVersion())
	assert.True(t, proto.Equal(patchedStatus, got.GetStatus()),
		"persisted status must equal the patch payload")
	assert.True(t, proto.Equal(created.GetResources(), got.GetResources()),
		"resources must remain untouched by a status-only patch")
}

func TestEstablishSession_PatchNodeWithEmptyPatchReturnsInBandError(t *testing.T) {
	h := startService(t, withEventStream())
	ctx := t.Context()

	stream, _ := connectAs(ctx, t, h, "node-patch-empty", 0)
	defer func() { _ = stream.CloseSend() }()

	// Neither node_metrics nor node_status set on the oneof.
	require.NoError(t, stream.Send(&cpv1.EstablishSessionRequest{
		Operation: &cpv1.EstablishSessionRequest_PatchNode{
			PatchNode: &cpv1.PatchNodeRequest{},
		},
	}))

	resp := recvError(t, stream)
	assert.Equal(t, int32(codes.InvalidArgument), resp.GetError().GetCode(),
		"empty PatchNodeRequest must surface as InvalidArgument")

	// The row must be untouched by the rejected patch.
	got, err := h.nodeDB.Get(ctx, "node-patch-empty")
	require.NoError(t, err)
	assert.Equal(t, int64(1), got.GetMetadata().GetVersion(),
		"a rejected patch must not bump version")
}
