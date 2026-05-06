package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

// seedNode persists a node directly through the harness's nodeDB.
// Until a node-write RPC exists, this is the only path to populate
// node fixtures for read-side tests.
func seedNode(ctx context.Context, t *testing.T, h *harness, id string, phase cpv1.NodeStatus_Phase) *cpv1.Node {
	t.Helper()
	n := &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: id},
		Resources: &cpv1.NodeResources{
			VcpuCount: 8,
			MemoryMib: 16384,
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
	h := startService(t)
	ctx := t.Context()

	seeded := seedNode(ctx, t, h, "node-get-1", cpv1.NodeStatus_PHASE_HEALTHY)

	resp, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{NodeId: "node-get-1"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(seeded, resp.GetNode()),
		"GetNode must round-trip the stored node")
}

func TestGetNode_NotFound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{NodeId: "no-such-node"})
	assertCode(t, err, codes.NotFound)
}

func TestGetNode_ValidatesMissingId(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.cluster.GetNode(ctx, &cpv1.GetNodeRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListNodes_HappyPath(t *testing.T) {
	h := startService(t)
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
	h := startService(t)
	ctx := t.Context()

	seedNode(ctx, t, h, "node-defaults", cpv1.NodeStatus_PHASE_HEALTHY)

	// Both PageSize and SortOrder unset; the handler must substitute the
	// documented defaults (30, NEWEST_FIRST) before calling the DB.
	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{})
	require.NoError(t, err, "unset PageSize/SortOrder must succeed via handler defaulting")
	assert.NotEmpty(t, resp.GetNodes())
}

func TestListNodes_FiltersByPhase(t *testing.T) {
	h := startService(t)
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
	h := startService(t)
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
	h := startService(t)
	ctx := t.Context()

	resp, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetNodes())
	assert.Equal(t, "", resp.GetContinuationToken())
}

func TestListNodes_ValidatesPageSizeUpperBound(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
		PageSize: 10_001,
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListNodes_RejectsGarbledContinuationToken(t *testing.T) {
	h := startService(t)
	ctx := t.Context()

	_, err := h.cluster.ListNodes(ctx, &cpv1.ListNodesRequest{
		ContinuationToken: "this-is-not-a-valid-token!!!",
	})
	assertCode(t, err, codes.InvalidArgument)
}
