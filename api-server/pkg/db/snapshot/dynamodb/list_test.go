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
	snapshotdb "golang.nuinfra.api-server/pkg/db/snapshot"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// listFixture creates and persists a snapshot with the supplied id,
// namespace, and sandbox id. The DB stamps CreatedAt with wall-clock
// precision on every Create; sleeping a short interval after each call
// guarantees a strictly later timestamp on the next fixture so the
// gsi_ct_sk-ordered list results are deterministic across rows.
func listFixture(ctx context.Context, t *testing.T, db *dynamoDB, id, namespace, sandboxID string) *cpv1.Snapshot {
	t.Helper()
	sn := newFixture(withID(id), withNamespace(namespace), withSandbox(sandboxID))
	require.NoError(t, db.Create(ctx, sn))
	time.Sleep(2 * time.Millisecond)
	return sn
}

func ids(snapshots []*cpv1.Snapshot) []string {
	out := make([]string, len(snapshots))
	for i, sn := range snapshots {
		out[i] = sn.GetMetadata().GetId()
	}
	return out
}

func TestList_RejectsUnboundedQuery(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, snapshotdb.ListOptions{
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"missing namespace and sandbox_id must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsBothFiltersSet(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: "team-alpha",
		SandboxID: "sb-001",
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"both namespace and sandbox_id set must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsNonPositivePageSize(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  0,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"page_size <= 0 must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_RejectsUnspecifiedSortOrder(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_UNSPECIFIED,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
		"ORDER_UNSPECIFIED must wrap ErrInvalidListOptions, got: %v", err)
}

func TestList_ByNamespace_ReturnsAllRowsInNamespace(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "snap-bn-a", ns, "sb-1")
	listFixture(ctx, t, db, "snap-bn-b", ns, "sb-2")
	listFixture(ctx, t, db, "snap-bn-c", ns, "sb-1")
	// Different namespace — must NOT appear.
	listFixture(ctx, t, db, "snap-bn-other", "team-beta", "sb-1")

	got, next, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: ns,
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, "", next, "page smaller than PageSize must clear the continuation token")
	assert.Equal(t, []string{"snap-bn-c", "snap-bn-b", "snap-bn-a"}, ids(got),
		"by_namespace_index must include every snapshot in the namespace, newest-first")
}

func TestList_BySandbox_ReturnsAllRowsForSandbox(t *testing.T) {
	db, ctx := setupDB(t)
	const sb = "sb-target"

	listFixture(ctx, t, db, "snap-bs-a", "team-alpha", sb)
	listFixture(ctx, t, db, "snap-bs-b", "team-beta", sb)
	listFixture(ctx, t, db, "snap-bs-c", "team-alpha", sb)
	// Different sandbox — must NOT appear.
	listFixture(ctx, t, db, "snap-bs-other", "team-alpha", "sb-other")

	got, _, err := db.List(ctx, snapshotdb.ListOptions{
		SandboxID: sb,
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-bs-c", "snap-bs-b", "snap-bs-a"}, ids(got),
		"by_sandbox_index must span namespaces but stay scoped to the sandbox, newest-first")
}

func TestList_OldestFirst(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	listFixture(ctx, t, db, "snap-of-a", ns, "sb-1")
	listFixture(ctx, t, db, "snap-of-b", ns, "sb-1")
	listFixture(ctx, t, db, "snap-of-c", ns, "sb-1")

	got, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: ns,
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_OLDEST_FIRST,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"snap-of-a", "snap-of-b", "snap-of-c"}, ids(got),
		"ORDER_OLDEST_FIRST must return rows in ascending gsi_ct_sk order")
}

