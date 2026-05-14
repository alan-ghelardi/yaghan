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
	hostForwardChain = "NUINFRA-FORWARD"
	hostNATChain     = "NUINFRA-POSTROUTING"

	// ruleComment tags every rule the daemon installs. It serves two
	// purposes: it makes the rules trivially greppable in iptables-save
	// output, and it lets a future Sweep() distinguish daemon rules
	// from anything else without parsing rule-specs.
	ruleComment = "nuinfra"
)

// adder is the small subset of iptables.IPTables the firewall uses,
// extracted so tests can substitute a recording fake.
type adder interface {
	ChainExists(table, chain string) (bool, error)
	NewChain(table, chain string) error
	AppendUnique(table, chain string, rulespec ...string) error
}

// NewIPTables returns a [Firewall] backed by go-iptables.
func NewIPTables() (Firewall, error) {
	ipt, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv4))
	if err != nil {
		return nil, fmt.Errorf("init iptables: %w", err)
	}
	return &iptablesFirewall{ipt: ipt}, nil
}

type iptablesFirewall struct {
	ipt adder
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

	// filter: ensure NUINFRA-FORWARD exists and is jumped to from FORWARD.
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

	// nat: ensure NUINFRA-POSTROUTING exists and is jumped to from POSTROUTING.
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

// ConfigureNamespace implements [Firewall]. The thread must already
// be in the target network namespace — see the package docstring.
//
// Rules are appended directly to the default FORWARD and nat/POSTROUTING
// chains rather than into our own chain, because the namespace is owned
// entirely by one VM and is destroyed on Deprovision; there is no
// foreign-rule contamination risk to worry about.
func (f *iptablesFirewall) ConfigureNamespace(c NamespaceConfig) error {
	if c.NamespaceVethName == "" {
		return fmt.Errorf("namespace veth name must be non-empty")
	}
	if !c.GuestSubnet.IsValid() {
		return fmt.Errorf("guest subnet %q is invalid", c.GuestSubnet)
	}
	subnet := c.GuestSubnet.String()

	// Forward guest traffic out the veth, and conntrack return traffic
	// back. Two explicit ACCEPT rules sidestep any non-default FORWARD
	// policy a security-hardened kernel might ship with.
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-s", subnet, "-o", c.NamespaceVethName,
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow ns forward egress: %w", err)
	}
	if err := f.ipt.AppendUnique("filter", "FORWARD",
		"-i", c.NamespaceVethName, "-d", subnet,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", ruleComment,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("allow ns forward return: %w", err)
	}

	// MASQUERADE: rewrite the (per-VM-duplicated) guest source IP to
	// the namespace-side veth address so the host sees a unique src.
	if err := f.ipt.AppendUnique("nat", "POSTROUTING",
		"-s", subnet, "-o", c.NamespaceVethName,
		"-m", "comment", "--comment", ruleComment,
		"-j", "MASQUERADE",
	); err != nil {
		return fmt.Errorf("masquerade ns egress: %w", err)
	}
	return nil
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
