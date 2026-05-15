// Command agent is the PID-1 entry point that runs inside Firecracker
// guest VMs. It performs the minimal init-system work the guest server
// relies on — mounting /proc, /sys and devpts, reaping orphaned
// grandchildren, translating SIGINT/SIGTERM into a context cancel —
// then hands off to [guest.ListenAndServe]. When the server returns
// the process exits, which causes the kernel to panic the VM down,
// matching the reboot=k panic=1 boot args the daemon supplies.
package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/alan-ghelardi/yaghan/agent/internal/guest"
)

const (
	// defaultVsockPort is used when /proc/cmdline omits agent.port or
	// carries a malformed value. 1024 matches the daemon's
	// agentVsockPort constant so a misconfigured boot args still
	// produces a reachable agent.
	defaultVsockPort uint32 = 1024

	// cmdlineKey is the /proc/cmdline token the daemon injects via
	// BootArgs to name the vsock port.
	cmdlineKey = "agent.port"

	// dnsCmdlineKey is the /proc/cmdline token the daemon injects to
	// seed /etc/resolv.conf. The value is a comma-separated list of
	// resolver IPs; absent means the guest gets no nameserver entry
	// and DNS resolution will fall back to whatever the rootfs
	// already ships with (or fail).
	dnsCmdlineKey = "agent.dns"

	// resolvConfPath is the location the agent writes nameserver
	// entries to. Standard glibc/musl location — both libcs read
	// from here on every resolve.
	resolvConfPath = "/etc/resolv.conf"

	// logFilePath is where the agent writes its own log. Firecracker
	// runs with --daemonize, so anything the kernel forwards to ttyS0
	// is discarded; a file on the rootfs is the only output surface
	// an operator can inspect after the fact (e.g. via SSH).
	logFilePath = "/var/log/agent.log"

	// logPrefix labels every line so mixed-source console dumps stay
	// attributable.
	logPrefix = "agent: "

	// defaultPATH is the PATH we install on the agent process when the
	// kernel boots us with an empty environment. Matches the
	// systemd / Alpine /etc/profile convention so guest commands
	// resolve the same way they would under a normal login shell.
	defaultPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

func main() {
	logger, closeLog := newLogger()
	defer closeLog()

	logger.Printf("booting as pid %d", os.Getpid())

	mountPseudoFS(logger)
	ensureBaseDirs(logger)
	ensureDefaultPath(logger)
	writeResolvConf(logger)

	port := parseVsockPort(logger)
	logger.Printf("vsock port %d", port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	guest.ListenAndServe(ctx, logger, port)
	logger.Printf("server exited; init returning — kernel will shut down the VM")
}

// newLogger builds a logger that tees to stderr and [logFilePath]. If
// the log file cannot be opened for any reason we fall back to
// stderr-only and report the failure to stderr — logging is useful
// enough that a lost file must never prevent the agent from coming up.
// The returned closer is safe to call once; on VM shutdown the kernel
// also syncs the disk.
func newLogger() (*log.Logger, func()) {
	stderrLogger := log.New(os.Stderr, logPrefix, log.LstdFlags)

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0o755); err != nil {
		stderrLogger.Printf("mkdir %s: %v (logging to stderr only)", filepath.Dir(logFilePath), err)
		return stderrLogger, func() {}
	}
	f, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		stderrLogger.Printf("open %s: %v (logging to stderr only)", logFilePath, err)
		return stderrLogger, func() {}
	}
	return log.New(io.MultiWriter(os.Stderr, f), logPrefix, log.LstdFlags), func() { _ = f.Close() }
}

// pseudoMount describes one pseudo-filesystem to install during boot.
type pseudoMount struct {
	source string
	target string
	fstype string
	flags  uintptr
	data   string
}

// mountPseudoFS installs the filesystems the guest server depends on.
// Each mount is best-effort: EBUSY (already mounted) is silently
// accepted so we stay idempotent when the kernel or an initramfs has
// already done the work.
//
// Order matters. devtmpfs on /dev must be first — vsock.Listen opens
// /dev/vsock, pty.Start opens /dev/ptmx, and everything else
// implicitly relies on the standard device nodes devtmpfs populates.
// /dev/pts is then mounted inside the now-populated /dev.
func mountPseudoFS(logger *log.Logger) {
	pseudoFlags := uintptr(syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV)
	mounts := []pseudoMount{
		// devtmpfs is where the kernel publishes device nodes
		// (/dev/vsock, /dev/null, /dev/ptmx, …). Minimal rootfs
		// images often ship without a /dev directory, in which case
		// the kernel's own auto-mount fails silently and we have to
		// do it here.
		{source: "devtmpfs", target: "/dev", fstype: "devtmpfs",
			flags: syscall.MS_NOSUID, data: "mode=0755"},
		{source: "proc", target: "/proc", fstype: "proc", flags: pseudoFlags},
		{source: "sysfs", target: "/sys", fstype: "sysfs", flags: pseudoFlags},
		// devpts is NOSUID + NOEXEC but drops NODEV — pts slaves are
		// character devices. gid=5 matches the conventional "tty"
		// group; mode 620 keeps pts endpoints owner-rw + group-w.
		{source: "devpts", target: "/dev/pts", fstype: "devpts",
			flags: syscall.MS_NOSUID | syscall.MS_NOEXEC,
			data:  "gid=5,mode=620,ptmxmode=666"},
	}
	for _, m := range mounts {
		if err := os.MkdirAll(m.target, 0o755); err != nil {
			logger.Printf("mkdir %s: %v", m.target, err)
			continue
		}
		if err := syscall.Mount(m.source, m.target, m.fstype, m.flags, m.data); err != nil {
			if errors.Is(err, syscall.EBUSY) {
				continue
			}
			logger.Printf("mount %s → %s: %v", m.fstype, m.target, err)
		}
	}
}

