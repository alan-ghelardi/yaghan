package firecracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.nuinfra.net/agent/transport"
	localclient "golang.nuinfra.net/daemon/pkg/firecracker/client"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	fcclient "golang.nuinfra.net/firecracker-client/client"
	"golang.nuinfra.net/firecracker-client/client/operations"
	"golang.nuinfra.net/firecracker-client/models"
)

const (
	// defaultChrootBaseDir matches the upstream jailer default. Every VM
	// gets a chroot at <base>/<exec-name>/<id>/root.
	defaultChrootBaseDir = "/srv/jailer"

	// Per-VM files inside the chroot. Paths with a leading "/" in this
	// file are what Firecracker sees after the chroot; the same files are
	// reached from the host by joining under jailRoot.
	vmConfigFile   = "vm_config.json"
	apiSocketFile  = "firecracker.sock"
	jailRootSubDir = "root" // jailer creates <vmDir>/root/ as the chroot

	// vmMetaFile is a daemon-owned sidecar that persists per-VM
	// metadata not captured by Firecracker itself (notably the
	// pseudo-init pid and the agent's vsock port). It lives in vmDir,
	// outside the chroot, so the guest can never read it. Wiped together
	// with the rest of vmDir on Delete.
	vmMetaFile = "meta.json"

	readyPollInterval = 10 * time.Millisecond
	readyTimeout      = 5 * time.Second
	attachPingTimeout = 200 * time.Millisecond
	pidPollInterval   = 10 * time.Millisecond
	gracefulShutdown  = 500 * time.Millisecond
	forceShutdownWait = 100 * time.Millisecond
)

// vmIDPattern is the constraint both Firecracker and the jailer enforce on
// instance identifiers. We validate up front to keep caller-supplied IDs
// from traversing out of the chroot base dir when composed into paths.
var vmIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// New returns a Firecracker-backed [Provider] configured by opts. It
// requires [WithFirecrackerPath] and [WithJailerPath]; other options have
// reasonable defaults.
//
// New is intentionally IO-free: it does not touch the chroot. Callers
// that need to recover existing MicroVMs from a previous process should
// invoke [Provider.Recover] after construction.
func New(opts ...Option) (Provider, error) {
	options := &Options{
		ChrootBaseDir: defaultChrootBaseDir,
	}
	for _, opt := range opts {
		opt.Apply(options)
	}
	if options.FirecrackerPath == "" {
		return nil, errors.New("firecracker path is required (use WithFirecrackerPath)")
	}
	if options.JailerPath == "" {
		return nil, errors.New("jailer path is required (use WithJailerPath)")
	}
	return &firecracker{
		options: options,
		vms:     make(map[string]*firecrackerVM),
	}, nil
}

type firecracker struct {
	options *Options

	// mu guards vms. Held only for the index mutation itself — never
	// during slow operations (jailer spawn, signal-and-wait).
	mu  sync.RWMutex
	vms map[string]*firecrackerVM
}

var _ Provider = (*firecracker)(nil)

// vmMeta is the on-disk shape of <vmDir>/meta.json. Forward-compatible:
// new fields can be appended; absent fields fall back to zero values
// when reading legacy chroots.
type vmMeta struct {
	PID            int    `json:"pid"`
	AgentVsockPort uint32 `json:"agent_vsock_port"`

	// NetworkIndex is the per-host index supplied to
	// network.Driver.Provision when the VM was first created. Persisted
	// here so a daemon restart can deprovision the right namespace on
	// delete without an in-memory ledger.
	NetworkIndex int `json:"network_index"`
}

// execName is the basename of the firecracker binary. Jailer uses it in
// both the chroot path (<base>/<exec-name>/<id>/root) and the pid file
// name (<exec-name>.pid), so we derive it once here.
func (f *firecracker) execName() string {
	return filepath.Base(f.options.FirecrackerPath)
}

// vmDir returns the jailer per-VM directory — the one that contains the
// chroot root alongside jailer-written state (the pid file).
func (f *firecracker) vmDir(id string) string {
	return filepath.Join(f.options.ChrootBaseDir, f.execName(), id)
}

