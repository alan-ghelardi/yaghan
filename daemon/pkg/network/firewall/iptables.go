package firewall

import (
	"fmt"
	"net/netip"

	"github.com/coreos/go-iptables/iptables"
)

// Names for the daemon-owned host chains. Rules in default chains are
// limited to a single jump into one of these; everything else lives in
// here so foreign rules (operator-installed, k8s, etc.) stay
// untouched.
const (
	hostForwardChain = "YAGHAN-FORWARD"
	hostNATChain     = "YAGHAN-POSTROUTING"

	// ruleComment tags every rule the daemon installs. It serves two
	// purposes: it makes the rules trivially greppable in iptables-save
	// output, and it lets a future Sweep() distinguish daemon rules
	// from anything else without parsing rule-specs.
	ruleComment = "yaghan"
)

// dnsPort is the destination port the resolver carve-out unblocks.
// Only port 53 — sandboxes have no reason to speak other protocols
// to the daemon's loopback-equivalent address, and a narrow carve-out
// keeps the security boundary tight under allow-mode policies.
const dnsPort = "53"

// adder is the small subset of iptables.IPTables the firewall uses,
// extracted so tests can substitute a recording fake.
type adder interface {
	ChainExists(table, chain string) (bool, error)
	NewChain(table, chain string) error
	AppendUnique(table, chain string, rulespec ...string) error
}

// setMaker is the small subset of [IPSet] ConfigureNamespace
// consumes. Extracted as an interface so tests can substitute a
// recording fake (mirrors the `adder` pattern above).
type setMaker interface {
	EnsureSandboxSet(index int) error
}

// NewIPTables returns a [Firewall] backed by go-iptables and the
// default shell-out ipset client.
func NewIPTables() (Firewall, error) {
	ipt, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv4))
	if err != nil {
		return nil, fmt.Errorf("init iptables: %w", err)
	}
	return &iptablesFirewall{ipt: ipt, sets: NewIPSet()}, nil
}

type iptablesFirewall struct {
	ipt  adder
	sets setMaker
}

// EnsureHost implements [Firewall].
func (f *iptablesFirewall) EnsureHost(upstream string, vmSubnet netip.Prefix) error {
	if upstream == "" {
		return fmt.Errorf("upstream device must be non-empty")
	}
	if !vmSubnet.IsValid() {
		return fmt.Errorf("vm subnet %q is invalid", vmSubnet)
	}
	subnet := vmSubnet.String()

	// filter: ensure YAGHAN-FORWARD exists and is jumped to from FORWARD.
	if err := ensureChain(f.ipt, "filter", hostForwardChain); err != nil {
		return err
	}
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-m", "comment", "--comment", ruleComment,
		"-j", hostForwardChain,
	); err != nil {
		return fmt.Errorf("link filter/FORWARD → %s: %w", hostForwardChain, err)
	}
	// Egress: traffic sourced from the VM pool leaving via upstream.
	if err := f.ipt.AppendUnique("filter", hostForwardChain,
		"-s", subnet, "-o", upstream,
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow host forward egress: %w", err)
	}
	// Return path: ESTABLISHED/RELATED replies coming back from upstream.
	if err := f.ipt.AppendUnique("filter", hostForwardChain,
		"-i", upstream, "-d", subnet,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow host forward return: %w", err)
	}

	// nat: ensure YAGHAN-POSTROUTING exists and is jumped to from POSTROUTING.
	if err := ensureChain(f.ipt, "nat", hostNATChain); err != nil {
		return err
	}
	if err := f.ipt.AppendUnique("nat", "POSTROUTING",
		"-m", "comment", "--comment", ruleComment,
		"-j", hostNATChain,
	); err != nil {
		return fmt.Errorf("link nat/POSTROUTING → %s: %w", hostNATChain, err)
	}
	// MASQUERADE: rewrite source IP to whatever upstream's primary
	// address is so return traffic finds its way back.
	if err := f.ipt.AppendUnique("nat", hostNATChain,
		"-s", subnet, "-o", upstream,
		"-m", "comment", "--comment", ruleComment,
		"-j", "MASQUERADE",
	); err != nil {
		return fmt.Errorf("masquerade host egress: %w", err)
	}
	return nil
}

