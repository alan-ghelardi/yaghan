// Package reconciler converges the actual MicroVM state on a host with
// the desired state expressed in a Sandbox proto's Intent field. It is
// invoked once per Sandbox event received from the control plane.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-openapi/swag/conv"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	"golang.nuinfra.net/firecracker-client/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Reconciler drives a Sandbox towards the state expressed in its Intent
// field. It is stateless across calls — each VM's network namespace
// index lives on the firecracker side (persisted in meta.json) so a
// daemon restart does not lose track of which namespace belongs to
// which VM.
type Reconciler struct {
	config         *config.Bundle
	provider       firecracker.Provider
	driver         network.Driver
	snapshotStore  *snapshot.Store
	snapshotClient controlplanev1alpha1.SnapshotServiceClient
}

// New constructs a Reconciler bound to the given provider, driver and
// daemon configuration. The Bundle's MicroVM section supplies the
// per-VM template constants (kernel paths, agent vsock port, guest MAC,
// …) used when composing a CreateMicroVMInput. The snapshotClient is
// used to register a Snapshot row in the api-server as the final stage
// of the snapshot reconcile path; a nil value disables that registration
// (acceptable when the daemon is configured without snapshot support).
func New(
	config *config.Bundle,
	provider firecracker.Provider,
	driver network.Driver,
	snapshotStore *snapshot.Store,
	snapshotClient controlplanev1alpha1.SnapshotServiceClient,
) *Reconciler {
	return &Reconciler{
		config:         config,
		provider:       provider,
		driver:         driver,
		snapshotStore:  snapshotStore,
		snapshotClient: snapshotClient,
	}
}

// Reconcile converges sandbox towards its Intent in three steps —
// state transition, resource update, snapshot — applied in order. Each
// step that succeeds clears the corresponding intent field and, where
// applicable, mirrors the achieved state into Status. Once every step
// has run successfully the entire Intent is set to nil.
//
// A nil Intent is treated as a no-op. The first step that fails
// returns its error immediately; subsequent steps are not attempted,
// and intent fields belonging to steps that already succeeded stay
// cleared so a retry resumes from where the previous attempt left off.
//
// All operations are designed to be idempotent.
func (r *Reconciler) Reconcile(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	if sandbox.GetIntent() == nil {
		return nil
	}

	if err := r.reconcilePhase(ctx, sandbox); err != nil {
		return err
	}
	if err := r.reconcileResources(ctx, sandbox); err != nil {
		return err
	}

	sandbox.Intent = nil
	return nil
}

// reconcilePhase applies the desired phase transition. The api-server writes
// Status.Phase as the in-progress marker (PENDING/PAUSING/RESUMING/DELETING)
// and Intent.Phase as the terminal target the reconciler should converge to.
// We dispatch on Status.Phase — it names the action 1:1 (boot vs resume both
// terminate in RUNNING, but only one starts in RESUMING) — and close the loop
// by writing Status.Phase = Intent.Phase.
//
// No-op when Intent.Phase is PHASE_UNSPECIFIED (already converged).
func (r *Reconciler) reconcilePhase(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	intent := sandbox.GetIntent()
	target := intent.GetPhase()
	if target == controlplanev1alpha1.SandboxStatus_PHASE_UNSPECIFIED {
		return nil
	}

	// intent.phase is the terminal target; the api-server only writes the
	// values RUNNING/PAUSED/DELETED here. A transitional value (PAUSING,
	// RESUMING, DELETING, PENDING) reaching the reconciler is a bug
	// upstream — surface it rather than silently mirror it into status.
	switch target {
	case controlplanev1alpha1.SandboxStatus_PHASE_RUNNING,
		controlplanev1alpha1.SandboxStatus_PHASE_PAUSED,
		controlplanev1alpha1.SandboxStatus_PHASE_DELETED:
		// valid terminal target
	default:
		return fmt.Errorf("reconciler: unsupported intent phase %v", target)
	}

	var (
		action  func(context.Context, *controlplanev1alpha1.Sandbox) error
		message string
	)
	switch sandbox.GetStatus().GetPhase() {
	case controlplanev1alpha1.SandboxStatus_PHASE_PENDING:
		action, message = r.boot, "Sandbox is running"
	case controlplanev1alpha1.SandboxStatus_PHASE_PAUSING:
		action, message = r.pause, "Sandbox has been paused"
	case controlplanev1alpha1.SandboxStatus_PHASE_RESUMING:
		action, message = r.resume, "Sandbox is running"
	case controlplanev1alpha1.SandboxStatus_PHASE_DELETING:
		action, message = r.delete, "Sandbox has been deleted"
	case controlplanev1alpha1.SandboxStatus_PHASE_SNAPSHOTTING:
		// The api-server stamped Intent.Phase with the saved phase
		// (RUNNING or PAUSED), so the shared post-action code below
		// restores it for us once snapshot() returns.
		action, message = r.snapshot, "Snapshot created"
	default:
		return fmt.Errorf(
			"reconciler: unsupported status phase %v with intent %v",
			sandbox.GetStatus().GetPhase(), target)
	}

	if err := action(ctx, sandbox); err != nil {
		return err
	}

	if sandbox.Status == nil {
		sandbox.Status = &controlplanev1alpha1.SandboxStatus{}
	}
	sandbox.Status.Phase = target
	sandbox.Status.Message = message
	intent.Phase = controlplanev1alpha1.SandboxStatus_PHASE_UNSPECIFIED
	return nil
}