// jailRoot is the chroot firecracker runs in. Every path firecracker
// writes (socket, config, snapshots) lives under here on the host.
func (f *firecracker) jailRoot(id string) string {
	return filepath.Join(f.vmDir(id), jailRootSubDir)
}

// pidFilePath is the host path of the file jailer writes when
// --new-pid-ns is set. It contains the host-visible PID of the
// firecracker pseudo-init; we signal that PID for pause/resume/destroy.
//
// As of jailer v1.15, the pid file is written inside the chroot root
// (jailRoot) alongside the socket, not in the vmDir parent.
func (f *firecracker) pidFilePath(id string) string {
	return filepath.Join(f.jailRoot(id), f.execName()+".pid")
}

// metaFilePath is the host path of the daemon-owned sidecar metadata
// file for a VM. Lives in vmDir (outside the chroot) so the guest
// cannot read it.
func (f *firecracker) metaFilePath(id string) string {
	return filepath.Join(f.vmDir(id), vmMetaFile)
}

// writeMeta persists meta to <vmDir>/meta.json with daemon-only
// permissions (0o600). Caller is responsible for ensuring vmDir exists.
func (f *firecracker) writeMeta(id string, meta vmMeta) error {
	path := f.metaFilePath(id)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	return json.NewEncoder(out).Encode(meta)
}

// readMeta loads <vmDir>/meta.json. Returns (zero, os.ErrNotExist)
// wrapped via the file system when the sidecar is absent — callers can
// check with errors.Is for legacy fallback handling.
func (f *firecracker) readMeta(id string) (vmMeta, error) {
	var meta vmMeta
	data, err := os.ReadFile(f.metaFilePath(id))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode meta.json: %w", err)
	}
	return meta, nil
}

// CreateMicroVM implements [Provider].
func (f *firecracker) CreateMicroVM(ctx context.Context, input *CreateMicroVMInput) (MicroVM, error) {
	if !vmIDPattern.MatchString(input.ID) {
		return nil, fmt.Errorf("invalid vm id %q: must match %s", input.ID, vmIDPattern)
	}
	vmDir := f.vmDir(input.ID)
	jailRoot := f.jailRoot(input.ID)
	socketHostPath := filepath.Join(jailRoot, apiSocketFile)

	if vm, err := f.attach(ctx, input.ID); err == nil {
		// Reusing a live VM: make sure it appears in the index. Callers
		// of Create may not have invoked Recover yet, or this VM may
		// have been spawned by an even earlier daemon process.
		f.indexInsert(vm)
		return vm, nil
	}

	if err := os.RemoveAll(vmDir); err != nil {
		return nil, fmt.Errorf("clean stale vm dir: %w", err)
	}
	// Jailer creates <vmDir>/root itself, but pre-creating it lets us
	// drop the config file in before the process starts. 0755 so the
	// unprivileged jail uid can traverse it post-chroot.
	if err := os.MkdirAll(jailRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create jail root: %w", err)
	}

	configHostPath := filepath.Join(jailRoot, vmConfigFile)
	if err := writeJSON(configHostPath, input.Config); err != nil {
		return nil, fmt.Errorf("write vm config: %w", err)
	}
	// World-readable so the jail uid can read the config post-chroot.
	if err := os.Chmod(configHostPath, 0o644); err != nil {
		return nil, fmt.Errorf("chmod vm config: %w", err)
	}

	for _, asset := range input.Assets {
		if err := stageAsset(jailRoot, asset); err != nil {
			return nil, fmt.Errorf("stage asset %q: %w", asset.SourcePath, err)
		}
	}

	cmd := f.buildJailerCommand(ctx, input)
	// Inherit our stdio so jailer's setup messages (including the ones
	// emitted before --daemonize redirects firecracker's I/O to
	// /dev/null) surface in the terminal instead of silently going to
	// the null device, which is what exec.Cmd does by default when
	// Stdout/Stderr are nil.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn jailer: %w", err)
	}
	if f.options.AttachConsole {
		// Without --daemonize, jailer stays alive as firecracker's
		// parent and the kernel's console streams through our stdio.
		// Reap it in the background so Destroy's SIGTERM to firecracker
		// still causes a clean teardown without leaking a zombie
		// jailer.
		go func() { _ = cmd.Wait() }()
	} else {
		// With --daemonize + --new-pid-ns the bootstrap jailer process
		// exits after forking firecracker into a fresh PID namespace.
		// Wait() reaps it so we don't leave a zombie; the surviving
		// firecracker is a grandchild reparented to init.
		if err := cmd.Wait(); err != nil {
			_ = os.RemoveAll(vmDir)
			return nil, fmt.Errorf("jailer bootstrap failed: %w", err)
		}
	}

	apiClient := localclient.New(socketHostPath)
	if err := waitUntilRunning(ctx, apiClient); err != nil {
		// Firecracker was forked before the readiness check started;
		// kill it by the pid jailer recorded so it doesn't linger.
		if pid, readErr := readPIDFile(f.pidFilePath(input.ID)); readErr == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		// Intentionally do NOT remove vmDir here: leaving the chroot
		// intact lets callers inspect the firecracker log, config, and
		// staged assets to diagnose the failure. The next CreateMicroVM
		// call for the same id wipes stale state before retrying.
		return nil, fmt.Errorf("vm did not become ready (artifacts kept at %s): %w", vmDir, err)
	}

	pid, err := readPIDFile(f.pidFilePath(input.ID))
	if err != nil {
		return nil, fmt.Errorf("read firecracker pid file: %w", err)
	}

	if err := f.writeMeta(input.ID, vmMeta{
		PID:            pid,
		AgentVsockPort: input.AgentVsockPort,
		NetworkIndex:   input.Network.Index,
	}); err != nil {
		return nil, fmt.Errorf("write meta.json: %w", err)
	}

	udsHostPath, agentPort := vsockEndpoint(jailRoot, input)
	vm := &firecrackerVM{
		client:       apiClient,
		vmDir:        vmDir,
		jailRoot:     jailRoot,
		id:           input.ID,
		pid:          pid,
		networkIndex: input.Network.Index,
		vsockUDS:     udsHostPath,
		vsockPort:    agentPort,
		jailUID:      f.options.JailUID,
		jailGID:      f.options.JailGID,
	}
	f.indexInsert(vm)
	return vm, nil
}

