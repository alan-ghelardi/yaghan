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
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	fcmocks "golang.nuinfra.net/daemon/pkg/firecracker/mocks"
	"golang.nuinfra.net/daemon/pkg/network"
	netmocks "golang.nuinfra.net/daemon/pkg/network/mocks"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	snapmocks "golang.nuinfra.net/daemon/pkg/snapshot/mocks"
	"google.golang.org/grpc"
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

func newReconcilerFixture(t *testing.T) (*Reconciler, *fcmocks.MockProvider, *netmocks.MockDriver, *fcmocks.MockMicroVM, *snapmocks.MockDurableStore, *cpmocks.MockSnapshotServiceClient) {
	t.Helper()
	ctrl := gomock.NewController(t)
	provider := fcmocks.NewMockProvider(ctrl)
	driver := netmocks.NewMockDriver(ctrl)
	vm := fcmocks.NewMockMicroVM(ctrl)
	durableStore := snapmocks.NewMockDurableStore(ctrl)
	snapshotClient := cpmocks.NewMockSnapshotServiceClient(ctrl)
	// Anchor ChrootBaseDir under t.TempDir() so bootFromSnapshot's
	// shared snapshot cache lives somewhere writable without touching
	// the host's /srv/jailer.
	bundle := testBundle()
	bundle.Firecracker.ChrootBaseDir = t.TempDir()
	r := New(bundle, provider, driver, snapshot.NewStore(durableStore), snapshotClient)
	return r, provider, driver, vm, durableStore, snapshotClient
}

// withSnapshotSource attaches a Metadata.Source.snapshot_id reference
// to a fixture sandbox. Mirrors the api-server side helper of the same
// name.
func withSnapshotSource(snapshotID string) func(*cpv1.Sandbox) {
	return func(sb *cpv1.Sandbox) {
		sb.Metadata.Source = &cpv1.SandboxSource{
			Reference: &cpv1.SandboxSource_SnapshotId{SnapshotId: snapshotID},
		}
	}
}

// stagedSnapshotPath returns the host path inside the daemon's shared
// snapshot cache where Store.Load writes the artifact for snapshotID.
// Used by tests that need to populate / assert on the cached files
// without going through the public Store API.
func stagedSnapshotPath(r *Reconciler, snapshotID string) (memPath, statePath string) {
	cache := filepath.Join(r.config.Firecracker.ChrootBaseDir, snapshotCacheDirName)
	memPath = filepath.Join(cache, snapshot.MemFilePath(snapshotID))
	statePath = filepath.Join(cache, snapshot.StateFilePath(snapshotID))
	return memPath, statePath
}

// expectSnapshotDownload primes the MockDurableStore so the next
// Store.Load(stagingDir, snapshotID) writes the supplied byte payloads
// into the cache. The handler does the same job as the real S3
// download — receive ContentsWriter, copy bytes into its WriterAts —
// just from in-memory data instead of from S3.
func expectSnapshotDownload(durableStore *snapmocks.MockDurableStore, snapshotID string, mem, state []byte) *gomock.Call {
	return durableStore.EXPECT().
		Get(gomock.Any(), snapshotID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, contents *snapshot.ContentsWriter) error {
			if _, err := contents.Memory.WriteAt(mem, 0); err != nil {
				return err
			}
			_, err := contents.State.WriteAt(state, 0)
			return err
		})
}

func TestReconcile_NilIntentIsNoOp(t *testing.T) {
	r, _, _, _, _, _ := newReconcilerFixture(t)

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
	r, _, _, _, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-empty")
	sandbox.Intent = &cpv1.Intent{}

	require.NoError(t, r.Reconcile(context.Background(), sandbox))
	assert.Nil(t, sandbox.Intent)
}

func TestReconcile_BootCreatesMicroVMAndUpdatesStatus(t *testing.T) {
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-boot")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-boot").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(0),
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
	r, provider, driver, vm, _, _ := newReconcilerFixture(t)
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
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-rollback")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	bootErr := errors.New("jailer exploded")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-rollback").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(0),
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
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-netfail")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provErr := errors.New("netlink ENOPERM")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-netfail").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(0),
		driver.EXPECT().Provision(0).Return(network.NamespaceHandle{}, provErr),
	)
	provider.EXPECT().CreateMicroVM(gomock.Any(), gomock.Any()).Times(0)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, provErr)
}

