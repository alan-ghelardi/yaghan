// Package firewall manages the netfilter rules required for per-VM
// egress connectivity. Two surfaces:
//
//   - Host-wide rules ([Firewall.EnsureHost]). One-time, daemon-owned
//     chains in the host network namespace that forward and MASQUERADE
//     traffic from the VM veth pool out the upstream egress device.
//
//   - Per-VM rules ([Firewall.ConfigureNamespace]). Installed inside a
//     VM's network namespace so the kernel forwards between the guest
//     TAP and the namespace-side veth, and MASQUERADEs the (deliberately
//     duplicated) guest IP to the unique namespace-side veth address
//     before the packet crosses into the host namespace.
//
// Per-VM state is contained entirely inside the VM's network namespace
// — deleting the namespace (which Deprovision does) removes those rules
// implicitly, so the firewall package has no per-VM teardown method.
//
// The default implementation shells out to iptables via go-iptables.
// On modern distros (Ubuntu 22.04+, Debian bookworm, Amazon Linux 2023)
// /sbin/iptables is iptables-nft, so the rules end up in the nftables
// kernel backend even though the user-space API is classic iptables.
package firewall

import "net/netip"

// Firewall installs the netfilter rules required for VM egress.
// Implementations must be safe for concurrent use.
type Firewall interface {
	// EnsureHost installs the daemon-wide rules in the host network
	// namespace: a forward-accept rule pair for vmSubnet on upstream
	// and a MASQUERADE of vmSubnet on upstream. Idempotent — safe to
	// call on every daemon start.
	EnsureHost(upstream string, vmSubnet netip.Prefix) error

	// ConfigureNamespace installs per-VM rules. The calling OS thread
	// must already be in the target network namespace; the function
	// does not perform a setns dance itself, so the caller controls
	// the thread-locking lifecycle (same convention as netlink
	// operations elsewhere in this codebase).
	ConfigureNamespace(NamespaceConfig) error
}

// NamespaceConfig is the input to [Firewall.ConfigureNamespace]. All
// fields refer to entities inside the target network namespace.
type NamespaceConfig struct {
	// NamespaceVethName is the namespace-side veth interface (e.g.
	// "vm-veth0"). MASQUERADE and FORWARD rules pin to this device
	// so the rules cannot match traffic from any other interface
	// that might be added later.
	NamespaceVethName string

	// GuestSubnet is the CIDR the guest's virtio-net interface
	// occupies (e.g. 172.16.0.0/30). Both directions of MASQUERADE
	// and FORWARD use it as the match key.
	GuestSubnet netip.Prefix
}
