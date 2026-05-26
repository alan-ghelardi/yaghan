// Package reconciler converges the actual MicroVM state on a host with
// the desired state expressed in a Sandbox proto's Intent field. It is
// invoked once per Sandbox event received from the control plane.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/config"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/dns"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/firecracker"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/network"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/snapshot"
	"github.com/alan-ghelardi/yaghan/firecracker-client/models"
	"github.com/go-openapi/swag/conv"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
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
	dnsPolicies    *dns.PolicyStore
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
// dnsPolicies, when non-nil, receives per-sandbox domain_names
// policies at Provision and is cleared at Deprovision; a nil value
// disables the in-daemon DNS responder integration (acceptable in
// tests and on daemons configured without the dns server).
func New(
	config *config.Bundle,
	provider firecracker.Provider,
	driver network.Driver,
	dnsPolicies *dns.PolicyStore,
	snapshotStore *snapshot.Store,
	snapshotClient controlplanev1alpha1.SnapshotServiceClient,
) *Reconciler {
	return &Reconciler{
		config:         config,
		provider:       provider,
		driver:         driver,
		dnsPolicies:    dnsPolicies,
		snapshotStore:  snapshotStore,
		snapshotClient: snapshotClient,
	}
}

// Reconcile converges sandbox towards its Intent. The only remaining
// intent step is the phase transition (boot / pause / resume / delete /
// snapshot); the snapshot sub-step is dispatched from inside
// reconcilePhase via the SNAPSHOTTING status. Once it runs successfully
// the entire Intent is cleared.
//
// A nil Intent is treated as a no-op. A failed step returns its error
// immediately so the controller can mark the sandbox failed and surface
// the message to the caller.
//
// All operations are designed to be idempotent.
func (r *Reconciler) Reconcile(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	if sandbox.GetIntent() == nil {
		return nil
	}

	if err := r.reconcilePhase(ctx, sandbox); err != nil {
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
	case controlplanev1alpha1.SandboxStatus_PHASE_FAILED,
		controlplanev1alpha1.SandboxStatus_PHASE_DELETED:
		// Terminal state. The daemon cannot drive this sandbox
		// forward — PHASE_FAILED carries the original reconcile
		// error in Status.Message (markFailed in controller.go);
		// PHASE_DELETED means the teardown completed in a prior
		// pass. Without this branch the default below treats either
		// case as "unsupported", masks the original error in daemon
		// logs, and loops forever because the controller's queue
		// never drains.
		//
		// Surface the primary error one last time at WARN, then
		// clear Intent.Phase so the outer Reconcile zeroes Intent
		// entirely. The controller's processItem will diff and send
		// an UpdateSandbox with the cleared Intent; subsequent
		// api-server events for this sandbox land here with
		// Intent.Phase=UNSPECIFIED and short-circuit at the top of
		// reconcilePhase.
		if msg := sandbox.GetStatus().GetMessage(); msg != "" {
			ctxzap.Extract(ctx).Warn("reconciler: sandbox in terminal state, abandoning",
				zap.String("sandbox.id", sandbox.GetMetadata().GetId()),
				zap.String("status.phase", sandbox.GetStatus().GetPhase().String()),
				zap.String("status.message", msg))
		}
		intent.Phase = controlplanev1alpha1.SandboxStatus_PHASE_UNSPECIFIED
		return nil
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

// snapshotCacheDirName is the subdirectory under
// Bundle.Firecracker.ChrootBaseDir where bootFromSnapshot downloads the
// snapshot artifacts retrieved from durable storage. Snapshot files
// are reusable across MicroVMs that load from the same snapshot id, so
// keeping them outside any single chroot lets firecracker.LoadSnapshot
// hard-link them into per-VM chroots zero-copy.
const snapshotCacheDirName = "snapshot-cache"

// boot ensures a MicroVM exists for the sandbox. It dispatches on the
// optional sandbox.Metadata.Source discriminator: when source carries
// a snapshot id we restore via LoadSnapshot, otherwise we boot fresh
// via CreateMicroVM.
func (r *Reconciler) boot(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	if snapshotID := sandbox.GetMetadata().GetSource().GetSnapshotId(); snapshotID != "" {
		return r.bootFromSnapshot(ctx, sandbox, snapshotID)
	}
	return r.bootFresh(ctx, sandbox)
}

// bootFresh creates a brand-new MicroVM from the sandbox's intrinsic
// resources and the daemon-level template constants. Idempotent: if
// the provider already has the VM indexed, bootFresh returns nil
// without touching the network driver or firecracker.
//
// The netns index comes from provider.NextNetNSIndex(), which sits
// strictly above any live VM's NetworkIndex — so the index handed to
// the driver never collides with an existing VM, even after deletes
// have left gaps in the population. The chosen index is persisted by
// the firecracker package alongside the chroot (meta.json) so a
// later delete (potentially after a daemon restart) can deprovision
// the matching namespace.
func (r *Reconciler) bootFresh(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	id := sandbox.GetMetadata().GetId()
	if vm := r.provider.GetMicroVM(id); vm != nil {
		return nil
	}
	if err := r.checkDNSPrereq(sandbox); err != nil {
		return err
	}

	index := r.provider.NextNetNSIndex()
	nsh, err := r.driver.Provision(index, translateEgressPolicy(ctx, sandbox))
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}
	r.registerDNSPolicy(ctx, sandbox, nsh)

	input := r.buildCreateInput(sandbox, nsh)
	if _, err := r.provider.CreateMicroVM(ctx, input); err != nil {
		// Roll the network back so we don't leak an orphaned namespace
		// if the VM never came up.
		r.removeDNSPolicy(nsh.NamespaceVethIP)
		_ = r.driver.Deprovision(index)
		return fmt.Errorf("create microvm: %w", err)
	}
	return nil
}

// bootFromSnapshot provisions a MicroVM by restoring the snapshot
// referenced by snapshotID. The order of operations is:
//
//  1. Fast path — if the provider already has the VM indexed, the
//     restore is already done; return immediately.
//  2. Reject early when the daemon has no snapshot store configured;
//     the sandbox is unbootable on this host.
//  3. Download the snapshot artifacts into the shared cache via
//     [snapshot.Store.Load]. The call is idempotent so subsequent
//     boots from the same snapshot are zero-cost.
//  4. Provision the network. Done after the download so a missing
//     snapshot fails before we touch the host's network stack.
//  5. Hand control to [firecracker.Provider.LoadSnapshot]; it stages
//     the cached files into the new chroot, starts firecracker
//     without --config-file, issues the load RPC with resume_vm=true,
//     and waits for Running. Network is rolled back on failure.
//
// Resource alignment is not a step here: the api-server enforces that
// snapshot-sourced sandboxes inherit the snapshot's resources, so by
// the time the reconcile runs sandbox.Resources already matches what
// the snapshot will boot with. Firecracker's PATCH /machine-config is
// pre-boot only — there is no in-band resize path on restore anyway.
//
// Known limitation: today's snapshot artifact is mem+state only. The
// drives the snapshot's state file references must still exist in the
// new chroot, so bootFromSnapshot stages a PRISTINE rootfs from the
// daemon's assets dir. Stateful workloads that wrote to rootfs between
// boot and snapshot will observe filesystem inconsistencies on
// restore. Extending CreateSnapshot to also persist post-snapshot
// drive files is a separate iteration.
func (r *Reconciler) bootFromSnapshot(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox, snapshotID string) error {
	id := sandbox.GetMetadata().GetId()
	if r.provider.GetMicroVM(id) != nil {
		return nil
	}

	if r.snapshotStore == nil {
		return fmt.Errorf(
			"sandbox %q references snapshot %q but this daemon was started without snapshot storage configured",
			id, snapshotID)
	}
	if err := r.checkDNSPrereq(sandbox); err != nil {
		return err
	}

	stagingDir := filepath.Join(r.config.Firecracker.ChrootBaseDir, snapshotCacheDirName)
	localRef, err := r.snapshotStore.Load(ctx, stagingDir, snapshotID)
	if err != nil {
		return fmt.Errorf("load snapshot %q from durable storage: %w", snapshotID, err)
	}

	index := r.provider.NextNetNSIndex()
	nsh, err := r.driver.Provision(index, translateEgressPolicy(ctx, sandbox))
	if err != nil {
		return fmt.Errorf("provision network: %w", err)
	}
	r.registerDNSPolicy(ctx, sandbox, nsh)

	input := r.buildLoadSnapshotInput(sandbox, nsh, localRef)
	if _, err := r.provider.LoadSnapshot(ctx, input); err != nil {
		r.removeDNSPolicy(nsh.NamespaceVethIP)
		_ = r.driver.Deprovision(index)
		return fmt.Errorf("restore microvm from snapshot %q: %w", snapshotID, err)
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
		// Evict the DNS policy before tearing the namespace down so
		// any in-flight resolver query lands on an empty store and
		// gets REFUSED rather than racing against a half-deleted
		// firewall set.
		if srcIP, ipErr := network.NamespaceVethIPFor(networkIndex); ipErr == nil {
			r.removeDNSPolicy(srcIP)
		}
		if err := r.driver.Deprovision(networkIndex); err != nil {
			return fmt.Errorf("deprovision network: %w", err)
		}
	}
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
			// Capture the resources the source sandbox is running with so
			// the api-server can later stamp them onto any sandbox
			// derived from this snapshot. Firecracker bakes vCPU /
			// memory into the snapshot state and forbids changing them
			// on restore, so this field — not user input — is the
			// authoritative source of resources for derived sandboxes.
			Resources: sandbox.GetResources(),
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
			BootArgs:        buildBootArgs(tmpl, sandbox, nsh, r.config.Network),
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
		RootfsDiskMiB:  resolveRootfsDiskMiB(sandbox, tmpl),
		Assets: []firecracker.Asset{
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.KernelFile), JailPath: kernelJail},
			// Writable because the rootfs drive is mounted rw — the
			// jailed firecracker uid needs write permission on the
			// staged copy.
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.RootfsFile), JailPath: rootfsJail, Writable: true},
		},
	}
}

