package reconciler

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	fcmocks "golang.nuinfra.net/daemon/pkg/firecracker/mocks"
	"golang.nuinfra.net/daemon/pkg/network"
	netmocks "golang.nuinfra.net/daemon/pkg/network/mocks"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	snapmocks "golang.nuinfra.net/daemon/pkg/snapshot/mocks"
)

// testBundle returns a config.Bundle with the daemon defaults
// pre-applied so tests don't need to spell out every field.
func testBundle() *config.Bundle {
	return &config.Bundle{
		AssetsDir: "/assets",
		Firecracker: &config.Firecracker{
			BinaryName:       "firecracker",
			JailerBinaryName: "jailer",
			JailUID:          1000,
			JailGID:          1000,
			ChrootBaseDir:    "/srv/jailer",
		},
		MicroVM: &config.MicroVM{
			KernelFile:       "vmlinux",
			RootfsFile:       "rootfs.ext4",
			KernelJailPath:   "/vmlinux",
			RootfsJailPath:   "/rootfs.ext4",
			LogJailPath:      "/firecracker.log",
			VsockUDSJailPath: "/v.sock",
			AgentInitPath:    "/init",
			AgentVsockPort:   1024,
			GuestCID:         3,
			GuestMAC:         "06:00:AC:10:00:02",
		},
	}
}

// fixtureSandbox returns a sandbox with id, namespace, and intrinsic
// resources populated. Callers attach intent / status as needed.
func fixtureSandbox(id string) *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{Id: id, Namespace: "test", Version: 1},
		Resources: &cpv1.Resources{
			VcpuCount: 2,
			MemoryMib: 1024,
		},
		Status: &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_PENDING},
	}
}

// fixtureNamespaceHandle returns a deterministic NamespaceHandle for a
// given index. Values mirror the smoke test's runtime layout closely
// enough that boot args / net interface assertions are meaningful.
func fixtureNamespaceHandle(index int) network.NamespaceHandle { //nolint:unparam // index is meaningful for future tests asserting non-zero values
	return network.NamespaceHandle{
		Index:         index,
		NamespaceName: "vm-ns0",
		TapDeviceName: "vm-tap0",
		TapIP:         netip.MustParseAddr("172.16.0.1"),
		GuestIP:       netip.MustParseAddr("172.16.0.2"),
		HostVethName:  "vm-veth0",
		HostVethIP:    netip.MustParseAddr("169.254.0.1"),
	}
}

// stubLocalRefWithFiles builds a snapshot.LocalReference with mem/state
// files materialized on disk so Store.Put can os.Open them without error.
// The chroot lives under t.TempDir() and is cleaned up automatically.
func stubLocalRefWithFiles(t *testing.T, snapshotID string) *snapshot.LocalReference {
	t.Helper()
	chroot := t.TempDir()
	memPath := filepath.Join(chroot, snapshot.MemFilePath(snapshotID))
	statePath := filepath.Join(chroot, snapshot.StateFilePath(snapshotID))
	require.NoError(t, os.MkdirAll(filepath.Dir(memPath), 0o755))
	require.NoError(t, os.WriteFile(memPath, []byte("mem"), 0o600))
	require.NoError(t, os.WriteFile(statePath, []byte("state"), 0o600))
	return &snapshot.LocalReference{
		SnapshotID:    snapshotID,
		MemFilePath:   memPath,
		StateFilePath: statePath,
	}
}

func newReconcilerFixture(t *testing.T) (*Reconciler, *fcmocks.MockProvider, *netmocks.MockDriver, *fcmocks.MockMicroVM, *snapmocks.MockDurableStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	provider := fcmocks.NewMockProvider(ctrl)
	driver := netmocks.NewMockDriver(ctrl)
	vm := fcmocks.NewMockMicroVM(ctrl)
	durableStore := snapmocks.NewMockDurableStore(ctrl)
	r := New(testBundle(), provider, driver, snapshot.NewStore(durableStore))
	return r, provider, driver, vm, durableStore
}

