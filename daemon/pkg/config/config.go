// Package config holds the strongly-typed configuration bundle the
// daemon binary loads at startup.
package config

import (
	"time"

	"github.com/spf13/viper"
	"golang.nuinfra.net/commons/pkg/config"
)

type NodeRuntime string

const (
	NodeRuntimeLocal NodeRuntime = "local"
	NodeRuntimeEC2   NodeRuntime = "aws-ec2"
)

const (
	defaultFirecrackerBinaryName = "firecracker"
	defaultJailerBinaryName      = "jailer"
	defaultJailUID               = 1000
	defaultJailGID               = 1000
	defaultChrootBaseDir         = "/srv/jailer"

	defaultKernelFile       = "vmlinux"
	defaultRootfsFile       = "rootfs.ext4"
	defaultKernelJailPath   = "/vmlinux"
	defaultRootfsJailPath   = "/rootfs.ext4"
	defaultLogJailPath      = "/firecracker.log"
	defaultVsockUDSJailPath = "/v.sock"
	defaultAgentInitPath    = "/init"
	defaultAgentVsockPort   = uint32(1024)
	defaultGuestCID         = int64(3)

	// defaultGuestMAC is a locally-administered address. Deterministic
	// across runs so tcpdump output stays predictable.
	defaultGuestMAC = "06:00:AC:10:00:02"

	defaultSessionIDFile        = "/var/lib/nuinfra/daemon/session.id"
	defaultControllerMaxRetries = 10
	defaultReconnectMaxInterval = 2 * time.Minute
	defaultResyncInterval       = 5 * time.Minute

	defaultNodeRuntime               NodeRuntime = NodeRuntimeLocal
	defaultNodeMetricsReportInterval             = 30 * time.Second
	defaultNodeHealthReportInterval              = 30 * time.Second
)

// Bundle is the daemon's top-level configuration. It embeds the gRPC
// server base shared with the api-server via [config.Base] and adds
// daemon-specific sections for the Firecracker provider and per-VM
// template constants.
type Bundle struct {
	config.Base `mapstructure:",squash"`

	// AssetsDir is the host directory containing the firecracker binary,
	// the jailer binary, the guest kernel image, and the base rootfs
	// image. Filenames inside it are configured via [Firecracker] and
	// [MicroVM].
	AssetsDir string `mapstructure:"assets-dir" validate:"required"`

	// Firecracker holds the jailer / chroot configuration applied to
	// every MicroVM spawned on this node.
	Firecracker *Firecracker `mapstructure:"firecracker"`

	// MicroVM holds per-VM template constants the reconciler uses when
	// translating a Sandbox proto into a CreateMicroVMInput.
	MicroVM *MicroVM `mapstructure:"microvm"`

	// APIServer is how the daemon reaches the control-plane gRPC
	// service. The controller dials Address to open the EstablishSession
	// stream.
	APIServer *APIServer `mapstructure:"api-server" validate:"required"`

	// Controller tunes the daemon's reconciliation loop.
	Controller *Controller `mapstructure:"controller"`

	// NodeAgent tunes the daemon's node metrics and health report cadence.
	NodeAgent *NodeAgent `mapstructure:"node-agent"`
}

// APIServer points the daemon at its control plane.
type APIServer struct {
	// Address is the host:port the daemon dials. Required.
	Address string `mapstructure:"address" validate:"required,hostname_port"`
}

// Controller tunes the daemon's reconciliation loop.
type Controller struct {
	// SessionIDFile is the host path where the daemon persists the
	// session id assigned by the api-server. Read at startup and sent
	// on connect to resume the previous session; written after the
	// first acknowledgement of a new session. Defaults to
	// "/var/lib/nuinfra/daemon/session.id".
	SessionIDFile string `mapstructure:"session-id-file"`

	// MaxRetries caps how many times a single sandbox can be requeued
	// before the controller gives up, marks Status.Phase = PHASE_FAILED,
	// and pushes that update to the api-server. Defaults to 10.
	MaxRetries int `mapstructure:"max-retries"`

	// ReconnectMaxInterval is the upper bound on the exponential backoff
	// used when re-establishing the EstablishSession stream after a
	// disconnect. Defaults to 2 minutes.
	ReconnectMaxInterval time.Duration `mapstructure:"reconnect-max-interval"`

	// ResyncInterval is the cadence at which the controller calls
	// SandboxService.ListSandboxes against the api-server to recover
	// from missed events. The first tick is staggered by a small
	// jitter to avoid fleet-wide thundering herds; each subsequent
	// tick is offset by ±10% of this interval. Setting it to 0
	// disables the resync goroutine entirely (useful in tests and
	// when narrowing down api-server load). Defaults to 5 minutes.
	ResyncInterval time.Duration `mapstructure:"resync-interval"`
}

// Firecracker collects settings that apply to every MicroVM the daemon
// spawns. They are passed through to the firecracker.Provider via its
// option-builders.
type Firecracker struct {
	// BinaryName is the firecracker executable inside [Bundle.AssetsDir].
	// Defaults to "firecracker".
	BinaryName string `mapstructure:"binary-name"`

	// JailerBinaryName is the jailer executable inside [Bundle.AssetsDir].
	// Defaults to "jailer".
	JailerBinaryName string `mapstructure:"jailer-binary-name"`

	// JailUID is the unprivileged uid the jailer drops privileges to
	// before exec'ing firecracker. Defaults to 1000.
	JailUID uint32 `mapstructure:"jail-uid"`

	// JailGID matches JailUID but for the group id. Defaults to 1000.
	JailGID uint32 `mapstructure:"jail-gid"`

	// AttachConsole keeps the guest's serial console attached to the
	// daemon's stdio (debug only; disables --daemonize).
	AttachConsole bool `mapstructure:"attach-console"`

	// ChrootBaseDir is where jailer roots every VM's chroot. Defaults
	// to "/srv/jailer" (matches upstream jailer's default).
	ChrootBaseDir string `mapstructure:"chroot-base-dir"`
}