// resolveRootfsDiskMiB picks the rootfs disk size for the fresh-boot
// path. Order of precedence:
//
//  1. sandbox.Resources.disk_mib when the spec sets it (proto bounds
//     keep this safe — proto-level validation rejects values out of
//     [256, 1048576] before we see them).
//  2. tmpl.DefaultRootfsDiskMiB when the spec leaves disk_mib at 0.
//
// Returns 0 if both are zero, which the firecracker layer treats as
// "no resize, leave the staged file at base image size" — useful for
// tests and migrations rather than a production happy path.
func resolveRootfsDiskMiB(sandbox *controlplanev1alpha1.Sandbox, tmpl *config.MicroVM) int64 {
	if v := sandbox.GetResources().GetDiskMib(); v > 0 {
		return int64(v)
	}
	return tmpl.DefaultRootfsDiskMiB
}

// buildLoadSnapshotInput composes a LoadSnapshotInput for restoring a
// MicroVM from snapshot artifacts. It mirrors buildCreateInput on the
// fields the firecracker package needs at runtime — Network, Assets,
// AgentVsockPort, VsockUDSPath — but skips the per-VM Config (the
// snapshot's state file carries that). The kernel asset is staged for
// symmetry with the fresh-boot chroot layout even though firecracker
// does not re-read it on restore; the rootfs is required because the
// snapshot's state file references it as a chroot-relative drive
// path. See the bootFromSnapshot doc for the known rootfs-staleness
// limitation.
func (r *Reconciler) buildLoadSnapshotInput(sandbox *controlplanev1alpha1.Sandbox, nsh network.NamespaceHandle, ref *snapshot.LocalReference) *firecracker.LoadSnapshotInput {
	tmpl := r.config.MicroVM
	return &firecracker.LoadSnapshotInput{
		ID:             sandbox.GetMetadata().GetId(),
		Network:        nsh,
		AgentVsockPort: tmpl.AgentVsockPort,
		LocalReference: ref,
		VsockUDSPath:   tmpl.VsockUDSJailPath,
		Assets: []firecracker.Asset{
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.KernelFile), JailPath: tmpl.KernelJailPath},
			{SourcePath: filepath.Join(r.config.AssetsDir, tmpl.RootfsFile), JailPath: tmpl.RootfsJailPath, Writable: true},
		},
	}
}