func TestReconcile_NilIntentIsNoOp(t *testing.T) {
	r, _, _, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-1")
	sandbox.Intent = nil

	require.NoError(t, r.Reconcile(context.Background(), sandbox))
	assert.Nil(t, sandbox.Intent, "Intent must remain nil")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PENDING, sandbox.GetStatus().GetPhase(),
		"Status must be untouched when there is nothing to reconcile")
}

func TestReconcile_EmptyIntentClearsItButLeavesStatusAlone(t *testing.T) {
	// Intent != nil but every field is the zero value: no work, but the
	// reconciler still flips Intent back to nil (the "converged" signal).
	r, _, _, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-empty")
	sandbox.Intent = &cpv1.Intent{}

	require.NoError(t, r.Reconcile(context.Background(), sandbox))
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_BootCreatesMicroVMAndUpdatesStatus(t *testing.T) {
	r, provider, driver, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-boot")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-boot").Return(nil),
		provider.EXPECT().Len().Return(0),
		driver.EXPECT().Provision(0).Return(nsh, nil),
		provider.EXPECT().
			CreateMicroVM(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *firecracker.CreateMicroVMInput) (firecracker.MicroVM, error) {
				assert.Equal(t, "sb-boot", input.ID)
				assert.Equal(t, nsh, input.Network)
				assert.Equal(t, uint32(1024), input.AgentVsockPort)
				require.Len(t, input.Assets, 2)
				assert.Equal(t, "/assets/vmlinux", input.Assets[0].SourcePath)
				assert.Equal(t, "/vmlinux", input.Assets[0].JailPath)
				assert.Equal(t, "/assets/rootfs.ext4", input.Assets[1].SourcePath)
				assert.True(t, input.Assets[1].Writable, "rootfs must be staged writable")
				require.NotNil(t, input.Config)
				require.NotNil(t, input.Config.MachineConfig)
				assert.Equal(t, int64(2), *input.Config.MachineConfig.VcpuCount)
				assert.Equal(t, int64(1024), *input.Config.MachineConfig.MemSizeMib)
				return nil, nil
			}),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))

	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.NotEmpty(t, sandbox.GetStatus().GetMessage())
	assert.Nil(t, sandbox.Intent, "Intent must be cleared after a successful reconcile")
}

func TestReconcile_BootIsIdempotentWhenVMAlreadyIndexed(t *testing.T) {
	r, provider, driver, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-already")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provider.EXPECT().GetMicroVM("sb-already").Return(vm)
	// No network/provision calls — idempotent fast path.
	driver.EXPECT().Provision(gomock.Any()).Times(0)
	provider.EXPECT().CreateMicroVM(gomock.Any(), gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_BootRollsBackNetworkOnCreateFailure(t *testing.T) {
	r, provider, driver, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-rollback")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	bootErr := errors.New("jailer exploded")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-rollback").Return(nil),
		provider.EXPECT().Len().Return(0),
		driver.EXPECT().Provision(0).Return(nsh, nil),
		provider.EXPECT().CreateMicroVM(ctx, gomock.Any()).Return(nil, bootErr),
		driver.EXPECT().Deprovision(0).Return(nil),
	)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, bootErr)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PENDING, sandbox.GetStatus().GetPhase(),
		"Status must not advance when boot fails")
	require.NotNil(t, sandbox.Intent)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetIntent().GetPhase(),
		"Intent must be preserved so the next reconcile retries")
}

func TestReconcile_BootSurfacesNetworkProvisionFailure(t *testing.T) {
	r, provider, driver, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-netfail")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provErr := errors.New("netlink ENOPERM")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-netfail").Return(nil),
		provider.EXPECT().Len().Return(0),
		driver.EXPECT().Provision(0).Return(network.NamespaceHandle{}, provErr),
	)
	provider.EXPECT().CreateMicroVM(gomock.Any(), gomock.Any()).Times(0)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, provErr)
}

