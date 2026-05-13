package firecracker

import (
	"context"
	"errors"

	"golang.nuinfra.net/agent/transport"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	"golang.nuinfra.net/firecracker-client/models"
)

// ErrVMNotFound is returned by [Provider.DeleteMicroVM] when no MicroVM
// is registered under the supplied id. Callers can use [errors.Is] to
// distinguish this from a teardown failure.
var ErrVMNotFound = errors.New("microvm not found")

type Provider interface {
	// CreateMicroVM provisions a new MicroVM and inserts it into the
	// in-memory index.
	CreateMicroVM(ctx context.Context, input *CreateMicroVMInput) (MicroVM, error)

	// LoadSnapshot loads the snapshot referenced by input.LocalReference,
	// provisioning a new MicroVM under input.ID and inserting it into the
	// in-memory index. The VM is automatically resumed before the call
	// returns (resume_vm = true on PUT /snapshot/load). A previously-loaded
	// VM at this ID is detected via the same attach idempotency CreateMicroVM
	// uses; the existing instance is returned.
	LoadSnapshot(ctx context.Context, input *LoadSnapshotInput) (MicroVM, error)

	// GetMicroVM looks up an indexed MicroVM by id. Returns nil when no
	// VM with that id exists. Callers should typically invoke
	// [Provider.Recover] at startup to populate the index from the
	// chroot before relying on Get.
	GetMicroVM(id string) MicroVM

	// DeleteMicroVM tears down the MicroVM identified by id and removes
	// it from the index. Returns [ErrVMNotFound] if the id is not
	// indexed.
	DeleteMicroVM(ctx context.Context, id string) error

	// Recover scans the configured chroot base directory and re-attaches
	// to MicroVMs that survived a previous process invocation, populating
	// the index. Per-VM errors are logged but do not abort the scan.
	Recover(ctx context.Context) error

	// Len returns the number of MicroVMs currently held in the index.
	// Useful for callers (e.g. the reconciler) that need to allocate a
	// per-host resource keyed off the current population, such as a
	// network namespace index.
	Len() int
}

// LoadSnapshotInput parameterises [Provider.LoadSnapshot]. The first
// four fields carry the same per-VM scaffolding as [CreateMicroVMInput];
// the last two are specific to snapshot restore.
type LoadSnapshotInput struct {
	// ID uniquely identifies the VM on this host. Same constraints as
	// [CreateMicroVMInput.ID] — used as the jailer --id and the chroot
	// path segment.
	ID string

	// Network is the per-VM network topology provisioned beforehand by a
	// [network.Driver]. Same role as [CreateMicroVMInput.Network]: the
	// jailer is told to join Network.NamespaceName, and the snapshot's
	// embedded virtio-net interface is expected to find
	// Network.TapDeviceName in that namespace.
	Network network.NamespaceHandle

	// Assets stages host files into the chroot before firecracker starts.
	// The snapshot's state file references drive paths chroot-relative,
	// so any file referenced (rootfs, additional drives) must be present
	// in the new chroot when load runs — list it here. Same semantics as
	// [CreateMicroVMInput.Assets].
	Assets []Asset

	// AgentVsockPort is the vsock port the in-VM agent listens on.
	// Recorded on the resulting MicroVM so callers can VSock() into the
	// guest after restore. Ignored when VsockUDSPath is empty.
	AgentVsockPort uint32

	// LocalReference points at the snapshot's mem and state files on the
	// host. Paths can live anywhere readable by the daemon —
	// LoadSnapshot hard-links them (with copy fallback on EXDEV) into
	// <jailRoot>/snapshots/{SnapshotID}/{mem,state} before issuing the
	// load RPC. When the source paths already resolve to the
	// destination (e.g. the caller used snapshot.Store.Load with
	// chroot=newJailRoot), staging is a no-op.
	LocalReference *snapshot.LocalReference

	// VsockUDSPath is the chroot-relative UDS path the snapshot was
	// taken with (e.g. "/v.sock"). LoadSnapshot pins this path on
	// restore via models.VsockOverride so the resulting VM uses it
	// regardless of what the snapshot's state file embedded — keeping
	// the host-side UDS we wire into firecrackerVM.vsockUDS
	// authoritative. Required when AgentVsockPort != 0; ignored
	// otherwise.
	VsockUDSPath string
}

