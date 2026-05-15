package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// staleTime is a date well in the past used to assert the DB advances
// CreatedAt past whatever the client supplied.
var staleTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// newSnapshotFixture builds a Snapshot proto with the supplied id, scoped
// to validNamespace and a sandbox ref by default. The DB stamps CreatedAt
// itself, so the fixture leaves it unset. Resources defaults to the same
// shape newCreateRequest uses; tests that need to assert inheritance can
// override via an opt.
func newSnapshotFixture(id string, opts ...func(*cpv1.Snapshot)) *cpv1.Snapshot {
	sn := &cpv1.Snapshot{
		Metadata: &cpv1.SnapshotMeta{
			Id:          id,
			Namespace:   validNamespace,
			Description: "nightly checkpoint",
		},
		Sandbox:   &cpv1.SandboxRef{Id: "sb-001"},
		Resources: &cpv1.Resources{VcpuCount: 2, MemoryMib: 1024},
	}
	for _, opt := range opts {
		opt(sn)
	}
	return sn
}

func snapshotIDs(resp *cpv1.ListSnapshotsResponse) []string {
	out := make([]string, len(resp.GetSnapshots()))
	for i, sn := range resp.GetSnapshots() {
		out[i] = sn.GetMetadata().GetId()
	}
	return out
}

// createSnapshot persists a snapshot through the gRPC client and returns
// the stored proto. The DB stamps CreatedAt server-side.
func createSnapshot(ctx context.Context, t *testing.T, h *harness, sn *cpv1.Snapshot) *cpv1.Snapshot {
	t.Helper()
	resp, err := h.snapshot.CreateSnapshot(ctx, &cpv1.CreateSnapshotRequest{Snapshot: sn})
	require.NoError(t, err)
	require.NotNil(t, resp.GetSnapshot())
	return resp.GetSnapshot()
}