func TestReconcile_PauseCallsMicroVMPause(t *testing.T) {
	r, provider, _, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	// Under the new state machine the api-server has already moved
	// status to PAUSING (in-progress marker) and intent to PAUSED
	// (terminal target) by the time the reconciler sees the event.
	sandbox := fixtureSandbox("sb-pause")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_PAUSING
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_PAUSED}

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-pause").Return(vm),
		vm.EXPECT().Pause(ctx).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, sandbox.GetStatus().GetPhase(),
		"closing write must converge status to intent.phase")
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_PauseFailsWhenVMNotIndexed(t *testing.T) {
	r, provider, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-ghost")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_PAUSING
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_PAUSED}

	provider.EXPECT().GetMicroVM("sb-ghost").Return(nil)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sb-ghost")
}

func TestReconcile_ResumeCallsMicroVMResume(t *testing.T) {
	r, provider, _, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	// Resume: status=RESUMING, intent=RUNNING. There is no PHASE_RESUMED;
	// the closing write converges status to RUNNING, identical to a
	// freshly booted VM.
	sandbox := fixtureSandbox("sb-resume")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_RESUMING
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-resume").Return(vm),
		vm.EXPECT().Resume(ctx).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase(),
		"a successful resume converges status to RUNNING")
}

func TestReconcile_DeleteReadsNetworkIndexFromVMAndDeprovisions(t *testing.T) {
	// Models the cross-restart case: the Reconciler instance issuing
	// delete is brand new; the network index for the VM lives in the
	// firecracker provider's recovered MicroVM, not on the Reconciler.
	r, provider, driver, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-life")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_DELETING
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_DELETED}

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-life").Return(vm),
		vm.EXPECT().NetworkIndex().Return(7),
		provider.EXPECT().DeleteMicroVM(ctx, "sb-life").Return(nil),
		driver.EXPECT().Deprovision(7).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETED, sandbox.GetStatus().GetPhase(),
		"closing write must converge status to intent.phase=DELETED")
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_DeleteIsIdempotentForUnknownVMs(t *testing.T) {
	r, provider, driver, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-already-gone")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_DELETING
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_DELETED}

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-already-gone").Return(nil),
		provider.EXPECT().DeleteMicroVM(ctx, "sb-already-gone").Return(firecracker.ErrVMNotFound),
	)
	driver.EXPECT().Deprovision(gomock.Any()).Times(0) // network index unknown, must not deprovision

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETED, sandbox.GetStatus().GetPhase())
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_UpdateResourcesAppliesAndMirrors(t *testing.T) {
	r, provider, _, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-resize")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_RUNNING
	sandbox.Intent = &cpv1.Intent{
		Resources: &cpv1.Resources{VcpuCount: 4, MemoryMib: 2048},
	}

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-resize").Return(vm),
		vm.EXPECT().UpdateResources(ctx, firecracker.UpdateResourcesInput{
			VCPUCount: 4,
			MemoryMiB: 2048,
		}).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, uint32(4), sandbox.GetResources().GetVcpuCount(),
		"sandbox.Resources must mirror the applied intent")
	assert.Equal(t, uint64(2048), sandbox.GetResources().GetMemoryMib())
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_UpdateResourcesNoOpWhenIntentMatchesSandbox(t *testing.T) {
	// Event redelivered after a previous successful update: Intent.Resources
	// matches sandbox.Resources, so the reconciler must clear the intent
	// without touching firecracker.
	r, provider, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-resize-noop")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_RUNNING
	sandbox.Resources = &cpv1.Resources{VcpuCount: 4, MemoryMib: 2048}
	sandbox.Intent = &cpv1.Intent{
		Resources: &cpv1.Resources{VcpuCount: 4, MemoryMib: 2048},
	}

	// No GetMicroVM, no UpdateResources — the fast path skips the lookup.
	provider.EXPECT().GetMicroVM(gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Nil(t, sandbox.Intent)
	assert.Equal(t, uint32(4), sandbox.GetResources().GetVcpuCount())
	assert.Equal(t, uint64(2048), sandbox.GetResources().GetMemoryMib())
}

func TestReconcile_UpdateResourcesFailureKeepsIntent(t *testing.T) {
	r, provider, _, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-update-fail")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_RUNNING
	sandbox.Intent = &cpv1.Intent{
		Resources: &cpv1.Resources{VcpuCount: 8, MemoryMib: 4096},
	}

	apiErr := errors.New("firecracker rejected resize")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-update-fail").Return(vm),
		vm.EXPECT().UpdateResources(ctx, gomock.Any()).Return(apiErr),
	)

	err := r.Reconcile(ctx, sandbox)
	require.ErrorIs(t, err, apiErr)
	assert.Equal(t, uint32(2), sandbox.GetResources().GetVcpuCount(),
		"sandbox.Resources must not be mutated when UpdateResources fails")
	require.NotNil(t, sandbox.Intent)
	require.NotNil(t, sandbox.GetIntent().GetResources(),
		"Intent.Resources must be preserved for retry")
}