// indexInsert publishes vm to the in-memory index. Safe to call
// multiple times for the same id — the latest pointer wins, matching
// the WatchableStream semantics elsewhere.
func (f *firecracker) indexInsert(vm *firecrackerVM) {
	f.mu.Lock()
	f.vms[vm.id] = vm
	f.mu.Unlock()
}

// vsockEndpoint resolves the host-visible UDS path and the agent's
// listening port for a VM. Returns ("", 0) when the input has no vsock
// device configured — the resulting firecrackerVM will then refuse
// VSock calls with a descriptive error.
func vsockEndpoint(jailRoot string, input *CreateMicroVMInput) (string, uint32) {
	if input == nil || input.Config == nil || input.Config.Vsock == nil || input.Config.Vsock.UdsPath == nil {
		return "", 0
	}
	rel := strings.TrimPrefix(*input.Config.Vsock.UdsPath, "/")
	return filepath.Join(jailRoot, rel), input.AgentVsockPort
}

// buildJailerCommand assembles the `jailer ... -- firecracker-args`
// invocation. Per-VM flags come from the input; process-level flags come
// from the Provider's Options. Everything after `--` is handed directly
// to firecracker.
//
// Note: jailer auto-appends --id to firecracker, so we don't pass it.
// --api-sock is rooted at the chroot's /, which maps to <jailRoot> on the
// host.
func (f *firecracker) buildJailerCommand(ctx context.Context, input *CreateMicroVMInput) *exec.Cmd {
	args := []string{
		"--id", input.ID,
		"--exec-file", f.options.FirecrackerPath,
		"--uid", strconv.FormatUint(uint64(f.options.JailUID), 10),
		"--gid", strconv.FormatUint(uint64(f.options.JailGID), 10),
		"--chroot-base-dir", f.options.ChrootBaseDir,
		"--netns", netnsPath(input.Network.NamespaceName),
		// --new-pid-ns isolates the guest in its own PID namespace.
		// Jailer forks firecracker as pseudo-init of that namespace and
		// writes the host-visible PID to <vmDir>/<exec-name>.pid, which
		// we read back for lifecycle management.
		"--new-pid-ns",
	}
	if !f.options.AttachConsole {
		// --daemonize redirects I/O to /dev/null and setsid() before
		// exec, decoupling firecracker's lifetime from our process.
		// Omitted in debug (AttachConsole) mode so guest kernel
		// output reaches the terminal.
		args = append(args, "--daemonize")
	}
	args = append(args,
		"--",
		// Flags after `--` are handed to firecracker itself.
		"--api-sock", "/"+apiSocketFile,
		"--config-file", "/"+vmConfigFile,
	)
	// #nosec G204 -- input.ID is validated by vmIDPattern; exec.Command
	// does not invoke a shell. Using CommandContext means ctx
	// cancellation terminates the short-lived bootstrap jailer; it has
	// no effect on the already-forked firecracker grandchild.
	return exec.CommandContext(ctx, f.options.JailerPath, args...)
}

