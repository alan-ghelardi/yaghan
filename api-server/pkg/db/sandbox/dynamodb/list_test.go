package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dberrors "golang.nuinfra.api-server/pkg/db"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// listFixture creates and persists a sandbox with the supplied id, namespace,
// node, and phase. The DB stamps LastModifiedAt with wall-clock precision on
// every Create; sleeping a short interval after each call guarantees a
// strictly later timestamp on the next fixture so the gsi_lmt_sk-ordered
// list results are deterministic across rows.
func listFixture(ctx context.Context, t *testing.T, db *dynamoDB, id, namespace, nodeID string, phase cpv1.SandboxStatus_Phase) *cpv1.Sandbox {
	t.Helper()
	sb := newFixture(withID(id))
	sb.Metadata.Namespace = namespace
	sb.Status.Phase = phase
	if nodeID != "" {
		sb.Node = &cpv1.NodeRef{Id: nodeID}
	}
	require.NoError(t, db.Create(ctx, sb))
	time.Sleep(2 * time.Millisecond)
	return sb
}

func ids(sandboxes []*cpv1.Sandbox) []string {
	out := make([]string, len(sandboxes))
	for i, sb := range sandboxes {
		out[i] = sb.GetMetadata().GetId()
	}
	return out
}

func TestList_RejectsUnboundedQuery(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, sandboxdb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"missing namespace and node_id must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsNonPositivePageSize(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  0,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"page_size <= 0 must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsUnspecifiedSortOrder(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_UNSPECIFIED,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"ORDER_UNSPECIFIED must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_ByNamespace_ReturnsAllRowsInNamespace(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "sb-bn-a", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bn-b", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bn-c", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	// Different namespace — must NOT appear.
	listFixture(ctx, t, db, "sb-bn-other", "team-beta", "", cpv1.SandboxStatus_PHASE_PENDING)

	got, next, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: ns,
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, "", next, "page smaller than PageSize must clear the continuation token")
	assert.Equal(t, []string{"sb-bn-c", "sb-bn-b", "sb-bn-a"}, ids(got),
		"by_namespace_index must include every sandbox in the namespace, newest-first")
}

func TestList_ByNamespace_NewestFirst(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "sb-nf-a", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-nf-b", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-nf-c", ns, "", cpv1.SandboxStatus_PHASE_PENDING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: ns,
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-nf-c", "sb-nf-b", "sb-nf-a"}, ids(got),
		"ORDER_NEWEST_FIRST must return rows in descending gsi_lmt_sk order")
}

func TestList_ByNamespace_OldestFirst(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "sb-of-a", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-of-b", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-of-c", ns, "", cpv1.SandboxStatus_PHASE_PENDING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: ns,
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_OLDEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-of-a", "sb-of-b", "sb-of-c"}, ids(got),
		"ORDER_OLDEST_FIRST must return rows in ascending gsi_lmt_sk order")
}

func TestList_ByNamespaceAndPhase_FiltersByPhase(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "sb-bnp-pending-1", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bnp-running-1", ns, "", cpv1.SandboxStatus_PHASE_RUNNING)
	listFixture(ctx, t, db, "sb-bnp-pending-2", ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bnp-running-2", ns, "", cpv1.SandboxStatus_PHASE_RUNNING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace:   ns,
		StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING,
		PageSize:    10,
		SortOrder:   cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-bnp-running-2", "sb-bnp-running-1"}, ids(got),
		"phase filter must restrict results to the requested phase, newest-first")
}