// TestReconcile_BootAfterDeleteUsesFreshIndex pins the bug fix: the
// reconciler must consult provider.NextNetNSIndex rather than Len
// when choosing the netns index. Before this change, a delete that
// shrank the live population would leave Len unchanged at the
// (delete-target's) value, and the next boot would collide with a
// surviving VM at that index. NextNetNSIndex sits strictly above
// every live VM, so no collision is possible.
func TestReconcile_BootAfterDeleteUsesFreshIndex(t *testing.T) {
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-post-delete")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	// Live population: VM at index 1 (e.g. sb-A at 0 was deleted,
	// leaving sb-B at 1 in the index). With the old Len()-based
	// allocator the next index would be 1 — colliding with sb-B.
	// NextNetNSIndex returns max+1 = 2 instead.
	const freshIndex = 2
	nsh := fixtureNamespaceHandle(freshIndex)

	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-post-delete").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(freshIndex),
		driver.EXPECT().
			Provision(freshIndex).
			DoAndReturn(func(idx int) (network.NamespaceHandle, error) {
				assert.Equal(t, freshIndex, idx,
					"reconciler must hand the driver the fresh index returned by NextNetNSIndex, not Len")
				return nsh, nil
			}),
		provider.EXPECT().
			CreateMicroVM(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *firecracker.CreateMicroVMInput) (firecracker.MicroVM, error) {
				assert.Equal(t, nsh, input.Network,
					"the namespace handle passed to CreateMicroVM must be the one bound to the fresh index")
				return nil, nil
			}),
	)
	// Critically, the driver MUST NOT be invoked with index 1 — that
	// would mean the reconciler walked back into Len's footsteps and
	// collided with the live sb-B at index 1.
	provider.EXPECT().GetMicroVM(gomock.Any()).Times(0)
	provider.EXPECT().Len().Times(0)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.Nil(t, sandbox.Intent)
}

// TestReconcile_BootFromSnapshot_HappyPath covers the snapshot-restore
// boot path end-to-end: the durable store is queried, the network is
// provisioned, and LoadSnapshot fires. Resource alignment is no longer
// a reconciler step — Firecracker's PATCH /machine-config is pre-boot
// only, so the api-server enforces that snapshot-sourced sandboxes
// inherit the snapshot's resources by the time the reconcile runs.
func TestReconcile_BootFromSnapshot_HappyPath(t *testing.T) {
	r, provider, driver, vm, durableStore, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-from-snap")
	withSnapshotSource("snap-A")(sandbox)
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-from-snap").Return(nil),
		expectSnapshotDownload(durableStore, "snap-A", []byte("mem"), []byte("state")),
		provider.EXPECT().NextNetNSIndex().Return(0),
		driver.EXPECT().Provision(0).Return(nsh, nil),
		provider.EXPECT().
			LoadSnapshot(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *firecracker.LoadSnapshotInput) (firecracker.MicroVM, error) {
				assert.Equal(t, "sb-from-snap", input.ID)
				assert.Equal(t, nsh, input.Network)
				require.NotNil(t, input.LocalReference)
				assert.Equal(t, "snap-A", input.LocalReference.SnapshotID)
				// LoadSnapshot is given the *staged* paths inside the
				// daemon's shared cache, not the original VM's chroot.
				memPath, statePath := stagedSnapshotPath(r, "snap-A")
				assert.Equal(t, memPath, input.LocalReference.MemFilePath)
				assert.Equal(t, statePath, input.LocalReference.StateFilePath)
				// VsockUDSPath and AgentVsockPort come from the daemon
				// template so the in-VM agent stays reachable.
				assert.Equal(t, "/v.sock", input.VsockUDSPath)
				assert.Equal(t, uint32(1024), input.AgentVsockPort)
				return vm, nil
			}),
	)
	// Describe must NOT be called — the reconciler trusts the api-server
	// to have aligned sandbox.Resources with the snapshot's at create
	// time, so a post-load round-trip is wasteful.
	vm.EXPECT().Describe(gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Nil(t, sandbox.Intent)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.Equal(t, "Sandbox is running", sandbox.GetStatus().GetMessage())
}

// TestReconcile_BootFromSnapshot_VMAlreadyExistsIsNoOp pins the retry
// path: when LoadSnapshot already succeeded in a prior pass, a
// re-delivered event finds the VM in the index and short-circuits. No
// download, no network, no LoadSnapshot, no Describe — the VM is up
// and there is nothing left to converge.
func TestReconcile_BootFromSnapshot_VMAlreadyExistsIsNoOp(t *testing.T) {
	r, provider, _, vm, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-retry")
	withSnapshotSource("snap-C")(sandbox)
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provider.EXPECT().GetMicroVM("sb-retry").Return(vm)
	vm.EXPECT().Describe(gomock.Any()).Times(0)
	provider.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
}