// netnsPath returns the bind-mount path a named namespace lives at,
// matching the convention used by the netns library and by `ip netns`.
func netnsPath(name string) string {
	return "/var/run/netns/" + name
}

// attach returns a handle to a pre-existing VM by reading its sidecar
// metadata, validating the API socket via DescribeInstance, and
// reconstructing the host-side vsock endpoint from the persisted
// config. Returns ErrVMNotFound when the VM is gone or unreachable;
// any other error indicates a corrupt or partially-removed chroot
// that warrants operator attention.
func (f *firecracker) attach(ctx context.Context, id string) (*firecrackerVM, error) {
	jailRoot := f.jailRoot(id)
	socketPath := filepath.Join(jailRoot, apiSocketFile)
	if _, err := os.Stat(socketPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrVMNotFound
		}
		return nil, fmt.Errorf("stat api socket: %w", err)
	}

	meta, err := f.readMeta(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrVMNotFound
		}
		return nil, fmt.Errorf("read meta.json: %w", err)
	}

	udsHostPath, err := readVsockUDSPath(filepath.Join(jailRoot, vmConfigFile), jailRoot)
	if err != nil {
		// A missing or unreadable vm_config.json doesn't mean the VM
		// is dead, but VSock dial would fail anyway; treat as no
		// vsock configured and continue.
		udsHostPath = ""
	}

	apiClient := localclient.New(socketPath)
	pingCtx, cancel := context.WithTimeout(ctx, attachPingTimeout)
	defer cancel()
	params := operations.NewDescribeInstanceParamsWithContext(pingCtx)
	if _, err := apiClient.Operations.DescribeInstance(params); err != nil {
		return nil, ErrVMNotFound
	}

	return &firecrackerVM{
		client:       apiClient,
		vmDir:        f.vmDir(id),
		jailRoot:     jailRoot,
		id:           id,
		pid:          meta.PID,
		networkIndex: meta.NetworkIndex,
		vsockUDS:     udsHostPath,
		vsockPort:    meta.AgentVsockPort,
		jailUID:      f.options.JailUID,
		jailGID:      f.options.JailGID,
	}, nil
}

// readVsockUDSPath parses vm_config.json and returns the host-side
// path of the vsock UDS, or "" if the VM has no vsock device.
func readVsockUDSPath(configPath, jailRoot string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	if cfg.Vsock == nil || cfg.Vsock.UdsPath == nil {
		return "", nil
	}
	rel := strings.TrimPrefix(*cfg.Vsock.UdsPath, "/")
	return filepath.Join(jailRoot, rel), nil
}