func TestList_ByNamespaceAndNode_ScopesToNamespaceAndNode(t *testing.T) {
	db, ctx := setupDB(t)
	const (
		ns   = "team-alpha"
		node = "node-1"
	)

	listFixture(ctx, t, db, "sb-bnn-1a", ns, node, cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bnn-2", ns, "node-2", cpv1.SandboxStatus_PHASE_PENDING)
	listFixture(ctx, t, db, "sb-bnn-1b", ns, node, cpv1.SandboxStatus_PHASE_PENDING)
	// Same node, different namespace — must NOT appear.
	listFixture(ctx, t, db, "sb-bnn-other", "team-beta", node, cpv1.SandboxStatus_PHASE_PENDING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: ns,
		NodeID:    node,
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-bnn-1b", "sb-bnn-1a"}, ids(got),
		"by_namespace_node_index must scope to the namespace+node tuple, newest-first")
}

func TestList_ByNamespaceNodeAndPhase_AppliesInAppPhaseFilter(t *testing.T) {
	db, ctx := setupDB(t)
	const (
		ns   = "team-alpha"
		node = "node-1"
	)

	listFixture(ctx, t, db, "sb-bnnp-running", ns, node, cpv1.SandboxStatus_PHASE_RUNNING)
	listFixture(ctx, t, db, "sb-bnnp-paused", ns, node, cpv1.SandboxStatus_PHASE_PAUSED)
	listFixture(ctx, t, db, "sb-bnnp-pending", ns, node, cpv1.SandboxStatus_PHASE_PENDING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace:   ns,
		NodeID:      node,
		StatusPhase: cpv1.SandboxStatus_PHASE_PAUSED,
		PageSize:    10,
		SortOrder:   cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-bnnp-paused"}, ids(got),
		"in-app filter must drop non-matching phases")
}

func TestList_ByNode_CrossesNamespaces(t *testing.T) {
	db, ctx := setupDB(t)
	const node = "node-1"

	listFixture(ctx, t, db, "sb-bnode-a", "team-alpha", node, cpv1.SandboxStatus_PHASE_RUNNING)
	listFixture(ctx, t, db, "sb-bnode-b", "team-beta", node, cpv1.SandboxStatus_PHASE_RUNNING)
	listFixture(ctx, t, db, "sb-bnode-c", "team-alpha", "node-2", cpv1.SandboxStatus_PHASE_RUNNING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		NodeID:    node,
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_OLDEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-bnode-a", "sb-bnode-b"}, ids(got),
		"by_node_index must span namespaces; sb-bnode-c on a different node must be excluded")
}

func TestList_ByNodeAndPhase_AppliesInAppPhaseFilter(t *testing.T) {
	db, ctx := setupDB(t)
	const node = "node-1"

	listFixture(ctx, t, db, "sb-bnp-r-1", "team-alpha", node, cpv1.SandboxStatus_PHASE_RUNNING)
	listFixture(ctx, t, db, "sb-bnp-p-1", "team-alpha", node, cpv1.SandboxStatus_PHASE_PAUSED)
	listFixture(ctx, t, db, "sb-bnp-r-2", "team-beta", node, cpv1.SandboxStatus_PHASE_RUNNING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		NodeID:      node,
		StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING,
		PageSize:    10,
		SortOrder:   cpv1.ListSandboxesRequest_ORDER_OLDEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sb-bnp-r-1", "sb-bnp-r-2"}, ids(got),
		"in-app filter must keep only RUNNING and span namespaces, oldest-first")
}

func TestList_EmptyResultSet(t *testing.T) {
	db, ctx := setupDB(t)

	got, next, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, "", next)
}

