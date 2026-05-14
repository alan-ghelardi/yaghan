package network

import (
	"fmt"
	"os"
)

// ipForwardPath is the procfs path for the IPv4 forwarding sysctl. The
// value is resolved per network namespace by the kernel — writing to
// this path from a thread that has setns'd into a target namespace
// affects only that namespace's forwarding flag.
const ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

// EnableHostIPForward sets net.ipv4.ip_forward=1 in the host network
// namespace. Required so the kernel will forward packets from
// host-veth* interfaces out the upstream device. Idempotent — writing
// "1" to an already-1 sysctl is a no-op.
func EnableHostIPForward() error {
	return writeSysctl(ipForwardPath, "1")
}

// EnableNamespaceIPForward sets net.ipv4.ip_forward=1 in whichever
// network namespace the calling OS thread is currently in. The caller
// is responsible for [runtime.LockOSThread] and the setns dance — same
// convention as the firewall package.
func EnableNamespaceIPForward() error {
	return writeSysctl(ipForwardPath, "1")
}

// writeSysctl writes value to a /proc/sys path. Truncates so partial
// previous values don't leak; sysctl files accept the value followed
// by a newline.
func writeSysctl(path, value string) error {
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return fmt.Errorf("write sysctl %s=%s: %w", path, value, err)
	}
	return nil
}