// waitUntilRunning polls DescribeInstance until the VM reaches the Running
// state, the parent context is cancelled, or readyTimeout elapses.
func waitUntilRunning(ctx context.Context, c *fcclient.FirecrackerAPI) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	ticker := time.NewTicker(readyPollInterval)
	defer ticker.Stop()

	for {
		params := operations.NewDescribeInstanceParamsWithContext(ctx)
		resp, err := c.Operations.DescribeInstance(params)
		if err == nil && resp.Payload != nil && resp.Payload.State != nil &&
			*resp.Payload.State == models.InstanceInfoStateRunning {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return errors.Join(ctx.Err(), err)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func writeJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

// readPIDFile parses a pid file written by jailer (one decimal number,
// optionally followed by whitespace).
func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// stageAsset places a single host file inside the chroot before firecracker
// starts. Immutable assets (Writable == false) are hard-linked when
// possible and fall back to a full copy on EXDEV. Writable assets are
// always copied so each VM gets its own independent file — the source at
// SourcePath is never mutated by the guest. Files are made world-readable
// so the unprivileged jail uid can open them; writable ones also get
// group+other write.
func stageAsset(jailRoot string, a Asset) error {
	rel := strings.TrimPrefix(a.JailPath, "/")
	dst := filepath.Join(jailRoot, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dst) // Link and copy both refuse when dst exists.

	if a.Writable {
		if err := copyFile(a.SourcePath, dst); err != nil {
			return err
		}
	} else if err := os.Link(a.SourcePath, dst); err != nil {
		var linkErr *os.LinkError
		if !errors.As(err, &linkErr) || !errors.Is(linkErr.Err, syscall.EXDEV) {
			return err
		}
		if err := copyFile(a.SourcePath, dst); err != nil {
			return err
		}
	}

	mode := os.FileMode(0o644)
	if a.Writable {
		mode = 0o666
	}
	return os.Chmod(dst, mode)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

type firecrackerVM struct {
	client       *fcclient.FirecrackerAPI
	vmDir        string // host-side per-VM dir (contains pid file, chroot root)
	jailRoot     string // chroot root as seen from the host
	id           string // matches the index key on the owning Provider
	pid          int
	networkIndex int // network.Driver index allocated to this VM

	// jailUID / jailGID are the unprivileged uid/gid the jailer drops
	// privileges to before exec'ing firecracker. We copy them off the
	// parent Provider's Options so chroot-side artifacts the daemon
	// creates at runtime (e.g. the snapshot dir) can be chowned to
	// match — otherwise firecracker, running as jailUID, can't write
	// into a root-owned dir the daemon mkdir'd.
	jailUID uint32
	jailGID uint32

	// vsockUDS and vsockPort are the host-visible UDS path and the
	// guest agent's listen port; zero-valued when the VM was created
	// without a vsock device, in which case VSock returns an error.
	vsockUDS  string
	vsockPort uint32

	// vsockMu guards the lazy dial performed on the first successful
	// VSock call. Failures are NOT cached — the caller may legitimately
	// invoke VSock before the guest agent has finished booting and
	// want to retry; we only memoise once the handshake has worked.
	vsockMu sync.Mutex
	vsock   transport.Transport
}

var _ MicroVM = (*firecrackerVM)(nil)

// Describe  implements [MicroVM].
func (f *firecrackerVM) Describe(ctx context.Context) (*MicroVMInfo, error) {
	state, err := f.instanceState(ctx)
	if err != nil {
		return nil, err
	}

	params := operations.NewGetMachineConfigurationParamsWithContext(ctx)
	resp, err := f.client.Operations.GetMachineConfiguration(params)
	if err != nil {
		return nil, err
	}

	payload := resp.GetPayload()
	if payload == nil {
		return nil, errors.New("Describe: payload is nil")
	}

	info := &MicroVMInfo{State: state}
	if vcpuCount := payload.VcpuCount; vcpuCount != nil {
		info.VCPUCount = *vcpuCount
	}
	if memSize := payload.MemSizeMib; memSize != nil {
		info.MemoryMiB = *memSize
	}

	return info, nil
}

// UpdateResources implements [MicroVM].
func (f *firecrackerVM) UpdateResources(ctx context.Context, input UpdateResourcesInput) error {
	params := operations.NewPatchMachineConfigurationParamsWithContext(ctx).
		WithBody(&models.MachineConfiguration{
			VcpuCount:  &input.VCPUCount,
			MemSizeMib: &input.MemoryMiB,
		})

	_, err := f.client.Operations.PatchMachineConfiguration(params)
	return err
}

// Pause implements [MicroVM].
func (f *firecrackerVM) Pause(ctx context.Context) error {
	return f.transition(ctx, models.VMStatePaused, models.InstanceInfoStatePaused)
}

// Resume implements [MicroVM].
func (f *firecrackerVM) Resume(ctx context.Context) error {
	return f.transition(ctx, models.VMStateResumed, models.InstanceInfoStateRunning)
}

// transition is a no-op when the VM already reports observedTarget, otherwise
// it asks the API to move into requestedTarget.
func (f *firecrackerVM) transition(ctx context.Context, requestedTarget, observedTarget string) error {
	current, err := f.instanceState(ctx)
	if err != nil {
		return err
	}
	if current == observedTarget {
		return nil
	}
	params := operations.NewPatchVMParamsWithContext(ctx).
		WithBody(&models.VM{State: &requestedTarget})
	_, err = f.client.Operations.PatchVM(params)
	return err
}

func (f *firecrackerVM) instanceState(ctx context.Context) (string, error) {
	params := operations.NewDescribeInstanceParamsWithContext(ctx)
	resp, err := f.client.Operations.DescribeInstance(params)
	if err != nil {
		return "", err
	}
	if resp.Payload == nil || resp.Payload.State == nil {
		return "", fmt.Errorf("empty instance state")
	}
	return *resp.Payload.State, nil
}

// CreateSnapshot implements [MicroVM]. Snapshot files live inside the jail
// (firecracker can only write to paths within its chroot); we return
// host-absolute paths for the caller's convenience.
//
// Firecracker requires the VM to be paused before save/restore. Rather
// than push that quirk onto the api-server contract, we encapsulate it
// here: if the VM is running we Pause it, take the snapshot, and
// Resume — but only if we were the ones that paused it. If a client
// paused the VM explicitly, we leave it paused on the way out.
func (f *firecrackerVM) CreateSnapshot(ctx context.Context) (*snapshot.LocalReference, error) {
	localRef := snapshot.MakeLocalReference(f.jailRoot)
	snapshotDir := filepath.Dir(localRef.MemFilePath)
	if err := os.MkdirAll(snapshotDir, snapshot.DirMode); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}
	// The daemon runs as root, so MkdirAll leaves the dir 0o755
	// root-owned. Firecracker (jailUID) needs to create mem/state files
	// inside it — chown so it can.
	if err := os.Chown(snapshotDir, int(f.jailUID), int(f.jailGID)); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return nil, fmt.Errorf("chown snapshot dir: %w", err)
	}

	state, err := f.instanceState(ctx)
	if err != nil {
		_ = os.RemoveAll(snapshotDir)
		return nil, fmt.Errorf("inspect instance state: %w", err)
	}
	pausedByUs := false
	if state == models.InstanceInfoStateRunning {
		if err := f.Pause(ctx); err != nil {
			_ = os.RemoveAll(snapshotDir)
			return nil, fmt.Errorf("pause for snapshot: %w", err)
		}
		pausedByUs = true
	}
	defer func() {
		if pausedByUs {
			// Best-effort: if resume fails, the next reconcile tick
			// will retry. Suppressing the error here keeps the
			// caller-facing failure (if any) about the snapshot
			// rather than the resume.
			_ = f.Resume(ctx)
		}
	}()

	memFilePath := snapshot.MemFilePath(localRef.SnapshotID)
	stateFilePath := snapshot.StateFilePath(localRef.SnapshotID)
	params := operations.NewCreateSnapshotParamsWithContext(ctx).
		WithBody(&models.SnapshotCreateParams{
			MemFilePath:  &memFilePath,
			SnapshotPath: &stateFilePath,
		})
	if _, err := f.client.Operations.CreateSnapshot(params); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return nil, fmt.Errorf("firecracker snapshot RPC: %w", err)
	}

	return localRef, nil
}

