package service

import (
	"context"
	"errors"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	"golang.nuinfra.api-server/pkg/scheduler"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultListSandboxesPageSize is the page size applied when ListSandboxes callers
// leave the field unset. Mirrors the documented default in the proto.
const defaultListSandboxesPageSize int32 = 30

// CreateSandbox implements [cpv1.SandboxServiceServer].
//
// Server-owned fields are set or zeroed regardless of what the client sends:
// the DB layer stamps Version, CreatedAt and LastModifiedAt itself, and we
// reset Status/Intent here. Intent.Phase is always PHASE_RUNNING on
// creation; transitioning to other phases is the job of Pause/Resume.
// Validation has already run in the interceptor by the time control reaches
// here.
//
// Node placement runs synchronously before persistence: the configured
// [scheduler.Scheduler] mutates sb.Node in place, and the row is then
// persisted with the assigned node. If the cluster has no healthy nodes
// available, the call surfaces as gRPC FailedPrecondition.
//
// Note: this couples idempotency to the scheduler's determinism. The
// random scheduler picks a (potentially) different node on retry, which
// changes the row's content digest and turns retries into AlreadyExists
// errors. This is a known property of the dev/test scheduler; a
// production scheduler will likely persist a placement decision before
// the sandbox row, breaking the coupling.
func (a *apiServer) CreateSandbox(ctx context.Context, req *cpv1.CreateSandboxRequest) (*cpv1.CreateSandboxResponse, error) {
	sb := req.GetSandbox()

	sb.Metadata.Version = 0 // db.Create stamps the initial version
	sb.Node = nil
	sb.Status = &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_PENDING}
	sb.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	if err := a.scheduler.Schedule(ctx, sb); err != nil {
		if errors.Is(err, scheduler.ErrNoHealthyNodes) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		ctxzap.Extract(ctx).Error("schedule sandbox failed",
			zap.String("sandbox.id", sb.GetMetadata().GetId()),
			zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to schedule sandbox")
	}

	if err := a.db.Create(ctx, sb); err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", "create", sb.GetMetadata().GetId(), err)
	}

	return &cpv1.CreateSandboxResponse{Sandbox: sb}, nil
}

// GetSandbox implements [cpv1.SandboxServiceServer].
func (a *apiServer) GetSandbox(ctx context.Context, req *cpv1.GetSandboxRequest) (*cpv1.GetSandboxResponse, error) {
	id := req.GetSandboxId()
	sb, err := a.db.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", "get", id, err)
	}
	return &cpv1.GetSandboxResponse{Sandbox: sb}, nil
}

// ListSandboxes implements [cpv1.SandboxServiceServer]. The protovalidate
// interceptor has already enforced that namespace or node_id is set and the
// page-size bounds; this layer only fills in defaults for fields the client
// left unset and forwards the rest to the DB. Empty results are valid.
func (a *apiServer) ListSandboxes(ctx context.Context, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
	opts := sandboxdb.ListOptions{
		Namespace:         req.GetNamespace(),
		NodeID:            req.GetNodeId(),
		StatusPhase:       req.GetStatusPhase(),
		SortOrder:         req.GetSortOrder(),
		PageSize:          req.GetPageSize(),
		ContinuationToken: req.GetContinuationToken(),
	}
	if opts.PageSize == 0 {
		opts.PageSize = defaultListSandboxesPageSize
	}
	if opts.SortOrder == cpv1.ListSandboxesRequest_ORDER_UNSPECIFIED {
		opts.SortOrder = cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST
	}

	sandboxes, nextToken, err := a.db.List(ctx, opts)
	if err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", "list", "", err)
	}
	return &cpv1.ListSandboxesResponse{
		Sandboxes:         sandboxes,
		ContinuationToken: nextToken,
	}, nil
}

// PauseSandbox implements [cpv1.SandboxServiceServer]. The saved Status.Phase
// must be PHASE_RUNNING; on success the row is left at status=PAUSING +
// intent=PAUSED so the data-plane reconciler can converge.
func (a *apiServer) PauseSandbox(ctx context.Context, req *cpv1.PauseSandboxRequest) (*cpv1.PauseSandboxResponse, error) {
	sb, err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_PAUSING, cpv1.SandboxStatus_PHASE_PAUSED, "pause")
	if err != nil {
		return nil, err
	}
	return &cpv1.PauseSandboxResponse{Sandbox: sb}, nil
}