// TestReconcile_BootFromSnapshot_DurableStoreFailureLeavesNetworkUntouched
// pins the order-of-operations contract: the snapshot download
// happens before network provisioning so a missing snapshot does not
// leak host network resources.
func TestReconcile_BootFromSnapshot_DurableStoreFailureLeavesNetworkUntouched(t *testing.T) {
	r, provider, driver, _, durableStore, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-no-snap")
	withSnapshotSource("snap-missing")(sandbox)
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	downloadErr := errors.New("S3 NoSuchKey")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-no-snap").Return(nil),
		durableStore.EXPECT().
			Get(gomock.Any(), "snap-missing", gomock.Any()).
			Return(downloadErr),
	)
	driver.EXPECT().Provision(gomock.Any()).Times(0)
	provider.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Times(0)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, downloadErr)
	assert.Contains(t, err.Error(), "snap-missing",
		"error must surface the snapshot id the daemon failed to load")
}

// TestReconcile_BootFromSnapshot_LoadFailureRollsBackNetwork pins
// network teardown on a LoadSnapshot failure — we must not leak an
// orphaned namespace when the snapshot artifact downloaded but
// firecracker refused to restore.
func TestReconcile_BootFromSnapshot_LoadFailureRollsBackNetwork(t *testing.T) {
	r, provider, driver, _, durableStore, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-load-fail")
	withSnapshotSource("snap-D")(sandbox)
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	loadErr := errors.New("firecracker rejected snapshot")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-load-fail").Return(nil),
		expectSnapshotDownload(durableStore, "snap-D", []byte("mem"), []byte("state")),
		provider.EXPECT().NextNetNSIndex().Return(0),
		driver.EXPECT().Provision(0).Return(fixtureNamespaceHandle(0), nil),
		provider.EXPECT().LoadSnapshot(ctx, gomock.Any()).Return(nil, loadErr),
		driver.EXPECT().Deprovision(0).Return(nil),
	)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, loadErr)
}

// TestReconcile_BootFromSnapshot_FailsWithoutSnapshotStore pins the
// "daemon was started without snapshot storage" branch. The reconciler
// must short-circuit with a meaningful error rather than nil-deref'ing
// the snapshot store.
func TestReconcile_BootFromSnapshot_FailsWithoutSnapshotStore(t *testing.T) {
	t.Helper()
	ctrl := gomock.NewController(t)
	provider := fcmocks.NewMockProvider(ctrl)
	driver := netmocks.NewMockDriver(ctrl)
	snapshotClient := cpmocks.NewMockSnapshotServiceClient(ctrl)
	bundle := testBundle()
	bundle.Firecracker.ChrootBaseDir = t.TempDir()
	// Explicitly nil snapshot store — simulates a daemon configured
	// without Snapshots.S3.
	r := New(bundle, provider, driver, nil, snapshotClient)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-no-store")
	withSnapshotSource("snap-E")(sandbox)
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provider.EXPECT().GetMicroVM("sb-no-store").Return(nil)
	driver.EXPECT().Provision(gomock.Any()).Times(0)
	provider.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Times(0)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sb-no-store")
	assert.Contains(t, err.Error(), "snap-E")
	assert.Contains(t, err.Error(), "snapshot storage")
}