// translateEgressPolicy projects the control-plane EgressPolicy proto
// onto the driver-internal [network.EgressPolicy] shape the network
// driver consumes. Returns nil when the sandbox carries no policy —
// the driver interprets nil as unrestricted egress.
//
// All three target shapes (ip_addresses, cidr_blocks, domain_names)
// are passed through. The firewall consumes IPs/CIDRs immediately as
// static rules; domain_names sit on the firewall struct only as a
// signal that the per-sandbox dynamic infrastructure (ipset, resolver
// carve-out) should be provisioned. The actual domain matching
// happens later, per query, in the dns handler — see
// [Reconciler.registerDNSPolicy].
//
// IP/CIDR parse errors are bugs: the api-server's buf.validate rejects
// malformed entries before they reach the persistence layer, so a
// failure here means the proto wire was tampered with or validation
// drifted. We log and skip the offending entry rather than failing the
// whole boot, so a single corrupt value does not strand a sandbox.
func translateEgressPolicy(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) *network.EgressPolicy {
	p := sandbox.GetEgressPolicy()
	if p == nil {
		return nil
	}

	var (
		mode    network.EgressMode
		targets *controlplanev1alpha1.EgressTargets
	)
	switch {
	case p.GetAllow() != nil:
		mode, targets = network.EgressAllow, p.GetAllow()
	case p.GetDeny() != nil:
		mode, targets = network.EgressDeny, p.GetDeny()
	default:
		// oneof unset — treat as no policy.
		return nil
	}

	logger := ctxzap.Extract(ctx).With(
		zap.String("sandbox.id", sandbox.GetMetadata().GetId()),
	)

	policy := &network.EgressPolicy{Mode: mode}
	for _, raw := range targets.GetIpAddresses() {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			logger.Warn("reconciler: skipping malformed egress ip_address",
				zap.String("value", raw), zap.Error(err))
			continue
		}
		policy.IPs = append(policy.IPs, addr)
	}
	for _, raw := range targets.GetCidrBlocks() {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			logger.Warn("reconciler: skipping malformed egress cidr_block",
				zap.String("value", raw), zap.Error(err))
			continue
		}
		policy.CIDRs = append(policy.CIDRs, prefix)
	}
	policy.Domains = append(policy.Domains, targets.GetDomainNames()...)
	return policy
}

