package network

import (
	"net/netip"

	"github.com/alan-ghelardi/yaghan/daemon/pkg/network/firewall"
)

// Options controls the behavior of a [Driver]. Construct an Options value
// implicitly by passing [Option] values to [NewLinuxDriver].
type Options struct {
	// TAPOwner sets the uid allowed to attach to the TAP device
	// (vm-tap0). Required when firecracker runs under a non-root jailer
	// uid; otherwise TUNSETIFF returns EPERM. Zero means "leave unset",
	// which restricts attachment to root.
	TAPOwner uint32

	// TAPGroup sets the gid allowed to attach to the TAP device.
	// Zero means "leave unset".
	TAPGroup uint32

	// Firewall installs the netfilter rules required for VM egress
	// connectivity. Nil disables firewall configuration entirely;
	// in that case the driver still provisions the netns/TAP/veth
	// topology but the guest will not reach anything beyond the
	// host-side of its own veth (the pre-egress behaviour).
	Firewall firewall.Firewall

	// ResolverIP is the daemon-wide DNS responder address. It is
	// reachable from inside every sandbox via the namespace's
	// default route (the host-side veth) and surfaces as a local
	// IP on the host (typically pinned to a dummy interface). The
	// per-namespace firewall installs a resolver carve-out for
	// this address under domain-bearing allow policies.
	//
	// Zero (the default) signals "no in-daemon responder
	// configured" and disables the carve-out. Sandboxes whose
	// policies reference domain_names are rejected before they
	// reach the driver in that case (see Reconciler.checkDNSPrereq).
	ResolverIP netip.Addr
}

// Option represents a single override applied to [Options].
type Option interface {
	Apply(opts *Options)
}

// OptionAdapter adapts a function to satisfy the Option interface.
type OptionAdapter func(opts *Options)

// Apply implements the Option interface.
func (f OptionAdapter) Apply(opts *Options) {
	f(opts)
}

// WithTAPOwner sets the uid allowed to attach to the TAP device.
func WithTAPOwner(uid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.TAPOwner = uid
	})
}

// WithTAPGroup sets the gid allowed to attach to the TAP device.
func WithTAPGroup(gid uint32) Option {
	return OptionAdapter(func(opts *Options) {
		opts.TAPGroup = gid
	})
}

// WithFirewall installs fw as the driver's firewall configurator.
// Passing nil (or omitting WithFirewall entirely) leaves egress
// disabled — useful in tests and on hosts where the operator manages
// netfilter rules externally.
func WithFirewall(fw firewall.Firewall) Option {
	return OptionAdapter(func(opts *Options) {
		opts.Firewall = fw
	})
}

// WithResolverIP records the daemon-wide DNS responder address.
// The driver propagates it into [firewall.NamespaceConfig.ResolverIP]
// when configuring per-namespace egress, so domain-bearing policies
// get a resolver carve-out at this address. Omitting the option (or
// passing the zero value) leaves the carve-out disabled, which is
// safe when the responder is not running — sandboxes that reference
// domain_names are rejected before reaching the driver in that case.
func WithResolverIP(ip netip.Addr) Option {
	return OptionAdapter(func(opts *Options) {
		opts.ResolverIP = ip
	})
}