// EnsureDNSAccess implements [Firewall]. Adds two INPUT rules
// (udp + tcp on port 53) that permit traffic from the VM subnet
// destined to the in-daemon resolver address. We anchor the rules
// on -s vmSubnet rather than -i <dummy> because the kernel routes
// the packet locally based on the destination IP — the input
// interface is the host-veth the sandbox traffic arrives on, not
// the dummy that owns the resolver IP, so a -i match would never
// fire.
func (f *iptablesFirewall) EnsureDNSAccess(resolverIP netip.Addr, vmSubnet netip.Prefix) error {
	if !resolverIP.IsValid() {
		return fmt.Errorf("resolver ip is invalid")
	}
	if !vmSubnet.IsValid() {
		return fmt.Errorf("vm subnet %q is invalid", vmSubnet)
	}
	subnet := vmSubnet.String()
	dst := resolverIP.String()
	for _, proto := range [...]string{"udp", "tcp"} {
		if err := f.ipt.AppendUnique("filter", "INPUT",
			"-s", subnet, "-d", dst,
			"-p", proto, "--dport", dnsPort,
			"-m", "comment", "--comment", ruleComment,
			"-j", "ACCEPT",
		); err != nil {
			return fmt.Errorf("allow host input %s to %s: %w", proto, dst, err)
		}
	}
	return nil
}

// ConfigureNamespace implements [Firewall]. The thread must already
// be in the target network namespace — see the package docstring.
//
// Rules are appended directly to the default FORWARD and nat/POSTROUTING
// chains rather than into our own chain, because the namespace is owned
// entirely by one VM and is destroyed on Deprovision; there is no
// foreign-rule contamination risk to worry about.
//
// Rule ordering matters: the egress filter (when an [EgressPolicy] is
// supplied) must precede any fall-through ACCEPT for the policy
// verdict to take effect. AppendUnique preserves insertion order, so
// we just issue the rules in the order we want them evaluated.
func (f *iptablesFirewall) ConfigureNamespace(c NamespaceConfig) error {
	if c.NamespaceVethName == "" {
		return fmt.Errorf("namespace veth name must be non-empty")
	}
	if !c.GuestSubnet.IsValid() {
		return fmt.Errorf("guest subnet %q is invalid", c.GuestSubnet)
	}
	subnet := c.GuestSubnet.String()

	// Conntrack return path is always permitted, regardless of policy:
	// without it the guest would never see the replies to its own
	// outbound connections.
	if err := f.appendReturnAccept(subnet, c.NamespaceVethName); err != nil {
		return err
	}

	// Egress filter — order-sensitive. The dispatch table is small
	// enough to inline; one branch per supported policy mode plus the
	// nil case (legacy unrestricted egress).
	switch {
	case c.Egress == nil:
		if err := f.appendEgressAccept(subnet, c.NamespaceVethName); err != nil {
			return err
		}
	case c.Egress.Mode == EgressAllow:
		if err := f.appendEgressAllow(subnet, c); err != nil {
			return err
		}
	case c.Egress.Mode == EgressDeny:
		if err := f.appendEgressDeny(subnet, c.NamespaceVethName, c.Egress); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported egress mode %d", c.Egress.Mode)
	}

	// MASQUERADE: rewrite the (per-VM-duplicated) guest source IP to
	// the namespace-side veth address so the host sees a unique src.
	// Installed after the filter so packets the policy rejects never
	// consume a conntrack slot.
	if err := f.ipt.AppendUnique("nat", "POSTROUTING",
		"-s", subnet, "-o", c.NamespaceVethName,
		"-m", "comment", "--comment", ruleComment,
		"-j", "MASQUERADE",
	); err != nil {
		return fmt.Errorf("masquerade ns egress: %w", err)
	}
	return nil
}

// appendReturnAccept opens the conntrack return path: replies to the
// guest's own outbound connections enter back through the veth and
// must reach the guest subnet.
func (f *iptablesFirewall) appendReturnAccept(subnet, veth string) error {
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-i", veth, "-d", subnet,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow ns forward return: %w", err)
	}
	return nil
}

// appendEgressAccept installs the unrestricted egress ACCEPT — the
// legacy behavior used when no [EgressPolicy] is supplied.
func (f *iptablesFirewall) appendEgressAccept(subnet, veth string) error {
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-s", subnet, "-o", veth,
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow ns forward egress: %w", err)
	}
	return nil
}