// registerDNSPolicy compiles the sandbox's domain_names matchers and
// hands the result to the in-daemon DNS responder so it can apply the
// per-sandbox verdict to incoming queries. No-ops in three cases:
//
//   - The reconciler was constructed without a PolicyStore (tests, or
//     a daemon without the dns server enabled).
//   - The sandbox carries no EgressPolicy at all.
//   - The policy carries IPs/CIDRs but no domain_names — there is
//     nothing for the responder to do, and skipping the registration
//     keeps the per-source-IP table small.
//
// A matcher-compile error is treated as a bug (the api-server's CEL
// validator rejects malformed entries upstream) and logged loudly,
// but the sandbox still boots — the firewall will enforce whatever
// IPs/CIDRs it received, and the worst case is the domain rules are
// silently ignored on this host.
func (r *Reconciler) registerDNSPolicy(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox, nsh network.NamespaceHandle) {
	if r.dnsPolicies == nil {
		return
	}
	p := sandbox.GetEgressPolicy()
	if p == nil {
		return
	}

	var (
		mode    dns.EgressMode
		targets *controlplanev1alpha1.EgressTargets
	)
	switch {
	case p.GetAllow() != nil:
		mode, targets = dns.ModeAllow, p.GetAllow()
	case p.GetDeny() != nil:
		mode, targets = dns.ModeDeny, p.GetDeny()
	default:
		return
	}
	domains := targets.GetDomainNames()
	if len(domains) == 0 {
		return
	}

	logger := ctxzap.Extract(ctx).With(
		zap.String("sandbox.id", sandbox.GetMetadata().GetId()),
		zap.Int("sandbox.index", nsh.Index),
	)
	matchers, err := dns.CompileMatchers(domains)
	if err != nil {
		logger.Error("reconciler: compile dns matchers (api-server validation drifted?); domain_names will be unenforced for this sandbox",
			zap.Error(err))
		return
	}
	r.dnsPolicies.Upsert(nsh.NamespaceVethIP, dns.SandboxDNSPolicy{
		Index:         nsh.Index,
		NamespaceName: nsh.NamespaceName,
		Mode:          mode,
		Matchers:      matchers,
	})
}