func TestReconcile_PauseCallsMicroVMPause(t *testing.T) {
	r, provider, _, vm, _, _ := newReconcilerFixture(t)
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
	r, provider, _, _, _, _ := newReconcilerFixture(t)
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
	r, provider, _, vm, _, _ := newReconcilerFixture(t)
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
	r, provider, driver, vm, _, _ := newReconcilerFixture(t)
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
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
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

// TestReconcile_SnapshotRestoresRunningPhase exercises the
// happy path from a sandbox that was running when the snapshot
// request arrived. The api-server stamps Status.Phase = SNAPSHOTTING
// and Intent.Phase = RUNNING (the saved phase to restore); the
// reconciler runs the snapshot action and converges Status.Phase
// back to RUNNING.
func TestReconcile_SnapshotRestoresRunningPhase(t *testing.T) {
	r, provider, _, vm, durableStore, snapshotClient := newReconcilerFixture(t)
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
		snapshotClient.EXPECT().
			CreateSnapshot(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, req *cpv1.CreateSnapshotRequest, _ ...grpc.CallOption) (*cpv1.CreateSnapshotResponse, error) {
				assert.Equal(t, "snap-1", req.GetSnapshot().GetMetadata().GetId())
				assert.Equal(t, "test", req.GetSnapshot().GetMetadata().GetNamespace())
				assert.Equal(t, "before-deploy", req.GetSnapshot().GetMetadata().GetDescription(),
					"description from Intent.StartSnapshot must be forwarded to the api-server")
				assert.Equal(t, "sb-snap", req.GetSnapshot().GetSandbox().GetId())
				// The Snapshot row must carry the source sandbox's
				// resources so the api-server can stamp them onto any
				// sandbox later restored from this snapshot.
				assert.Equal(t, sandbox.GetResources().GetVcpuCount(),
					req.GetSnapshot().GetResources().GetVcpuCount(),
					"Snapshot.Resources.VcpuCount must mirror the source sandbox")
				assert.Equal(t, sandbox.GetResources().GetMemoryMib(),
					req.GetSnapshot().GetResources().GetMemoryMib(),
					"Snapshot.Resources.MemoryMib must mirror the source sandbox")
				return &cpv1.CreateSnapshotResponse{Snapshot: req.GetSnapshot()}, nil
			}),
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
	r, provider, _, vm, durableStore, snapshotClient := newReconcilerFixture(t)
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
		snapshotClient.EXPECT().
			CreateSnapshot(ctx, gomock.Any()).
			Return(&cpv1.CreateSnapshotResponse{}, nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_PAUSED, sandbox.GetStatus().GetPhase(),
		"snapshot of a paused sandbox must converge back to PAUSED, not RUNNING")
	assert.Nil(t, sandbox.Intent)
}

// TestReconcile_SnapshotFailsWhenApiServerRegistrationFails verifies
// that a failure of the api-server's CreateSnapshot RPC propagates as a
// reconcile error: the local artifact is left in place, Intent is not
// cleared, and LastSnapshot is not stamped. A retry on the next
// reconcile must therefore re-issue the (idempotent) Create with the
// same body and converge.
func TestReconcile_SnapshotFailsWhenApiServerRegistrationFails(t *testing.T) {
	r, provider, _, vm, durableStore, snapshotClient := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-snap-fail")
	sandbox.Status.Phase = cpv1.SandboxStatus_PHASE_SNAPSHOTTING
	sandbox.Intent = &cpv1.Intent{
		Phase:         cpv1.SandboxStatus_PHASE_RUNNING,
		StartSnapshot: &cpv1.StartSnapshotInput{Description: "will-fail"},
	}

	localRef := stubLocalRefWithFiles(t, "snap-fail")
	registrationErr := errors.New("api-server unreachable")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-snap-fail").Return(vm),
		vm.EXPECT().CreateSnapshot(ctx).Return(localRef, nil),
		durableStore.EXPECT().Put(ctx, "snap-fail", gomock.Any()).Return(nil),
		snapshotClient.EXPECT().
			CreateSnapshot(ctx, gomock.Any()).
			Return(nil, registrationErr),
	)

	err := r.Reconcile(ctx, sandbox)
	require.Error(t, err)
	assert.ErrorIs(t, err, registrationErr,
		"reconcile must propagate the api-server registration error")

	// Intent retains StartSnapshot so the next reconcile cycle re-issues
	// the same logical CreateSnapshot — idempotent at the api-server.
	require.NotNil(t, sandbox.GetIntent())
	require.NotNil(t, sandbox.GetIntent().GetStartSnapshot())
	assert.Nil(t, sandbox.GetLastSnapshot(),
		"LastSnapshot must NOT be stamped when api-server registration fails")
}

// TestReconcile_BootIntentRunsCreateMicroVMAndClearsIntent covers a
// fresh boot from intent in a single reconcile pass. With live resize
// removed, this is the simple boot-and-converge case.
func TestReconcile_BootIntentRunsCreateMicroVMAndClearsIntent(t *testing.T) {
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-combined")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	nsh := fixtureNamespaceHandle(0)
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-combined").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(0),
		driver.EXPECT().Provision(0).Return(nsh, nil),
		provider.EXPECT().CreateMicroVM(ctx, gomock.Any()).Return(nil, nil),
	)

	require.NoError(t, r.Reconcile(ctx, sandbox))
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetStatus().GetPhase())
	assert.Nil(t, sandbox.Intent)
}