func TestCreateSnapshot_HappyPath(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	stored := createSnapshot(ctx, t, h, newSnapshotFixture("snap-create-1"))

	require.NotNil(t, stored.GetMetadata().GetCreatedAt(),
		"response must carry server-stamped CreatedAt")

	// Read-back must round-trip the same proto Get returns.
	resp, err := h.snapshot.GetSnapshot(ctx, &cpv1.GetSnapshotRequest{SnapshotId: "snap-create-1"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(stored, resp.GetSnapshot()),
		"GetSnapshot must round-trip the snapshot CreateSnapshot returned")
}

func TestCreateSnapshot_ZeroesClientSuppliedCreatedAt(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	// A client-supplied CreatedAt must not leak into the stored row —
	// the DB owns the field.
	sn := newSnapshotFixture("snap-clock-bug")
	// Force a stale timestamp on the input; the handler zeros it and the
	// DB stamps a fresh one.
	sn.Metadata.CreatedAt = timestamppb.New(staleTime)

	stored := createSnapshot(ctx, t, h, sn)
	require.NotNil(t, stored.GetMetadata().GetCreatedAt())
	assert.True(t,
		stored.GetMetadata().GetCreatedAt().AsTime().After(staleTime),
		"server-stamped CreatedAt must advance past the client-supplied stale value")
}

func TestCreateSnapshot_IdempotentRetry(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	first := createSnapshot(ctx, t, h, newSnapshotFixture("snap-idem"))

	// A retry with the same logical content (different CreatedAt is
	// fine — the digest excludes it) must be a no-op success.
	retry := newSnapshotFixture("snap-idem")
	_, err := h.snapshot.CreateSnapshot(ctx, &cpv1.CreateSnapshotRequest{Snapshot: retry})
	require.NoError(t, err)

	// The stored row must reflect the FIRST write.
	got, err := h.snapshot.GetSnapshot(ctx, &cpv1.GetSnapshotRequest{SnapshotId: "snap-idem"})
	require.NoError(t, err)
	assert.True(t,
		first.GetMetadata().GetCreatedAt().AsTime().Equal(got.GetSnapshot().GetMetadata().GetCreatedAt().AsTime()),
		"the stored CreatedAt must remain the first-write value after a retry")
}

func TestCreateSnapshot_ConflictOnDifferentBody(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	createSnapshot(ctx, t, h, newSnapshotFixture("snap-conflict",
		func(s *cpv1.Snapshot) { s.Metadata.Description = "first" }))

	// Same id, different description → ErrAlreadyExists → AlreadyExists.
	_, err := h.snapshot.CreateSnapshot(ctx, &cpv1.CreateSnapshotRequest{
		Snapshot: newSnapshotFixture("snap-conflict",
			func(s *cpv1.Snapshot) { s.Metadata.Description = "second" }),
	})
	assertCode(t, err, codes.AlreadyExists)
}

func TestCreateSnapshot_ValidatesMissingFields(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	cases := []struct {
		name string
		req  *cpv1.CreateSnapshotRequest
	}{
		{
			name: "missing snapshot",
			req:  &cpv1.CreateSnapshotRequest{},
		},
		{
			name: "missing id",
			req: &cpv1.CreateSnapshotRequest{
				Snapshot: &cpv1.Snapshot{
					Metadata: &cpv1.SnapshotMeta{Namespace: validNamespace},
					Sandbox:  &cpv1.SandboxRef{Id: "sb-1"},
				},
			},
		},
		{
			name: "missing sandbox ref",
			req: &cpv1.CreateSnapshotRequest{
				Snapshot: &cpv1.Snapshot{
					Metadata:  &cpv1.SnapshotMeta{Id: "snap-x", Namespace: validNamespace},
					Resources: &cpv1.Resources{VcpuCount: 1, MemoryMib: 128},
				},
			},
		},
		{
			// Snapshot.Resources is marked required at the proto level so
			// every persisted row carries the values a derived sandbox
			// will inherit. Omitting it must surface as InvalidArgument
			// from the buf.validate interceptor.
			name: "missing resources",
			req: &cpv1.CreateSnapshotRequest{
				Snapshot: &cpv1.Snapshot{
					Metadata: &cpv1.SnapshotMeta{Id: "snap-y", Namespace: validNamespace},
					Sandbox:  &cpv1.SandboxRef{Id: "sb-1"},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.snapshot.CreateSnapshot(ctx, tc.req)
			assertCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestGetSnapshot_NotFound(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.snapshot.GetSnapshot(ctx, &cpv1.GetSnapshotRequest{SnapshotId: "no-such-snapshot"})
	assertCode(t, err, codes.NotFound)
}

func TestGetSnapshot_ValidatesMissingId(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.snapshot.GetSnapshot(ctx, &cpv1.GetSnapshotRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListSnapshots_ByNamespace(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	createSnapshot(ctx, t, h, newSnapshotFixture("snap-list-a"))
	createSnapshot(ctx, t, h, newSnapshotFixture("snap-list-b"))
	createSnapshot(ctx, t, h, newSnapshotFixture("snap-list-other",
		func(s *cpv1.Snapshot) { s.Metadata.Namespace = "team-beta" }))

	resp, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{Namespace: validNamespace})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"snap-list-a", "snap-list-b"}, snapshotIDs(resp),
		"namespace filter must include only matching snapshots")
}

func TestListSnapshots_BySandbox(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	createSnapshot(ctx, t, h, newSnapshotFixture("snap-bs-a",
		func(s *cpv1.Snapshot) { s.Sandbox = &cpv1.SandboxRef{Id: "sb-target"} }))
	createSnapshot(ctx, t, h, newSnapshotFixture("snap-bs-b",
		func(s *cpv1.Snapshot) { s.Sandbox = &cpv1.SandboxRef{Id: "sb-target"} }))
	createSnapshot(ctx, t, h, newSnapshotFixture("snap-bs-other",
		func(s *cpv1.Snapshot) { s.Sandbox = &cpv1.SandboxRef{Id: "sb-other"} }))

	resp, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{SandboxId: "sb-target"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"snap-bs-a", "snap-bs-b"}, snapshotIDs(resp),
		"sandbox_id filter must include only snapshots from the requested sandbox")
}

func TestListSnapshots_AppliesDefaults(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	createSnapshot(ctx, t, h, newSnapshotFixture("snap-defaults"))

	// PageSize zero + SortOrder unspecified must succeed because the
	// handler substitutes the documented defaults (30, NEWEST_FIRST)
	// before reaching the DB.
	resp, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{Namespace: validNamespace})
	require.NoError(t, err, "unset PageSize/SortOrder must succeed via handler defaulting")
	assert.NotEmpty(t, resp.GetSnapshots())
}

func TestListSnapshots_PaginatesAcrossPages(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	const total = 5
	for i := 0; i < total; i++ {
		createSnapshot(ctx, t, h, newSnapshotFixture(fmt.Sprintf("snap-pg-%02d", i)))
	}

	var collected []string
	var token string
	for page := 0; page < total+2; page++ { // safety bound
		resp, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{
			Namespace:         validNamespace,
			PageSize:          2,
			ContinuationToken: token,
		})
		require.NoError(t, err)
		collected = append(collected, snapshotIDs(resp)...)
		token = resp.GetContinuationToken()
		if token == "" {
			break
		}
	}

	assert.Len(t, collected, total,
		"pagination must enumerate every snapshot exactly once")
}

func TestListSnapshots_ValidatesMissingFilters(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	// Neither namespace nor sandbox_id set → CEL rule fires →
	// codes.InvalidArgument.
	_, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListSnapshots_ValidatesMutuallyExclusiveFilters(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	// Both namespace and sandbox_id set → CEL rule fires →
	// codes.InvalidArgument.
	_, err := h.snapshot.ListSnapshots(ctx, &cpv1.ListSnapshotsRequest{
		Namespace: validNamespace,
		SandboxId: "sb-1",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestDeleteSnapshot_RemovesExistingRow(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	createSnapshot(ctx, t, h, newSnapshotFixture("snap-del-1"))

	_, err := h.snapshot.DeleteSnapshot(ctx, &cpv1.DeleteSnapshotRequest{SnapshotId: "snap-del-1"})
	require.NoError(t, err)

	_, err = h.snapshot.GetSnapshot(ctx, &cpv1.GetSnapshotRequest{SnapshotId: "snap-del-1"})
	assertCode(t, err, codes.NotFound)
}

func TestDeleteSnapshot_IdempotentOnMissingRow(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	// Delete on a never-created id must return OK — matching the
	// DB-layer contract.
	_, err := h.snapshot.DeleteSnapshot(ctx, &cpv1.DeleteSnapshotRequest{SnapshotId: "snap-never"})
	require.NoError(t, err)
}

func TestDeleteSnapshot_ValidatesMissingId(t *testing.T) {
	h := startService(t, withoutDefaultNode())
	ctx := t.Context()

	_, err := h.snapshot.DeleteSnapshot(ctx, &cpv1.DeleteSnapshotRequest{})
	assertCode(t, err, codes.InvalidArgument)
}