// removeDNSPolicy clears the per-sandbox entry from the policy store.
// No-op when the reconciler is running without a store (see
// [Reconciler.registerDNSPolicy]) or when srcIP is the zero value
// (e.g. the index lookup failed in the caller).
func (r *Reconciler) removeDNSPolicy(srcIP netip.Addr) {
	if r.dnsPolicies == nil || !srcIP.IsValid() {
		return
	}
	r.dnsPolicies.Remove(srcIP)
}

// checkDNSPrereq refuses to boot a sandbox whose policy references
// domain_names when the daemon is running without the in-process DNS
// responder. Silently dropping the domain_names rules would be a
// security regression — the api-server accepted the policy expecting
// the daemon to enforce it.
func (r *Reconciler) checkDNSPrereq(sandbox *controlplanev1alpha1.Sandbox) error {
	if !hasDomainPolicy(sandbox) {
		return nil
	}
	if r.dnsPolicies == nil {
		return fmt.Errorf(
			"sandbox %q egress policy references domain_names but this daemon was started without the DNS responder enabled",
			sandbox.GetMetadata().GetId())
	}
	return nil
}

// hasDomainPolicy reports whether the sandbox's egress policy
// references domain_names. Used by buildBootArgs to decide whether
// the guest's /etc/resolv.conf should point at the in-daemon resolver
// or at the daemon-wide default.
func hasDomainPolicy(sandbox *controlplanev1alpha1.Sandbox) bool {
	p := sandbox.GetEgressPolicy()
	if p == nil {
		return false
	}
	if a := p.GetAllow(); a != nil && len(a.GetDomainNames()) > 0 {
		return true
	}
	if d := p.GetDeny(); d != nil && len(d.GetDomainNames()) > 0 {
		return true
	}
	return false
}

// buildBootArgs assembles the kernel command line. The ip= parameter
// drives the in-guest agent's network stack: address, gateway, mask,
// device, off-DHCP. init= names the agent binary execed by the kernel
// as PID 1; agent.port= tells the agent which vsock port to listen on.
// agent.dns=, when present, is a comma-separated resolver list the
// agent writes into /etc/resolv.conf early in PID-1 init.
//
// When the sandbox's policy references domain_names, agent.dns is
// overridden to the daemon-wide responder address
// (network.dns.listen-ip) so DNS traffic terminates at the
// in-daemon responder. The override unconditionally wins over the
// daemon-wide network.guest-dns config — the policy would otherwise
// be unenforceable. The override is skipped when the responder is
// not configured (DNS section absent or disabled); the reconciler
// rejects domain-bearing sandboxes earlier in that case so we never
// hit this path with hasDomainPolicy=true.
func buildBootArgs(tmpl *config.MicroVM, sandbox *controlplanev1alpha1.Sandbox, nsh network.NamespaceHandle, netCfg *config.Network) string {
	base := fmt.Sprintf(
		"console=ttyS0 reboot=k panic=1 pci=off ip=%s::%s:255.255.255.252::eth0:off init=%s agent.port=%d",
		nsh.GuestIP, nsh.TapIP, tmpl.AgentInitPath, tmpl.AgentVsockPort,
	)
	switch {
	case hasDomainPolicy(sandbox) && netCfg != nil && netCfg.DNS != nil && netCfg.DNS.ListenIP != "":
		base += " agent.dns=" + netCfg.DNS.ListenIP
	case netCfg != nil && len(netCfg.GuestDNS) > 0:
		base += " agent.dns=" + strings.Join(netCfg.GuestDNS, ",")
	}
	return base
}