// ResumeSandbox implements [cpv1.SandboxServiceServer]. The saved Status.Phase
// must be PHASE_PAUSED; on success the row is left at status=RESUMING +
// intent=RUNNING. There is no separate PHASE_RESUMED — once the reconciler
// closes the loop, status converges to RUNNING, identical to a freshly booted
// VM.
func (a *apiServer) ResumeSandbox(ctx context.Context, req *cpv1.ResumeSandboxRequest) (*cpv1.ResumeSandboxResponse, error) {
	sb, err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_RESUMING, cpv1.SandboxStatus_PHASE_RUNNING, "resume")
	if err != nil {
		return nil, err
	}
	return &cpv1.ResumeSandboxResponse{Sandbox: sb}, nil
}

// transitionPhase reads the sandbox by id, stamps the caller's read version
// onto the proto (arming the DB's optimistic-lock check), atomically flips
// Status.Phase to targetStatus and Intent.Phase to targetIntent, and writes
// back through DB.Update — which also enforces the state-machine guard
// derived from the new targetStatus. The returned *Sandbox is the
// post-update proto (DB.Update mutates in place: bumped Version, stamped
// LastModifiedAt), suitable for inclusion in the RPC response so clients
// don't need a follow-up GetSandbox.
func (a *apiServer) transitionPhase(
	ctx context.Context,
	id string,
	version int64,
	targetStatus, targetIntent cpv1.SandboxStatus_Phase,
	op string,
) (*cpv1.Sandbox, error) {
	sb, err := a.db.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", op, id, err)
	}

	sb.Metadata.Version = version
	sb.Status = &cpv1.SandboxStatus{Phase: targetStatus}
	sb.Intent = &cpv1.Intent{Phase: targetIntent}

	if err := a.db.Update(ctx, sb); err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", op, id, err)
	}
	return sb, nil
}

// DeleteSandbox implements [cpv1.SandboxServiceServer]. The control plane
// only records the user's intent to delete; the data-plane reconciler is
// responsible for tearing the sandbox down and eventually removing the row.
// Delete is accepted from any saved phase — the reconciler decides what to
// actually do based on the current Status.
func (a *apiServer) DeleteSandbox(ctx context.Context, req *cpv1.DeleteSandboxRequest) (*cpv1.DeleteSandboxResponse, error) {
	sb, err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_DELETING, cpv1.SandboxStatus_PHASE_DELETED, "delete")
	if err != nil {
		return nil, err
	}
	return &cpv1.DeleteSandboxResponse{Sandbox: sb}, nil
}

// StartSnapshot implements [cpv1.SandboxServiceServer]. The control plane
// records the user's intent to snapshot; the data-plane reconciler
// triggers firecracker, persists the artifacts to durable storage,
// clears the intent, and stamps Sandbox.LastSnapshot.
//
// Status.Phase is flipped to PHASE_SNAPSHOTTING (the in-progress
// marker) and Intent.Phase is stamped with the saved phase the
// daemon should restore the sandbox to after the snapshot completes
// (either RUNNING or PAUSED — these are the two phases the DB
// state-machine guard accepts as prior). Intent.Resources is left
// alone if previously set.
//
// Preflight: existence via db.Get → ErrNotFound → codes.NotFound;
// version conflicts → codes.Aborted; state-machine violations
// (saved phase not RUNNING/PAUSED) → codes.FailedPrecondition.
// Description max-len 256 is enforced by the protovalidate interceptor.
func (a *apiServer) StartSnapshot(ctx context.Context, req *cpv1.StartSnapshotRequest) (*cpv1.StartSnapshotResponse, error) {
	id := req.GetSandboxId()
	sb, err := a.db.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", "start snapshot", id, err)
	}

	savedPhase := sb.GetStatus().GetPhase()
	sb.Metadata.Version = req.GetVersion()
	if sb.Intent == nil {
		sb.Intent = &cpv1.Intent{}
	}
	sb.Intent.Phase = savedPhase // restore target — set even if PHASE_UNSPECIFIED, the DB guard handles it
	sb.Intent.StartSnapshot = &cpv1.StartSnapshotInput{Description: req.GetDescription()}
	sb.Status = &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_SNAPSHOTTING}

	if err := a.db.Update(ctx, sb); err != nil {
		return nil, dbErrToStatus(ctx, "sandbox", "start snapshot", id, err)
	}
	return &cpv1.StartSnapshotResponse{Sandbox: sb}, nil
}