// appendEgressAllow installs an allow-list egress filter: one ACCEPT
// per listed IP and CIDR, optional dynamic infrastructure for
// Domains, followed by a terminal REJECT that fires for every
// destination not on the list. The REJECT uses icmp-admin-prohibited
// so guest clients see a fast EHOSTUNREACH-class failure rather than
// hanging until their own timeout.
//
// Rule order mirrors the precedence documented on EgressTargets:
// ip_addresses before cidr_blocks before domain_names (the ipset
// match). With a single verdict in play the order is observable only
// in logs today, but keeping it consistent makes future mixed-verdict
// policies behave predictably.
//
// When policy.Domains is non-empty the function also creates the
// per-sandbox ipset and adds a resolver carve-out so the guest can
// reach the in-daemon DNS handler (`resolverIP:53/{udp,tcp}`).
// Without the carve-out a domain-only allow policy would block its
// own resolver, breaking name resolution entirely. The carve-out is
// installed before the ipset match so it doesn't depend on a
// successful prior resolution to take effect.
func (f *iptablesFirewall) appendEgressAllow(subnet string, c NamespaceConfig) error {
	policy := c.Egress
	veth := c.NamespaceVethName
	for _, ip := range policy.IPs {
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth, "-d", ip.String(),
			"-m", "comment", "--comment", ruleComment,
			"-j", "ACCEPT",
		); err != nil {
			return fmt.Errorf("allow ns egress to %s: %w", ip, err)
		}
	}
	for _, cidr := range policy.CIDRs {
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth, "-d", cidr.String(),
			"-m", "comment", "--comment", ruleComment,
			"-j", "ACCEPT",
		); err != nil {
			return fmt.Errorf("allow ns egress to %s: %w", cidr, err)
		}
	}
	if len(policy.Domains) > 0 {
		if !c.ResolverIP.IsValid() {
			return fmt.Errorf("domain_names policy requires a valid ResolverIP")
		}
		if err := f.sets.EnsureSandboxSet(c.SandboxIndex); err != nil {
			return fmt.Errorf("ensure sandbox ipset: %w", err)
		}
		if err := f.appendResolverCarveOut(subnet, veth, c.ResolverIP); err != nil {
			return err
		}
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth,
			"-m", "set", "--match-set", SandboxIPSetName(c.SandboxIndex), "dst",
			"-m", "comment", "--comment", ruleComment,
			"-j", "ACCEPT",
		); err != nil {
			return fmt.Errorf("allow ns egress via ipset: %w", err)
		}
	}
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-s", subnet, "-o", veth,
		"-m", "comment", "--comment", ruleComment,
		"-j", "REJECT", "--reject-with", "icmp-admin-prohibited",
	); err != nil {
		return fmt.Errorf("reject ns egress fallthrough: %w", err)
	}
	return nil
}

// appendResolverCarveOut installs the udp/tcp ACCEPT pair that keeps
// the guest's DNS path open to the in-daemon resolver when an
// allow-mode policy carries domain_names but does not list the
// resolver's IP. Without it the terminal REJECT would catch DNS
// traffic and the domain-only policy would never resolve anything.
func (f *iptablesFirewall) appendResolverCarveOut(subnet, veth string, resolverIP netip.Addr) error {
	for _, proto := range [...]string{"udp", "tcp"} {
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth, "-d", resolverIP.String(),
			"-p", proto, "--dport", dnsPort,
			"-m", "comment", "--comment", ruleComment,
			"-j", "ACCEPT",
		); err != nil {
			return fmt.Errorf("allow ns resolver %s %s: %w", proto, resolverIP, err)
		}
	}
	return nil
}

// appendEgressDeny installs a deny-list egress filter: one REJECT per
// listed IP and CIDR, followed by the generic ACCEPT for everything
// else.
func (f *iptablesFirewall) appendEgressDeny(subnet, veth string, policy *EgressPolicy) error {
	for _, ip := range policy.IPs {
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth, "-d", ip.String(),
			"-m", "comment", "--comment", ruleComment,
			"-j", "REJECT", "--reject-with", "icmp-admin-prohibited",
		); err != nil {
			return fmt.Errorf("deny ns egress to %s: %w", ip, err)
		}
	}
	for _, cidr := range policy.CIDRs {
		if err := f.ipt.AppendUnique("filter", "FORWARD",
			"-s", subnet, "-o", veth, "-d", cidr.String(),
			"-m", "comment", "--comment", ruleComment,
			"-j", "REJECT", "--reject-with", "icmp-admin-prohibited",
		); err != nil {
			return fmt.Errorf("deny ns egress to %s: %w", cidr, err)
		}
	}
	return f.appendEgressAccept(subnet, veth)
}

// ensureChain creates chain in table if and only if it does not already
// exist. AppendUnique handles its own idempotency for rules, but chain
// creation must be guarded explicitly — NewChain returns an error if
// the chain is already present.
func ensureChain(a adder, table, chain string) error {
	exists, err := a.ChainExists(table, chain)
	if err != nil {
		return fmt.Errorf("check %s/%s exists: %w", table, chain, err)
	}
	if exists {
		return nil
	}
	if err := a.NewChain(table, chain); err != nil {
		return fmt.Errorf("create %s/%s: %w", table, chain, err)
	}
	return nil
}