// destroy tears down the underlying firecracker process and wipes the
// chroot. It is invoked exclusively by [firecracker.DeleteMicroVM]; the
// Provider is the single source of truth for the in-memory index, so
// destroy itself does not touch it. Safe to call more than once.
//
// The pid we hold is firecracker's host-visible PID (originally written
// by jailer to <jailRoot>/<exec-name>.pid and mirrored into meta.json).
// Because firecracker runs as pseudo-init of its own PID namespace,
// killing it tears down every descendant in that namespace.
func (f *firecrackerVM) destroy(ctx context.Context) error {
	// Tear down the vsock channel first, if any — the UDS sits inside
	// the chroot we are about to remove, and shutting it down cleanly
	// is kinder than letting the conn die with the VM.
	if f.vsock != nil {
		_ = f.vsock.Close()
	}
	if f.pid > 0 {
		if err := syscall.Kill(f.pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("signal firecracker: %w", err)
		}
		if !waitProcessExit(ctx, f.pid, gracefulShutdown) {
			_ = syscall.Kill(f.pid, syscall.SIGKILL)
			_ = waitProcessExit(ctx, f.pid, forceShutdownWait)
		}
	}
	if err := os.RemoveAll(f.vmDir); err != nil {
		return fmt.Errorf("remove vm dir: %w", err)
	}
	return nil
}