func TestList_EmptyResultSet(t *testing.T) {
	db, ctx := setupDB(t)

	got, next, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
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
		listFixture(ctx, t, db, fmt.Sprintf("snap-pg-%02d", i), ns, "sb-pg")
	}

	var collected []string
	var token string
	for page := 0; page < total+2; page++ { // safety bound
		got, next, err := db.List(ctx, snapshotdb.ListOptions{
			Namespace:         ns,
			PageSize:          2,
			SortOrder:         cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
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
		[]string{"snap-pg-04", "snap-pg-03", "snap-pg-02", "snap-pg-01", "snap-pg-00"},
		collected,
		"pagination must traverse rows newest-first with no duplicates or gaps")
}

func TestList_ContinuationTokenRejectedOnIndexMismatch(t *testing.T) {
	db, ctx := setupDB(t)
	const ns = "team-alpha"

	for i := 0; i < 3; i++ {
		listFixture(ctx, t, db, fmt.Sprintf("snap-cti-%02d", i), ns, "sb-cti")
	}

	// First page issued against by_namespace_index.
	_, token, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: ns,
		PageSize:  1,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.NotEmpty(t, token, "expected a continuation token after the first page")

	// Reuse against by_sandbox_index: the embedded index name no longer
	// matches and the token must be rejected before issuing a query that
	// DynamoDB would reject with a less friendly error.
	_, _, err = db.List(ctx, snapshotdb.ListOptions{
		SandboxID:         "sb-cti",
		PageSize:          1,
		SortOrder:         cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: token,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"token reuse across indexes must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_ContinuationTokenRejectedOnGarbledInput(t *testing.T) {
	db, ctx := setupDB(t)

	_, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace:         "team-alpha",
		PageSize:          1,
		SortOrder:         cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
		ContinuationToken: "this-is-not-a-valid-token!!!",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dberrors.ErrInvalidContinuationToken),
		"garbled token must wrap ErrInvalidContinuationToken, got: %v", err)
}

func TestList_RoundTripsSnapshotProto(t *testing.T) {
	db, ctx := setupDB(t)

	original := listFixture(ctx, t, db, "snap-rt", "team-alpha", "sb-rt")

	got, _, err := db.List(ctx, snapshotdb.ListOptions{
		Namespace: "team-alpha",
		PageSize:  10,
		SortOrder: cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, proto.Equal(original, got[0]),
		"List must return the same snapshot proto Get would")
}

func TestScanIndexForward_MapsSortOrder(t *testing.T) {
	t.Run("newest-first descends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST)
		require.NoError(t, err)
		assert.False(t, got, "ORDER_NEWEST_FIRST must map to ScanIndexForward=false")
	})
	t.Run("oldest-first ascends the SK", func(t *testing.T) {
		got, err := scanIndexForward(cpv1.ListSnapshotsRequest_ORDER_OLDEST_FIRST)
		require.NoError(t, err)
		assert.True(t, got, "ORDER_OLDEST_FIRST must map to ScanIndexForward=true")
	})
	t.Run("unspecified is rejected", func(t *testing.T) {
		_, err := scanIndexForward(cpv1.ListSnapshotsRequest_ORDER_UNSPECIFIED)
		require.Error(t, err)
		assert.True(t, errors.Is(err, dberrors.ErrInvalidListOptions),
			"ORDER_UNSPECIFIED must wrap ErrInvalidListOptions, got: %v", err)
	})
}

func TestContinuationToken_RoundTrip(t *testing.T) {
	const indexName = indexByNamespace
	original := map[string]types.AttributeValue{
		"snapshot_id":        &types.AttributeValueMemberS{Value: "snap-abc"},
		"snapshot_namespace": &types.AttributeValueMemberS{Value: "team-alpha"},
		attrGSICtSK:          &types.AttributeValueMemberS{Value: "2026-05-05T10:00:00Z#snap-abc"},
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
		opts      snapshotdb.ListOptions
		wantIndex string
		wantHK    string
		wantValue string
	}{
		{
			name:      "namespace only",
			opts:      snapshotdb.ListOptions{Namespace: "team-alpha"},
			wantIndex: indexByNamespace,
			wantHK:    attrSnapshotNamespace,
			wantValue: "team-alpha",
		},
		{
			name:      "sandbox only",
			opts:      snapshotdb.ListOptions{SandboxID: "sb-1"},
			wantIndex: indexBySandbox,
			wantHK:    attrSandboxRefID,
			wantValue: "sb-1",
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