// TestReconcile_PhaseFailureReturnsError pins that a boot failure
// surfaces up to the caller (which is responsible for marking the
// sandbox PHASE_FAILED). The network is rolled back so a retry on the
// next reconcile starts from a clean slate.
func TestReconcile_PhaseFailureReturnsError(t *testing.T) {
	r, provider, driver, _, _, _ := newReconcilerFixture(t)
	ctx := context.Background()

	sandbox := fixtureSandbox("sb-phase-bad")
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	bootErr := errors.New("kernel panic during boot")
	gomock.InOrder(
		provider.EXPECT().GetMicroVM("sb-phase-bad").Return(nil),
		provider.EXPECT().NextNetNSIndex().Return(0),
		driver.EXPECT().Provision(0).Return(fixtureNamespaceHandle(0), nil),
		provider.EXPECT().CreateMicroVM(ctx, gomock.Any()).Return(nil, bootErr),
		driver.EXPECT().Deprovision(0).Return(nil),
	)

	err := r.Reconcile(ctx, sandbox)
	require.ErrorIs(t, err, bootErr)
	require.NotNil(t, sandbox.Intent, "Intent stays set so the retry resumes")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, sandbox.GetIntent().GetPhase())
}

func TestReconcile_UnsupportedPhaseReturnsError(t *testing.T) {
	r, _, _, _, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-unsupported")
	// PHASE_PAUSING is a transitional state the api-server never sets
	// as an intent — feeding it to the reconciler must surface an
	// error rather than silently accept it.
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_PAUSING}

	err := r.Reconcile(context.Background(), sandbox)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported intent phase")
}

// TestReconcile_FailedSandboxIsTerminalAndClearsIntent pins the
// no-op-with-Intent-clear behaviour for sandboxes the controller
// previously marked PHASE_FAILED. Before this branch existed, the
// reconcilePhase default case returned "unsupported status phase
// PHASE_FAILED with intent PHASE_RUNNING" — masking the original
// failure in Status.Message and looping forever because the
// controller never saw a "done" signal.
func TestReconcile_FailedSandboxIsTerminalAndClearsIntent(t *testing.T) {
	r, provider, driver, vm, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-failed")
	sandbox.Status = &cpv1.SandboxStatus{
		Phase:   cpv1.SandboxStatus_PHASE_FAILED,
		Message: "boom: original reconcile error",
	}
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	// No action methods should fire on a terminal sandbox.
	provider.EXPECT().CreateMicroVM(gomock.Any(), gomock.Any()).Times(0)
	provider.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Times(0)
	provider.EXPECT().DeleteMicroVM(gomock.Any(), gomock.Any()).Times(0)
	driver.EXPECT().Provision(gomock.Any()).Times(0)
	driver.EXPECT().Deprovision(gomock.Any()).Times(0)
	vm.EXPECT().Pause(gomock.Any()).Times(0)
	vm.EXPECT().Resume(gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(context.Background(), sandbox))

	assert.Nil(t, sandbox.Intent,
		"outer Reconcile must clear Intent so the controller stops re-enqueuing")
	assert.Equal(t, cpv1.SandboxStatus_PHASE_FAILED, sandbox.GetStatus().GetPhase(),
		"terminal Status.Phase must be preserved")
	assert.Equal(t, "boom: original reconcile error", sandbox.GetStatus().GetMessage(),
		"the original failure message must NOT be overwritten")
}

// TestReconcile_DeletedSandboxWithStaleIntentClearsIntent covers the
// defence-in-depth case: a PHASE_DELETED sandbox should never arrive
// with non-nil Intent because the delete action clears it, but if it
// ever does (api-server bug, replay of an out-of-order event) the
// reconciler must not error.
func TestReconcile_DeletedSandboxWithStaleIntentClearsIntent(t *testing.T) {
	r, provider, driver, vm, _, _ := newReconcilerFixture(t)

	sandbox := fixtureSandbox("sb-deleted")
	sandbox.Status = &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_DELETED}
	sandbox.Intent = &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING}

	provider.EXPECT().CreateMicroVM(gomock.Any(), gomock.Any()).Times(0)
	provider.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Times(0)
	provider.EXPECT().DeleteMicroVM(gomock.Any(), gomock.Any()).Times(0)
	driver.EXPECT().Provision(gomock.Any()).Times(0)
	driver.EXPECT().Deprovision(gomock.Any()).Times(0)
	vm.EXPECT().Pause(gomock.Any()).Times(0)
	vm.EXPECT().Resume(gomock.Any()).Times(0)

	require.NoError(t, r.Reconcile(context.Background(), sandbox))

	assert.Nil(t, sandbox.Intent)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_DELETED, sandbox.GetStatus().GetPhase())
}
