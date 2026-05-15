package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	dberrors "github.com/alan-ghelardi/yaghan/api-server/pkg/db"
	nodedb "github.com/alan-ghelardi/yaghan/api-server/pkg/db/node"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// listFixture creates and persists a node with the supplied id and
// phase. The DB stamps LastModifiedAt with wall-clock precision on
// every Put; sleeping a short interval after each call guarantees a
// strictly later timestamp on the next fixture so the gsi_lmt_sk
// ordering of list results is deterministic.
func listFixture(ctx context.Context, t *testing.T, db *dynamoDB, id string, phase cpv1.NodeStatus_Phase) *cpv1.Node {
	t.Helper()
	n := newFixture(withID(id), withPhase(phase))
	require.NoError(t, db.Put(ctx, n))
	time.Sleep(2 * time.Millisecond)
	return n
}

func ids(nodes []*cpv1.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.GetMetadata().GetId()
	}
	return out
}

func TestList_RejectsNonPositivePageSize(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  0,
		SortOrder: cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"page_size <= 0 must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsUnspecifiedSortOrder(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListNodesRequest_ORDER_UNSPECIFIED,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"ORDER_UNSPECIFIED must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_AllNodes_NewestFirst(t *testing.T) {
	db, ctx := setupDB(t)

	listFixture(ctx, t, db, "node-a", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-b", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-c", cpv1.NodeStatus_PHASE_HEALTHY)

	got, next, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, "", next, "page smaller than PageSize must clear the continuation token")
	assert.Equal(t, []string{"node-c", "node-b", "node-a"}, ids(got),
		"all_nodes_index must return every node, newest-first")
}

func TestList_AllNodes_OldestFirst(t *testing.T) {
	db, ctx := setupDB(t)

	listFixture(ctx, t, db, "node-a", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-b", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-c", cpv1.NodeStatus_PHASE_HEALTHY)

	got, _, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListNodesRequest_ORDER_OLDEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a", "node-b", "node-c"}, ids(got),
		"all_nodes_index must return every node, oldest-first")
}

