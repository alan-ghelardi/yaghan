package service

import (
	"context"

	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// CreateSandbox implements [cpv1.SandboxServiceServer].
//
// Server-owned fields are set or zeroed regardless of what the client sends:
// the DB layer stamps Version, CreatedAt and LastModifiedAt itself, and we
// reset Node/Status/Intent here. Intent.Phase is always PHASE_RUNNING on
// creation; transitioning to other phases is the job of Pause/Resume.
// Validation has already run in the interceptor by the time control reaches
// here.
func (a *apiServer) CreateSandbox(ctx context.Context, req *cpv1.CreateSandboxRequest) (*cpv1.CreateSandboxResponse, error) {
	sb := req.GetSandbox()

	sb.Metadata.Version = 0 // db.Create stamps the initial version
	sb.Node = nil
	sb.Status = &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_PENDING}
	sb.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	if err := a.db.Create(ctx, sb); err != nil {
		return nil, dbErrToStatus(ctx, "create", sb.GetMetadata().GetId(), err)
	}

	return &cpv1.CreateSandboxResponse{Sandbox: sb}, nil
}

// GetSandbox implements [cpv1.SandboxServiceServer].
func (a *apiServer) GetSandbox(ctx context.Context, req *cpv1.GetSandboxRequest) (*cpv1.GetSandboxResponse, error) {
	id := req.GetSandboxId()
	sb, err := a.db.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "get", id, err)
	}
	return &cpv1.GetSandboxResponse{Sandbox: sb}, nil
}

// PauseSandbox implements [cpv1.SandboxServiceServer]. The saved Status.Phase
// must be PHASE_RUNNING; on success the row is left at status=PAUSING +
// intent=PAUSED so the data-plane reconciler can converge.
func (a *apiServer) PauseSandbox(ctx context.Context, req *cpv1.PauseSandboxRequest) (*cpv1.PauseSandboxResponse, error) {
	if err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_PAUSING, cpv1.SandboxStatus_PHASE_PAUSED, "pause"); err != nil {
		return nil, err
	}
	return &cpv1.PauseSandboxResponse{}, nil
}

// ResumeSandbox implements [cpv1.SandboxServiceServer]. The saved Status.Phase
// must be PHASE_PAUSED; on success the row is left at status=RESUMING +
// intent=RUNNING. There is no separate PHASE_RESUMED — once the reconciler
// closes the loop, status converges to RUNNING, identical to a freshly booted
// VM.
func (a *apiServer) ResumeSandbox(ctx context.Context, req *cpv1.ResumeSandboxRequest) (*cpv1.ResumeSandboxResponse, error) {
	if err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_RESUMING, cpv1.SandboxStatus_PHASE_RUNNING, "resume"); err != nil {
		return nil, err
	}
	return &cpv1.ResumeSandboxResponse{}, nil
}

// transitionPhase reads the sandbox by id, stamps the caller's read version
// onto the proto (arming the DB's optimistic-lock check), atomically flips
// Status.Phase to targetStatus and Intent.Phase to targetIntent, and writes
// back through DB.Update — which also enforces the state-machine guard
// derived from the new targetStatus.
func (a *apiServer) transitionPhase(
	ctx context.Context,
	id string,
	version int64,
	targetStatus, targetIntent cpv1.SandboxStatus_Phase,
	op string,
) error {
	sb, err := a.db.Get(ctx, id)
	if err != nil {
		return dbErrToStatus(ctx, op, id, err)
	}

	sb.Metadata.Version = version
	sb.Status = &cpv1.SandboxStatus{Phase: targetStatus}
	sb.Intent = &cpv1.Intent{Phase: targetIntent}

	if err := a.db.Update(ctx, sb); err != nil {
		return dbErrToStatus(ctx, op, id, err)
	}
	return nil
}

// DeleteSandbox implements [cpv1.SandboxServiceServer]. The control plane
// only records the user's intent to delete; the data-plane reconciler is
// responsible for tearing the sandbox down and eventually removing the row.
// Delete is accepted from any saved phase — the reconciler decides what to
// actually do based on the current Status.
func (a *apiServer) DeleteSandbox(ctx context.Context, req *cpv1.DeleteSandboxRequest) (*cpv1.DeleteSandboxResponse, error) {
	if err := a.transitionPhase(ctx, req.GetSandboxId(), req.GetVersion(),
		cpv1.SandboxStatus_PHASE_DELETING, cpv1.SandboxStatus_PHASE_DELETED, "delete"); err != nil {
		return nil, err
	}
	return &cpv1.DeleteSandboxResponse{}, nil
}