type CreateMicroVMInput struct {
	// ID uniquely identifies the VM on this host. It is used as the
	// jailer --id, appears in the chroot path, and feeds back into
	// firecracker via --id.
	ID string

	// Network is the per-VM network topology provisioned beforehand by a
	// [network.Driver]. The jailer is told to join Network.NamespaceName
	// and the guest's virtio-net interface is expected to reference
	// Network.TapDeviceName.
	Network network.NamespaceHandle

	// Config is the JSON-encoded Firecracker VM configuration passed to
	// the binary via --config-file after the chroot is in place. Paths
	// referenced inside (kernel_image_path, drive paths, etc.) are
	// resolved from the chroot root — i.e. they match [Asset.JailPath]
	// values staged via [Assets].
	Config *Config

	// Assets stages host files into the chroot before firecracker starts.
	// Jailer chroots into an empty tree and does not copy resources
	// itself, so any kernel image, rootfs, or other file referenced by
	// Config must be listed here. Each asset is hard-linked when the
	// source and chroot live on the same filesystem and copied
	// otherwise.
	Assets []Asset

	// AgentVsockPort is the vsock port the in-VM agent listens on.
	// [MicroVM.VSock] CONNECTs to this port through
	// Config.Vsock.UDSPath. Ignored when Config.Vsock is nil.
	AgentVsockPort uint32
}

// Asset stages a single host file into a VM's chroot.
type Asset struct {
	// SourcePath is the absolute host path of the file to stage.
	SourcePath string

	// JailPath is where the file should appear inside the chroot,
	// relative to the chroot root (leading "/" allowed and stripped).
	// Firecracker's config should reference this same path.
	JailPath string

	// Writable stages the file with permissions that let the jailed
	// firecracker process (running as [Options.JailUID]) open it
	// read-write. Needed for any drive whose IsReadOnly flag is
	// false. Leave zero for immutable assets like the kernel or a
	// read-only rootfs.
	//
	// Writable assets are copied rather than hard-linked, so each VM
	// gets its own independent file and the source at SourcePath is
	// never mutated by the guest. Non-writable assets continue to be
	// hard-linked when possible (zero-copy).
	Writable bool
}

type Config struct {
	BootSource        *models.BootSource           `json:"boot-source,omitempty"`
	Drives            []*models.Drive              `json:"drives,omitempty"`
	MachineConfig     *models.MachineConfiguration `json:"machine-config,omitempty"`
	CPUConfig         *models.CPUConfig            `json:"cpu-config,omitempty"`
	Balloon           *models.Balloon              `json:"balloon,omitempty"`
	NetworkInterfaces []*models.NetworkInterface   `json:"network-interfaces,omitempty"`
	Vsock             *models.Vsock                `json:"vsock,omitempty"`
	Logger            *models.Logger               `json:"logger,omitempty"`
	Metrics           *models.Metrics              `json:"metrics,omitempty"`
	MmdsConfig        *models.MmdsConfig           `json:"mmds-config,omitempty"`
	Entropy           *models.EntropyDevice        `json:"entropy,omitempty"`
	Pmem              []*models.Pmem               `json:"pmem,omitempty"`
	MemoryHotplug     *models.MemoryHotplugConfig  `json:"memory-hotplug,omitempty"`
}

type MicroVM interface {
	Describe(ctx context.Context) (*MicroVMInfo, error)

	UpdateResources(ctx context.Context, input UpdateResourcesInput) error

	CreateSnapshot(ctx context.Context) (*snapshot.LocalReference, error)

	Pause(ctx context.Context) error

	Resume(ctx context.Context) error

	// NetworkIndex returns the network namespace index that was passed
	// to [network.Driver.Provision] when the VM was first created. It is
	// persisted alongside the chroot so callers can deprovision the
	// matching namespace on delete even after a daemon restart.
	NetworkIndex() int

	// VSock returns a [transport.Transport] to the in-VM agent. The
	// underlying UDS is dialed lazily on first call and the same
	// Transport is returned on every subsequent call; it is closed as
	// part of [Provider.DeleteMicroVM]. An error is returned when the
	// VM has no vsock device configured or when the initial CONNECT
	// handshake fails.
	VSock() (transport.Transport, error)
}

type UpdateResourcesInput struct {
	VCPUCount int64

	MemoryMiB int64
}

type MicroVMInfo struct {
	State string

	VCPUCount int64

	MemoryMiB int64
}