// boot ensures a MicroVM exists for the sandbox. Idempotent: if the
// provider already has the VM indexed, boot returns nil without
// touching the network driver or firecracker. The network index is
// persisted by the firecracker package alongside the chroot so a
// later delete (potentially after a daemon restart) can deprovision
// the matching namespace.
func (r *Reconciler) boot(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	id := sandbox.GetMetadata().GetId()
	if vm := r.provider.GetMicroVM(id); vm != nil {
		return nil
	}

	index := r.provider.Len()
	nsh, err := r.driver.Provision(index)
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}

	input := r.buildCreateInput(sandbox, nsh)
	if _, err := r.provider.CreateMicroVM(ctx, input); err != nil {
		// Roll the network back so we don't leak an orphaned namespace
		// if the VM never came up.
		_ = r.driver.Deprovision(index)
		return fmt.Errorf("create microvm: %w", err)
	}
	return nil
}

// pause moves a running MicroVM into the paused state.
func (r *Reconciler) pause(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	vm, err := r.lookup(sandbox.GetMetadata().GetId(), "pause")
	if err != nil {
		return err
	}
	if err := vm.Pause(ctx); err != nil {
		return fmt.Errorf("pause microvm: %w", err)
	}
	return nil
}

// resume restores a paused MicroVM to the running state.
func (r *Reconciler) resume(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	vm, err := r.lookup(sandbox.GetMetadata().GetId(), "resume")
	if err != nil {
		return err
	}
	if err := vm.Resume(ctx); err != nil {
		return fmt.Errorf("resume microvm: %w", err)
	}
	return nil
}

// delete tears down the MicroVM and its network namespace. Treating
// firecracker.ErrVMNotFound as success keeps the operation idempotent
// across retries.
//
// The network index lives in the VM's persisted meta.json, so we read
// it BEFORE DeleteMicroVM (which wipes the chroot). When the VM is
// already gone the index is unknown — we skip Deprovision to avoid
// taking down a namespace that belongs to a different VM.
func (r *Reconciler) delete(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	id := sandbox.GetMetadata().GetId()

	var (
		networkIndex int
		hasIndex     bool
	)
	if vm := r.provider.GetMicroVM(id); vm != nil {
		networkIndex = vm.NetworkIndex()
		hasIndex = true
	}

	if err := r.provider.DeleteMicroVM(ctx, id); err != nil && !errors.Is(err, firecracker.ErrVMNotFound) {
		return fmt.Errorf("delete microvm: %w", err)
	}

	if hasIndex {
		if err := r.driver.Deprovision(networkIndex); err != nil {
			return fmt.Errorf("deprovision network: %w", err)
		}
	}
	return nil
}

// reconcileResources applies an in-place vCPU/memory update. No-op
// when Intent.Resources is unset, and a free fast-path no-op when the
// intent matches the sandbox's last-applied resources — that case
// shows up when an event is redelivered after a successful update,
// and there's no point round-tripping firecracker just to set the
// values it already has.
func (r *Reconciler) reconcileResources(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	intent := sandbox.GetIntent()
	if intent.GetResources() == nil {
		return nil
	}

	if proto.Equal(intent.GetResources(), sandbox.GetResources()) {
		intent.Resources = nil
		return nil
	}

	vm, err := r.lookup(sandbox.GetMetadata().GetId(), "update resources")
	if err != nil {
		return err
	}

	if err := vm.UpdateResources(ctx, firecracker.UpdateResourcesInput{
		VCPUCount: int64(intent.Resources.GetVcpuCount()),
		MemoryMiB: int64(intent.Resources.GetMemoryMib()),
	}); err != nil {
		return fmt.Errorf("update resources: %w", err)
	}

	sandbox.Resources = intent.Resources
	intent.Resources = nil
	return nil
}

