package network

import (
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// EnsureDummyInterface ensures a dummy network interface named
// ifname exists in the host namespace with addr assigned and the
// link UP. Idempotent: if the interface already exists with a
// different IP, the IP is reassigned; if the interface exists with
// the same IP, the call is a no-op.
//
// The DNS responder binds to a daemon-owned dummy IP rather than to
// `:53` so port collisions with the host's resolver stack (typically
// systemd-resolved on 127.0.0.53) don't kill the bind. Routing
// works because the guest reaches the dummy IP via its default
// route — the namespace forwards out through the veth, the host
// recognises the destination as a local IP, and the kernel
// delivers locally to the listener socket.
//
// The interface is intentionally not configured as a veth peer:
// dummy(4) carries no traffic of its own, it exists solely so the
// kernel will route packets destined to addr to a local socket.
func EnsureDummyInterface(ifname string, addr netip.Addr) error {
	if ifname == "" {
		return fmt.Errorf("ifname must be non-empty")
	}
	if !addr.IsValid() {
		return fmt.Errorf("addr must be valid (got %q)", addr)
	}

	link, err := netlink.LinkByName(ifname)
	switch {
	case err == nil:
		// Already exists — make sure it's a dummy. A type
		// mismatch means an operator wired something else into
		// the same name; refuse rather than blindly mutate.
		if link.Type() != "dummy" {
			return fmt.Errorf("interface %q exists but is of type %q, not dummy", ifname, link.Type())
		}
	case isLinkNotFound(err):
		dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: ifname}}
		if err := netlink.LinkAdd(dummy); err != nil {
			return fmt.Errorf("create dummy %q: %w", ifname, err)
		}
		link = dummy
	default:
		return fmt.Errorf("lookup %q: %w", ifname, err)
	}

	want := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   net.IP(addr.AsSlice()),
			Mask: hostmask(addr),
		},
	}
	existing, err := netlink.AddrList(link, unix.AF_UNSPEC)
	if err != nil {
		return fmt.Errorf("list addrs on %q: %w", ifname, err)
	}
	for _, a := range existing {
		if a.IP.Equal(want.IP) && a.Mask.String() == want.Mask.String() {
			// Already present — proceed straight to LinkSetUp.
			goto bringUp
		}
	}
	// Wipe any other addresses so the interface presents a single,
	// known identity. AddrFlush ignores ENOENT internally.
	for _, a := range existing {
		_ = netlink.AddrDel(link, &a)
	}
	if err := netlink.AddrAdd(link, want); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("assign %s to %q: %w", addr, ifname, err)
	}

bringUp:
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring %q up: %w", ifname, err)
	}
	return nil
}

// hostmask returns the /32 (v4) or /128 (v6) mask for a single-host
// address. The dummy interface only ever holds the resolver IP, so
// the broadest possible mask narrows broadcast traffic and makes the
// intent obvious in `ip addr show`.
func hostmask(addr netip.Addr) net.IPMask {
	if addr.Is4() {
		return net.CIDRMask(32, 32)
	}
	return net.CIDRMask(128, 128)
}
