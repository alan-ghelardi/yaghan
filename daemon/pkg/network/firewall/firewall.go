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

	// EnsureDNSAccess installs INPUT-chain rules that permit DNS
	// traffic from the VM subnet to the in-daemon resolver IP. The
	// dummy interface carrying resolverIP routes packets locally,
	// so they hit the INPUT chain — which on most distros (Ubuntu
	// with UFW, RHEL with firewalld) defaults to DROP. Without
	// this carve-out the resolver is unreachable from any sandbox.
	// Idempotent. Pass the same resolverIP supplied to
	// [WithResolverIP] on the network driver.
	EnsureDNSAccess(resolverIP netip.Addr, vmSubnet netip.Prefix) error

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

	// Egress optionally constrains the destinations the guest can
	// reach. A nil value preserves the legacy behavior (unrestricted
	// egress). See [EgressPolicy] for the semantics.
	Egress *EgressPolicy

	// SandboxIndex is the per-host unique identifier of the sandbox
	// being configured. Used to derive the per-sandbox ipset name
	// for domain-based dynamic allow rules; ignored when Egress is
	// nil or carries no Domains.
	SandboxIndex int

	// ResolverIP is the address the guest is configured to use as
	// its DNS resolver — the sandbox's host-side veth IP. The
	// firewall carves out a resolver-only ACCEPT for this address on
	// port 53 under allow-mode policies that reference Domains, so a
	// domain-only allow rule still leaves DNS reachable. Ignored
	// when Egress is nil or carries no Domains.
	ResolverIP netip.Addr
}

// EgressMode discriminates between the two policy verdicts an
// [EgressPolicy] can carry.
type EgressMode int

const (
	// EgressAllow means only destinations listed in the policy are
	// reachable; everything else is rejected.
	EgressAllow EgressMode = iota + 1

	// EgressDeny means every destination is reachable except those
	// listed in the policy.
	EgressDeny
)

// EgressPolicy is the firewall-internal projection of a sandbox's
// egress policy.
//
// IPs and CIDRs are translated into static allow/reject rules at
// boot. Domains, when non-empty, switch on the per-sandbox dynamic
// path: ConfigureNamespace creates the per-sandbox ipset and installs
// a `--match-set` ACCEPT (allow mode) plus a resolver carve-out so
// the in-daemon DNS handler can reach the guest. The handler is what
// populates the ipset at runtime; the firewall only owns the
// scaffolding. The Domains list itself is not consulted by the
// firewall — match decisions are made by the DNS handler against the
// control-plane proto.
type EgressPolicy struct {
	Mode    EgressMode
	IPs     []netip.Addr
	CIDRs   []netip.Prefix
	Domains []string
}
