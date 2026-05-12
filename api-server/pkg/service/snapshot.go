package service

import (
	"context"

	snapshotdb "golang.nuinfra.api-server/pkg/db/snapshot"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// defaultListSnapshotsPageSize is the page size applied when ListSnapshots
// callers leave the field unset. Mirrors the documented default in the proto.
const defaultListSnapshotsPageSize int32 = 30

// CreateSnapshot implements [cpv1.SnapshotServiceServer].
//
// Server-owned fields are zeroed regardless of what the client sends: the
// DB layer stamps CreatedAt itself. Validation (id / namespace pattern /
// description length / required sandbox ref) has already run in the
// protovalidate interceptor by the time control reaches here.
//
// The caller is currently the daemon's reconciler, which persists the
// snapshot row as the final stage of its snapshot loop. Idempotency
// piggy-backs on the DB's content-digest mechanism: a retry of the same
// logical Create returns OK without overwriting the existing row.
func (a *apiServer) CreateSnapshot(ctx context.Context, req *cpv1.CreateSnapshotRequest) (*cpv1.CreateSnapshotResponse, error) {
	sn := req.GetSnapshot()
	sn.Metadata.CreatedAt = nil // db.Create stamps it

	if err := a.snapshotDB.Create(ctx, sn); err != nil {
		return nil, dbErrToStatus(ctx, "snapshot", "create", sn.GetMetadata().GetId(), err)
	}
	return &cpv1.CreateSnapshotResponse{Snapshot: sn}, nil
}

// GetSnapshot implements [cpv1.SnapshotServiceServer].
func (a *apiServer) GetSnapshot(ctx context.Context, req *cpv1.GetSnapshotRequest) (*cpv1.GetSnapshotResponse, error) {
	id := req.GetSnapshotId()
	sn, err := a.snapshotDB.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "snapshot", "get", id, err)
	}
	return &cpv1.GetSnapshotResponse{Snapshot: sn}, nil
}

// ListSnapshots implements [cpv1.SnapshotServiceServer]. The protovalidate
// interceptor enforces the mutual exclusion between namespace and sandbox_id
// (and that at least one is set) plus the page-size bounds; this layer
// only fills in defaults for fields the client left unset and forwards
// the rest to the DB. Empty results are valid.
func (a *apiServer) ListSnapshots(ctx context.Context, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
	opts := snapshotdb.ListOptions{
		Namespace:         req.GetNamespace(),
		SandboxID:         req.GetSandboxId(),
		SortOrder:         req.GetSortOrder(),
		PageSize:          req.GetPageSize(),
		ContinuationToken: req.GetContinuationToken(),
	}
	if opts.PageSize == 0 {
		opts.PageSize = defaultListSnapshotsPageSize
	}
	if opts.SortOrder == cpv1.ListSnapshotsRequest_ORDER_UNSPECIFIED {
		opts.SortOrder = cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST
	}

	snapshots, nextToken, err := a.snapshotDB.List(ctx, opts)
	if err != nil {
		return nil, dbErrToStatus(ctx, "snapshot", "list", "", err)
	}
	return &cpv1.ListSnapshotsResponse{
		Snapshots:         snapshots,
		ContinuationToken: nextToken,
	}, nil
}

// DeleteSnapshot implements [cpv1.SnapshotServiceServer]. The operation
// is synchronous and idempotent: deleting a non-existent snapshot
// returns OK, matching the DB-layer contract. Cleaning up the snapshot
// artifact in durable storage is a separate concern owned by the
// daemon-side store.
func (a *apiServer) DeleteSnapshot(ctx context.Context, req *cpv1.DeleteSnapshotRequest) (*cpv1.DeleteSnapshotResponse, error) {
	id := req.GetSnapshotId()
	if err := a.snapshotDB.Delete(ctx, id); err != nil {
		return nil, dbErrToStatus(ctx, "snapshot", "delete", id, err)
	}
	return &cpv1.DeleteSnapshotResponse{}, nil
}