// NetworkIndex implements [MicroVM]. The field is set at construction
// (CreateMicroVM or attach) and never mutated, so no synchronisation
// is required.
func (f *firecrackerVM) NetworkIndex() int {
	return f.networkIndex
}

// VSock implements [MicroVM]. The UDS is dialed lazily on the first
// successful call and the resulting Transport cached thereafter. A
// failed dial (the guest agent may not yet be listening) is NOT
// memoised — callers can retry until the agent is up without
// reinstantiating the VM handle.
func (f *firecrackerVM) VSock() (transport.Transport, error) {
	f.vsockMu.Lock()
	defer f.vsockMu.Unlock()
	if f.vsock != nil {
		return f.vsock, nil
	}
	if f.vsockUDS == "" || f.vsockPort == 0 {
		return nil, errors.New("vsock not configured for this vm")
	}
	tr, err := transport.New(f.vsockUDS, f.vsockPort)
	if err != nil {
		return nil, err
	}
	f.vsock = tr
	return f.vsock, nil
}

// waitProcessExit returns true when pid is no longer reachable via signal 0
// before timeout or ctx cancellation.
func waitProcessExit(ctx context.Context, pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(pidPollInterval)
	defer ticker.Stop()
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// Len implements [Provider]. Reads the index size under the same
// read-lock used by GetMicroVM.
func (f *firecracker) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.vms)
}

// GetMicroVM implements [Provider].
func (f *firecracker) GetMicroVM(id string) MicroVM {
	f.mu.RLock()
	defer f.mu.RUnlock()
	vm, ok := f.vms[id]
	if !ok {
		return nil
	}
	return vm
}

// DeleteMicroVM implements [Provider]. Returns [ErrVMNotFound] when no
// VM is indexed under id; otherwise the entry is removed from the
// index and the VM is torn down. If teardown fails the chroot is left
// on disk for [Provider.Recover] to pick up on the next start (or for
// the next [CreateMicroVM] of the same id to wipe via the existing
// stale-state cleanup).
func (f *firecracker) DeleteMicroVM(ctx context.Context, id string) error {
	f.mu.Lock()
	vm, ok := f.vms[id]
	if !ok {
		f.mu.Unlock()
		return ErrVMNotFound
	}
	delete(f.vms, id)
	f.mu.Unlock()

	return vm.destroy(ctx)
}

// Recover implements [Provider]. It scans the configured chroot base
// directory for per-VM subdirectories and re-attaches to each VM whose
// API socket still answers DescribeInstance. Per-VM failures are
// logged via the standard library logger and do not abort the scan;
// the only error this method returns is a failure to read the chroot
// base directory itself (other than its absence, which is treated as
// "no VMs to recover").
func (f *firecracker) Recover(ctx context.Context) error {
	root := filepath.Join(f.options.ChrootBaseDir, f.execName())
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read chroot base dir %s: %w", root, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if !vmIDPattern.MatchString(id) {
			log.Printf("firecracker: skipping non-vm directory %q in chroot", id)
			continue
		}
		vm, err := f.attach(ctx, id)
		switch {
		case err == nil:
			f.indexInsert(vm)
			log.Printf("firecracker: recovered microVM %s (pid=%d)", id, vm.pid)
		case errors.Is(err, ErrVMNotFound):
			log.Printf("firecracker: stale chroot for microVM %s (left in place for inspection)", id)
		default:
			log.Printf("firecracker: cannot attach to microVM %s: %v", id, err)
		}
	}
	return nil
}