// snapshot triggers a CreateSnapshot on the microVM, persists the
// resulting artifacts to durable storage, and registers a Snapshot row
// in the api-server. Dispatched from reconcilePhase when
// Status.Phase == PHASE_SNAPSHOTTING; the api-server stamps Intent.Phase
// with the saved phase to return to, and the shared post-action code in
// reconcilePhase restores it after we return.
//
// The description carried on Intent.StartSnapshot is forwarded to the
// api-server's Snapshot.Metadata.Description so it surfaces in
// ListSnapshots / GetSnapshot results.
//
// Idempotency: each step is replay-safe. vm.CreateSnapshot and
// snapshotStore.Put are individually idempotent on the snapshot id, and
// the api-server's CreateSnapshot is digest-idempotent on the canonical
// Snapshot body. A retry after a partial failure (e.g. local Put
// succeeded, RPC failed) re-issues the same body and converges.
func (r *Reconciler) snapshot(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	vm, err := r.lookup(sandbox.GetMetadata().GetId(), "create snapshot")
	if err != nil {
		return err
	}

	localRef, err := vm.CreateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	if err := r.snapshotStore.Put(ctx, localRef); err != nil {
		return fmt.Errorf("persist snapshot: %w", err)
	}

	if r.snapshotClient != nil {
		snap := &controlplanev1alpha1.Snapshot{
			Metadata: &controlplanev1alpha1.SnapshotMeta{
				Id:          localRef.SnapshotID,
				Namespace:   sandbox.GetMetadata().GetNamespace(),
				Description: sandbox.GetIntent().GetStartSnapshot().GetDescription(),
			},
			Sandbox: &controlplanev1alpha1.SandboxRef{Id: sandbox.GetMetadata().GetId()},
		}
		if _, err := r.snapshotClient.CreateSnapshot(ctx,
			&controlplanev1alpha1.CreateSnapshotRequest{Snapshot: snap}); err != nil {
			return fmt.Errorf("register snapshot in api-server: %w", err)
		}
	}

	sandbox.GetIntent().StartSnapshot = nil
	sandbox.LastSnapshot = &controlplanev1alpha1.SnapshotOutput{
		SnapshotId: localRef.SnapshotID,
		CreatedAt:  timestamppb.Now(),
	}
	return nil
}

// lookup returns the MicroVM indexed under id or wraps a context-rich
// error when none exists.
func (r *Reconciler) lookup(id, op string) (firecracker.MicroVM, error) {
	vm := r.provider.GetMicroVM(id)
	if vm == nil {
		return nil, fmt.Errorf("%s: microvm %q not found", op, id)
	}
	return vm, nil
}

// buildCreateInput composes a CreateMicroVMInput from the sandbox's
// intrinsic resources and the daemon-level template constants in
// Bundle.MicroVM. Asset paths are resolved against Bundle.AssetsDir.
func (r *Reconciler) buildCreateInput(sandbox *controlplanev1alpha1.Sandbox, nsh network.NamespaceHandle) *firecracker.CreateMicroVMInput {
	tmpl := r.config.MicroVM
	kernelJail := tmpl.KernelJailPath
	rootfsJail := tmpl.RootfsJailPath
	logJail := tmpl.LogJailPath
	vsockUDSJail := tmpl.VsockUDSJailPath

	fcConfig := &firecracker.Config{
		BootSource: &models.BootSource{
			KernelImagePath: &kernelJail,
			BootArgs:        buildBootArgs(tmpl, nsh),
		},
		Drives: []*models.Drive{
			{
				DriveID:      new("rootfs"),
				PathOnHost:   rootfsJail,
				IsRootDevice: new(true),
				IsReadOnly:   false,
			},
		},
		MachineConfig: &models.MachineConfiguration{
			VcpuCount:  new(int64(sandbox.GetResources().GetVcpuCount())),
			MemSizeMib: new(int64(sandbox.GetResources().GetMemoryMib())),
		},
		NetworkInterfaces: []*models.NetworkInterface{
			{
				IfaceID:     new("eth0"),
				HostDevName: new(nsh.TapDeviceName),
				GuestMac:    tmpl.GuestMAC,
			},
		},
		Logger: &models.Logger{
			LogPath: logJail,
			Level:   conv.Pointer(models.LoggerLevelInfo),
		},
		Vsock: &models.Vsock{
			GuestCid: new(tmpl.GuestCID),
			UdsPath:  new(vsockUDSJail),
		},
	}

	return &firecracker.CreateMicroVMInput{
		ID:             sandbox.GetMetadata().GetId(),
		Network:        nsh,
		Config:         fcConfig,
		AgentVsockPort: tmpl.AgentVsockPort,
		Assets: []firecracker.Asset{
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.KernelFile), JailPath: kernelJail},
			// Writable because the rootfs drive is mounted rw — the
			// jailed firecracker uid needs write permission on the
			// staged copy.
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.RootfsFile), JailPath: rootfsJail, Writable: true},
		},
	}
}

// buildBootArgs assembles the kernel command line. The ip= parameter
// drives the in-guest agent's network stack: address, gateway, mask,
// device, off-DHCP. init= names the agent binary execed by the kernel
// as PID 1; agent.port= tells the agent which vsock port to listen on.
func buildBootArgs(tmpl *config.MicroVM, nsh network.NamespaceHandle) string {
	return fmt.Sprintf(
		"console=ttyS0 reboot=k panic=1 pci=off ip=%s::%s:255.255.255.252::eth0:off init=%s agent.port=%d",
		nsh.GuestIP, nsh.TapIP, tmpl.AgentInitPath, tmpl.AgentVsockPort,
	)
}