func TestList_PaginatesAcrossPages(t *testing.T) {
	db, ctx := setupDB(t)
	const (
		ns    = "team-alpha"
		total = 5
	)

	for i := 0; i < total; i++ {
		listFixture(ctx, t, db, fmt.Sprintf("sb-pg-%02d", i), ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	}

	var collected []string
	var token string
	for page := 0; page < total+2; page++ { // safety bound
		got, next, err := db.List(ctx, sandboxdb.ListOptions{
			Namespace:         ns,
			PageSize:          2,
			SortOrder:         cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
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
		[]string{"sb-pg-04", "sb-pg-03", "sb-pg-02", "sb-pg-01", "sb-pg-00"},
		collected,
		"pagination must traverse rows newest-first with no duplicates or gaps")
}

func TestList_ContinuationTokenRejectedOnIndexMismatch(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	for i := 0; i < 3; i++ {
		listFixture(ctx, t, db, fmt.Sprintf("sb-cti-%02d", i), ns, "", cpv1.SandboxStatus_PHASE_PENDING)
	}

	// First page issued against by_namespace_index.
	_, token, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: ns,
		PageSize:  1,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.NotEmpty(t, token, "expected a continuation token after the first page")

	// Reuse against by_status_phase_index: the embedded index name no
	// longer matches and the token must be rejected before issuing a
	// query that DynamoDB would reject with a less friendly error.
	_, _, err = db.List(ctx, sandboxdb.ListOptions{
		Namespace:         ns,
		StatusPhase:       cpv1.SandboxStatus_PHASE_PENDING,
		PageSize:          1,
		SortOrder:         cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: token,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"token reuse across indexes must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_ContinuationTokenRejectedOnGarbledInput(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace:         "team-alpha",
		PageSize:          1,
		SortOrder:         cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: "this-is-not-a-valid-token!!!",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"garbled token must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_RoundTripsSandboxProto(t *testing.T) {
	db, ctx := setupDB(t)

	original := listFixture(ctx, t, db, "sb-rt", "team-alpha", "node-1", cpv1.SandboxStatus_PHASE_RUNNING)

	got, _, err := db.List(ctx, sandboxdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)

	// The DB layer stamped LastModifiedAt on Create; assert the original
	// fixture and the listed sandbox agree on the post-create state.
	assert.True(t, proto.Equal(original, got[0]),
		"List must return the same sandbox proto Get would")
}

func TestScanIndexForward_MapsSortOrder(t *testing.T) {
	t.Run("newest-first descends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST)
		require.NoError(t, err)
		assert.False(t, got, "ORDER_NEWEST_FIRST must map to ScanIndexForward=false")
	})
	t.Run("oldest-first ascends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListSandboxesRequest_ORDER_OLDEST_FIRST)
		require.NoError(t, err)
		assert.True(t, got, "ORDER_OLDEST_FIRST must map to ScanIndexForward=true")
	})
	t.Run("unspecified is rejected", func(t *testing.T) {
		_, err := scanIndexForward(cpv1.ListSandboxesRequest_ORDER_UNSPECIFIED)
		require.Error(t, err)
		assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
			"ORDER_UNSPECIFIED must wrap ErrInvalidListOptions, got: %v", err)
	})
}

func TestContinuationToken_RoundTrip(t *testing.T) {
	const indexName = indexByNamespace
	original := map[string]types.AttributeValue{
		"sandbox_id":        &types.AttributeValueMemberS{Value: "sb-abc"},
		"sandbox_namespace": &types.AttributeValueMemberS{Value: "team-alpha"},
		attrGSILmtSK:        &types.AttributeValueMemberS{Value: "2026-05-05T10:00:00Z#sb-abc"},
	}

	token, err := encodeContinuationToken(indexName, original)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	decoded, err := decodeContinuationToken(token, indexName)
	require.NoError(t, err)
	assert.Equal(t, len(original), len(decoded))
	for k, v := range original {
		want := v.(*types.AttributeValueMemberS).Value
		got, ok := decoded[k].(*types.AttributeValueMemberS)
		require.True(t, ok, "decoded attribute %q must remain a string", k)
		assert.Equal(t, want, got.Value)
	}
}

func TestContinuationToken_EmptyOnEmptyLEK(t *testing.T) {
	token, err := encodeContinuationToken(indexByNamespace, nil)
	require.NoError(t, err)
	assert.Equal(t, "", token,
		"a nil LEK must encode to the empty token, signalling no more pages")
}

func TestSelectIndex_PicksTheRightGSI(t *testing.T) {
	cases := []struct {
		name      string
		opts      sandboxdb.ListOptions
		wantIndex string
		wantHK    string
		wantValue string
	}{
		{
			name:      "namespace only",
			opts:      sandboxdb.ListOptions{Namespace: "team-alpha"},
			wantIndex: indexByNamespace,
			wantHK:    attrSandboxNamespace,
			wantValue: "team-alpha",
		},
		{
			name:      "namespace and phase",
			opts:      sandboxdb.ListOptions{Namespace: "team-alpha", StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING},
			wantIndex: indexByStatusPhase,
			wantHK:    attrGSINamespacePhaseHK,
			wantValue: "team-alpha#PHASE_RUNNING",
		},
		{
			name:      "namespace and node",
			opts:      sandboxdb.ListOptions{Namespace: "team-alpha", NodeID: "node-1"},
			wantIndex: indexByNamespaceNode,
			wantHK:    attrGSINamespaceNodeHK,
			wantValue: "team-alpha#node-1",
		},
		{
			name:      "namespace, node and phase prefers namespace+node index (phase filtered in-app)",
			opts:      sandboxdb.ListOptions{Namespace: "team-alpha", NodeID: "node-1", StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING},
			wantIndex: indexByNamespaceNode,
			wantHK:    attrGSINamespaceNodeHK,
			wantValue: "team-alpha#node-1",
		},
		{
			name:      "node only",
			opts:      sandboxdb.ListOptions{NodeID: "node-1"},
			wantIndex: indexByNode,
			wantHK:    attrNodeRefID,
			wantValue: "node-1",
		},
		{
			name:      "node and phase prefers node index (phase filtered in-app)",
			opts:      sandboxdb.ListOptions{NodeID: "node-1", StatusPhase: cpv1.SandboxStatus_PHASE_RUNNING},
			wantIndex: indexByNode,
			wantHK:    attrNodeRefID,
			wantValue: "node-1",
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