// MicroVM captures the per-VM constants the reconciler uses when
// composing a [firecracker.CreateMicroVMInput] from a Sandbox proto.
// Paths suffixed with JailPath are what firecracker sees inside its
// chroot; the matching host-side files come from
// [Bundle.AssetsDir]/<file-name>.
type MicroVM struct {
	// KernelFile is the kernel image filename inside [Bundle.AssetsDir].
	// Defaults to "vmlinux".
	KernelFile string `mapstructure:"kernel-file"`

	// RootfsFile is the base rootfs image inside [Bundle.AssetsDir].
	// Defaults to "rootfs.ext4".
	RootfsFile string `mapstructure:"rootfs-file"`

	// KernelJailPath is where KernelFile is staged inside the chroot.
	// Defaults to "/vmlinux".
	KernelJailPath string `mapstructure:"kernel-jail-path"`

	// RootfsJailPath is where RootfsFile is staged inside the chroot.
	// Defaults to "/rootfs.ext4".
	RootfsJailPath string `mapstructure:"rootfs-jail-path"`

	// LogJailPath is firecracker's --log-path within the chroot.
	// Defaults to "/firecracker.log".
	LogJailPath string `mapstructure:"log-jail-path"`

	// VsockUDSJailPath is the host-side UDS path firecracker exposes
	// inside the chroot. Defaults to "/v.sock".
	VsockUDSJailPath string `mapstructure:"vsock-uds-jail-path"`

	// AgentInitPath is the in-guest path of the agent binary, used as
	// init= on the kernel cmdline. Defaults to "/init".
	AgentInitPath string `mapstructure:"agent-init-path"`

	// AgentVsockPort is the vsock port the in-guest agent listens on.
	// Defaults to 1024.
	AgentVsockPort uint32 `mapstructure:"agent-vsock-port"`

	// GuestCID is the vsock guest CID assigned to every VM.
	// Defaults to 3 (the first non-reserved CID).
	GuestCID int64 `mapstructure:"guest-cid"`

	// GuestMAC is the guest virtio-net MAC address. Locally-administered
	// by default; deterministic for predictable tcpdump output.
	GuestMAC string `mapstructure:"guest-mac"`
}

// NodeAgent configures the node agent.
type NodeAgent struct {
	// Runtime identifies the environment in which the node agent is running.
	//
	// Supported runtimes are:
	//   - local: intended for development and integration tests
	//   - aws-ec2: running inside an AWS EC2 instance
	Runtime NodeRuntime `mapstructure:"node-runtime" validate:"oneof=local aws-ec2"`

	// MetricsReportInterval is the cadence at which the node agent collects
	// node metrics and reports them to the API server.
	MetricsReportInterval time.Duration `mapstructure:"metrics-report-interval"`

	// HealthReportInterval is the cadence at which the node agent checks node
	// health and reports it to the API server.
	HealthReportInterval time.Duration `mapstructure:"health-report-interval"`
}

// NewFromFile loads configuration from filename into a [Bundle], applying
// daemon-level defaults before viper's unmarshal step.
func NewFromFile(filename string) (*Bundle, error) {
	v := viper.New()
	applyDefaults(v)
	return config.NewFromFile[*Bundle](filename, v)
}

// applyDefaults wires the daemon's defaults into v. Exported indirectly
// via NewFromFile; exposed inside the package so tests can construct a
// pre-populated viper instance.
func applyDefaults(v *viper.Viper) {
	v.SetDefault("firecracker.binary-name", defaultFirecrackerBinaryName)
	v.SetDefault("firecracker.jailer-binary-name", defaultJailerBinaryName)
	v.SetDefault("firecracker.jail-uid", defaultJailUID)
	v.SetDefault("firecracker.jail-gid", defaultJailGID)
	v.SetDefault("firecracker.chroot-base-dir", defaultChrootBaseDir)

	v.SetDefault("microvm.kernel-file", defaultKernelFile)
	v.SetDefault("microvm.rootfs-file", defaultRootfsFile)
	v.SetDefault("microvm.kernel-jail-path", defaultKernelJailPath)
	v.SetDefault("microvm.rootfs-jail-path", defaultRootfsJailPath)
	v.SetDefault("microvm.log-jail-path", defaultLogJailPath)
	v.SetDefault("microvm.vsock-uds-jail-path", defaultVsockUDSJailPath)
	v.SetDefault("microvm.agent-init-path", defaultAgentInitPath)
	v.SetDefault("microvm.agent-vsock-port", defaultAgentVsockPort)
	v.SetDefault("microvm.guest-cid", defaultGuestCID)
	v.SetDefault("microvm.guest-mac", defaultGuestMAC)

	v.SetDefault("controller.session-id-file", defaultSessionIDFile)
	v.SetDefault("controller.max-retries", defaultControllerMaxRetries)
	v.SetDefault("controller.reconnect-max-interval", defaultReconnectMaxInterval)
	v.SetDefault("controller.resync-interval", defaultResyncInterval)

	v.SetDefault("node-agent.runtime", defaultNodeRuntime)
	v.SetDefault("node-agent.metrics-report-interval", defaultNodeMetricsReportInterval)
	v.SetDefault("node-agent.health-report-interval", defaultNodeHealthReportInterval)
}