func TestList_FilterByPhase_UsesPhaseIndex(t *testing.T) {
	db, ctx := setupDB(t)

	listFixture(ctx, t, db, "node-h-1", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-u-1", cpv1.NodeStatus_PHASE_UNHEALTHY)
	listFixture(ctx, t, db, "node-h-2", cpv1.NodeStatus_PHASE_HEALTHY)
	listFixture(ctx, t, db, "node-l-1", cpv1.NodeStatus_PHASE_LOST)

	got, _, err := db.List(ctx, nodedb.ListOptions{
		StatusPhase: cpv1.NodeStatus_PHASE_HEALTHY,
		PageSize:    10,
		SortOrder:   cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"node-h-2", "node-h-1"}, ids(got),
		"by_status_phase_index must scope to the requested phase")
}

func TestList_FilterByPhase_ReturnsEmptyForMissingPhase(t *testing.T) {
	db, ctx := setupDB(t)

	listFixture(ctx, t, db, "node-h-only", cpv1.NodeStatus_PHASE_HEALTHY)

	got, next, err := db.List(ctx, nodedb.ListOptions{
		StatusPhase: cpv1.NodeStatus_PHASE_LOST,
		PageSize:    10,
		SortOrder:   cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, "", next)
}

func TestList_PaginatesAcrossPages(t *testing.T) {
	db, ctx := setupDB(t)

	const total = 5
	for i := 0; i < total; i++ {
		listFixture(ctx, t, db, fmt.Sprintf("node-pg-%02d", i), cpv1.NodeStatus_PHASE_HEALTHY)
	}

	var collected []string
	var token string
	for page := 0; page < total+2; page++ { // safety bound
		got, next, err := db.List(ctx, nodedb.ListOptions{
			PageSize:          2,
			SortOrder:         cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
			ContinuationToken: token,
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(got), 2, "page must not exceed PageSize")
		collected = append(collected, ids(got)...)
		if next == "" {
			break
		}
		token = next
	}

	assert.Equal(t,
		[]string{"node-pg-04", "node-pg-03", "node-pg-02", "node-pg-01", "node-pg-00"},
		collected,
		"pagination must traverse rows newest-first with no duplicates or gaps")
}

func TestList_ContinuationTokenRejectedOnIndexMismatch(t *testing.T) {
	db, ctx := setupDB(t)

	for i := 0; i < 3; i++ {
		listFixture(ctx, t, db, fmt.Sprintf("node-cti-%02d", i), cpv1.NodeStatus_PHASE_HEALTHY)
	}

	// First page issued against all_nodes_index.
	_, token, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  1,
		SortOrder: cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.NotEmpty(t, token, "expected a continuation token after the first page")

	// Reuse the token against by_status_phase_index. The embedded index
	// name no longer matches and the token must be rejected before
	// issuing a query that DynamoDB would otherwise reject with a less
	// friendly error.
	_, _, err = db.List(ctx, nodedb.ListOptions{
		StatusPhase:       cpv1.NodeStatus_PHASE_HEALTHY,
		PageSize:          1,
		SortOrder:         cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: token,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"token reuse across indexes must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_ContinuationTokenRejectedOnGarbledInput(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, nodedb.ListOptions{
		PageSize:          1,
		SortOrder:         cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: "this-is-not-a-valid-token!!!",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"garbled token must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_RoundTripsNodeProto(t *testing.T) {
	db, ctx := setupDB(t)

	original := listFixture(ctx, t, db, "node-rt", cpv1.NodeStatus_PHASE_HEALTHY)

	got, _, err := db.List(ctx, nodedb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListNodesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, proto.Equal(original, got[0]),
		"List must return the same node proto Get would")
}

func TestScanIndexForward_MapsSortOrder(t *testing.T) {
	t.Run("newest-first descends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListNodesRequest_ORDER_NEWEST_FIRST)
		require.NoError(t, err)
		assert.False(t, got)
	})
	t.Run("oldest-first ascends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListNodesRequest_ORDER_OLDEST_FIRST)
		require.NoError(t, err)
		assert.True(t, got)
	})
	t.Run("unspecified is rejected", func(t *testing.T) {
		_, err := scanIndexForward(cpv1.ListNodesRequest_ORDER_UNSPECIFIED)
		require.Error(t, err)
		assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions))
	})
}

func TestSelectIndex_PicksTheRightGSI(t *testing.T) {
	cases := []struct {
		name      string
		opts      nodedb.ListOptions
		wantIndex string
		wantHK    string
		wantValue string
	}{
		{
			name:      "no phase filter",
			opts:      nodedb.ListOptions{},
			wantIndex: indexAllNodes,
			wantHK:    attrGSIConstHK,
			wantValue: allNodesPartition,
		},
		{
			name:      "with phase filter",
			opts:      nodedb.ListOptions{StatusPhase: cpv1.NodeStatus_PHASE_HEALTHY},
			wantIndex: indexByStatusPhase,
			wantHK:    attrNodeStatusPhase,
			wantValue: "PHASE_HEALTHY",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotIndex, gotHK, gotValue := selectIndex(tc.opts)
			assert.Equal(t, tc.wantIndex, gotIndex)
			assert.Equal(t, tc.wantHK, gotHK)
			assert.Equal(t, tc.wantValue, gotValue)
		})
	}
}

func TestContinuationToken_RoundTrip(t *testing.T) {
	original := map[string]types.AttributeValue{
		"node_id":    &types.AttributeValueMemberS{Value: "node-abc"},
		attrGSILmtSK: &types.AttributeValueMemberS{Value: "2026-05-06T10:00:00Z#node-abc"},
	}

	token, err := encodeContinuationToken(indexAllNodes, original)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	decoded, err := decodeContinuationToken(token, indexAllNodes)
	require.NoError(t, err)
	assert.Equal(t, len(original), len(decoded))
	for k, v := range original {
		want := v.(*types.AttributeValueMemberS).Value
		got, ok := decoded[k].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, want, got.Value)
	}
}

func TestContinuationToken_EmptyOnEmptyLEK(t *testing.T) {
	token, err := encodeContinuationToken(indexAllNodes, nil)
	require.NoError(t, err)
	assert.Equal(t, "", token)
}