// TestReconcile_SnapshotRestoresRunningPhase exercises the
// happy path from a sandbox that was running when the snapshot
// request arrived. The api-server stamps Status.Phase = SNAPSHOTTING
// and Intent.Phase = RUNNING (the saved phase to restore); the
// reconciler runs the snapshot action and converges Status.Phase
// back to RUNNING.
func TestReconcile_SnapshotRestoresRunningPhase(t *testing.T) {
	r, provider, _, vm, durableStore := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-snap")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_SNAPSHOTTING
	sandbox.Intent = &cpv1.Intent{
		Phase:         cpv1.SandboxStatus_PHASE_RUNNING,
		StartSnapshot: &cpv1.StartSnapshotInput{Description: "before-deploy"},
	}

	localRef := stubLocalRefWithFiles(t, "snap-1")

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-snap").Return(vm),
		vm.EXPECT().CreateSnapshot(ctx).Return(localRef, nil),
		durableStore.EXPECT().
			Put(ctx, "snap-1", gomock.Any()).
			Return(nil),
	)

	before := time.Now()
	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Nil(t, sandbox.Intent)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase(),
		"after snapshot Status.Phase must be restored to the saved phase")
	assert.Equal(t, "Snapshot created", sandbox.GetStatus().GetMessage())

	require.NotNil(t, sandbox.GetLastSnapshot())
	assert.Equal(t, "snap-1", sandbox.GetLastSnapshot().GetSnapshotId())
	assert.WithinRange(t, sandbox.GetLastSnapshot().GetCreatedAt().AsTime(),
		before, time.Now(),
		"LastSnapshot.CreatedAt must be stamped during the reconcile")
}

// TestReconcile_SnapshotRestoresPausedPhase verifies the
// reconciler routes a snapshot triggered on a paused sandbox back to
// PAUSED rather than implicitly resuming. The api-server sets
// Intent.Phase = PAUSED in that case.
func TestReconcile_SnapshotRestoresPausedPhase(t *testing.T) {
	r, provider, _, vm, durableStore := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-snap-paused")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_SNAPSHOTTING
	sandbox.Intent = &cpv1.Intent{
		Phase:         cpv1.SandboxStatus_PHASE_PAUSED,
		StartSnapshot: &cpv1.StartSnapshotInput{},
	}

	localRef := stubLocalRefWithFiles(t, "snap-paused")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-snap-paused").Return(vm),
		vm.EXPECT().CreateSnapshot(ctx).Return(localRef, nil),
		durableStore.EXPECT().Put(ctx, "snap-paused", gomock.Any()).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, sandbox.GetStatus().GetPhase(),
		"snapshot of a paused sandbox must converge back to PAUSED, not RUNNING")
	assert.Nil(t, sandbox.Intent)
}