// ensureDefaultPath sets PATH on the agent process when the kernel
// hands us an empty environment. The agent runs as PID 1, so there is
// no shell to source /etc/profile and seed PATH; without this, Go's
// exec.Command resolves a bare program name (e.g. "ls") via LookPath
// against the agent's own PATH, which is "" — and the spawn fails
// before any guest env can apply.
//
// Two behaviours fall out of setting it here:
//  1. LookPath finds the standard guest binaries (Alpine's BusyBox
//     symlinks under /bin and /sbin).
//  2. makeEnviron's os.Environ() snapshot now carries PATH, so spawned
//     children inherit a sensible default unless the request overrides
//     it.
//
// We never overwrite an explicit PATH the kernel cmdline (or a
// hypothetical pre-init step) might have set.
func ensureDefaultPath(logger *log.Logger) {
	if os.Getenv("PATH") != "" {
		return
	}
	if err := os.Setenv("PATH", defaultPATH); err != nil {
		logger.Printf("setenv PATH: %v", err)
		return
	}
	logger.Printf("PATH defaulted to %q", defaultPATH)
}

// ensureBaseDirs creates standard writable directories the guest
// server (and commands it spawns) expect to exist. Minimal rootfs
// images sometimes ship without /tmp and friends; a sandbox that
// can't write a temp file is useless.
func ensureBaseDirs(logger *log.Logger) {
	// (path, mode) pairs. /tmp and /var/tmp want the sticky bit (01777)
	// so unprivileged callers can share them safely.
	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{"/tmp", 0o1777},
		{"/var/tmp", 0o1777},
		{"/run", 0o755},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.mode); err != nil {
			logger.Printf("mkdir %s: %v", d.path, err)
			continue
		}
		// MkdirAll honours umask; force the mode (esp. sticky bit).
		if err := os.Chmod(d.path, d.mode); err != nil {
			logger.Printf("chmod %s: %v", d.path, err)
		}
	}
}

// writeResolvConf seeds /etc/resolv.conf with the resolvers the daemon
// passed in via agent.dns=. Missing token or unreadable cmdline is
// non-fatal — the agent still comes up, the guest just won't resolve
// names. Any pre-existing /etc/resolv.conf in the rootfs is replaced,
// since the daemon's value is the authoritative one for this boot.
func writeResolvConf(logger *log.Logger) {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		logger.Printf("read /proc/cmdline for DNS: %v (skipping resolv.conf)", err)
		return
	}
	var servers []string
	for _, tok := range strings.Fields(string(data)) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || k != dnsCmdlineKey || v == "" {
			continue
		}
		for _, ip := range strings.Split(v, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				servers = append(servers, ip)
			}
		}
		break
	}
	if len(servers) == 0 {
		logger.Printf("no %s= on cmdline; leaving resolv.conf untouched", dnsCmdlineKey)
		return
	}
	var b strings.Builder
	for _, s := range servers {
		b.WriteString("nameserver ")
		b.WriteString(s)
		b.WriteByte('\n')
	}
	// 0o644 is the conventional resolv.conf mode — libc resolvers in
	// non-root processes (the workload running inside the sandbox)
	// need read access, and 0o600 would prevent DNS resolution for
	// any uid other than root.
	if err := os.WriteFile(resolvConfPath, []byte(b.String()), 0o644); err != nil { //nolint:gosec // see comment above
		logger.Printf("write %s: %v (DNS will not resolve in this guest)", resolvConfPath, err)
		return
	}
	logger.Printf("wrote %s with %d nameserver(s)", resolvConfPath, len(servers))
}

// parseVsockPort reads /proc/cmdline once at startup and extracts the
// agent.port=N token the daemon injects. Falls back to
// [defaultVsockPort] for any read, parse, or range failure — the agent
// should still come up on a sane default so the VM is at least
// inspectable via the known UDS path.
func parseVsockPort(logger *log.Logger) uint32 {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		logger.Printf("read /proc/cmdline: %v (falling back to port %d)", err, defaultVsockPort)
		return defaultVsockPort
	}
	for _, tok := range strings.Fields(string(data)) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || k != cmdlineKey {
			continue
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n == 0 || n > 65535 {
			logger.Printf("invalid %s=%q, falling back to port %d", cmdlineKey, v, defaultVsockPort)
			return defaultVsockPort
		}
		return uint32(n)
	}
	return defaultVsockPort
}