// TestReconcile_SnapshotDropsDescription pins the current behavior:
// the daemon does not forward the user-supplied Description into
// LastSnapshot or anywhere else. The day we want to forward it (into
// the snapshot manifest, an S3 object tag, etc.) this test must be
// updated explicitly so the change is intentional.
func TestReconcile_SnapshotDropsDescription(t *testing.T) {
	r, provider, _, vm, durableStore := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-snap-desc")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_SNAPSHOTTING
	sandbox.Intent = &cpv1.Intent{
		Phase:         cpv1.SandboxStatus_PHASE_RUNNING,
		StartSnapshot: &cpv1.StartSnapshotInput{Description: "pre-deploy snapshot"},
	}

	localRef := stubLocalRefWithFiles(t, "snap-desc")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-snap-desc").Return(vm),
		vm.EXPECT().CreateSnapshot(ctx).Return(localRef, nil),
		durableStore.EXPECT().Put(ctx, "snap-desc", gomock.Any()).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))

	// Description currently goes nowhere — neither LastSnapshot nor any
	// proto field carries it. When we wire it through, expand the
	// LastSnapshot proto and update this assertion.
	require.NotNil(t, sandbox.GetLastSnapshot())
	assert.Equal(t, "snap-desc", sandbox.GetLastSnapshot().GetSnapshotId())
}

// TestReconcile_CombinedIntentsAreAppliedInOrderAndCleared covers a
// boot + resize fanout in a single reconcile. StartSnapshot is no
// longer combinable with a phase transition — the api-server rejects
// StartSnapshot unless saved Status.Phase ∈ {RUNNING, PAUSED}, and
// the snapshot itself flows through PHASE_SNAPSHOTTING as its own
// transitional state.
func TestReconcile_CombinedIntentsAreAppliedInOrderAndCleared(t *testing.T) {
	r, provider, driver, vm, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-combined")
	sandbox.Intent = &cpv1.Intent{
		Phase:     cpv1.SandboxStatus_PHASE_RUNNING,
		Resources: &cpv1.Resources{VcpuCount: 4, MemoryMib: 4096},
	}

	nsh := fixtureNamespaceHandle(0)
	gomock.InOrder(
		// Boot.
		provider.EXPECT().GetMicroVM("sb-combined").Return(nil),
		provider.EXPECT().Len().Return(0),
		driver.EXPECT().Provision(0).Return(nsh, nil),
		provider.EXPECT().CreateMicroVM(ctx, gomock.Any()).Return(nil, nil),
		// Resize.
		provider.EXPECT().GetMicroVM("sb-combined").Return(vm),
		vm.EXPECT().UpdateResources(ctx, firecracker.UpdateResourcesInput{
			VCPUCount: 4, MemoryMiB: 4096,
		}).Return(nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.Equal(t, uint32(4), sandbox.GetResources().GetVcpuCount())
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_PhaseFailureSkipsResources(t *testing.T) {
	r, provider, driver, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-phase-bad")
	sandbox.Intent = &cpv1.Intent{
		Phase:     cpv1.SandboxStatus_PHASE_RUNNING,
		Resources: &cpv1.Resources{VcpuCount: 4, MemoryMib: 4096},
	}

	bootErr := errors.New("kernel panic during boot")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-phase-bad").Return(nil),
		provider.EXPECT().Len().Return(0),
		driver.EXPECT().Provision(0).Return(fixtureNamespaceHandle(0), nil),
		provider.EXPECT().CreateMicroVM(ctx, gomock.Any()).Return(nil, bootErr),
		driver.EXPECT().Deprovision(0).Return(nil),
	)

	err := r.Reconcile(ctx, sandbox)
	require.ErrorIs(t, err, bootErr)
	require.NotNil(t, sandbox.Intent, "Intent stays set so the retry resumes")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetIntent().GetPhase())
	require.NotNil(t, sandbox.GetIntent().GetResources())
}

func TestReconcile_UnsupportedPhaseReturnsError(t *testing.T) {
	r, _, _, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-unsupported")
	// PHASE_PAUSING is a transitional state the api-server never sets
	// as an intent — feeding it to the reconciler must surface an
	// error rather than silently accept it.
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_PAUSING}

	err := r.Reconcile(context.Background(), sandbox)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported intent phase")
}
