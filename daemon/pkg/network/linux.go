// Package network provisions per-VM network topologies for Firecracker
// guests.
//
// Each VM gets its own linux network namespace (vm-ns<index>) containing a
// TAP device (vm-tap0 at 172.16.0.1/30) and one end of a veth pair that
// connects back to the host. The host-side veth (host-veth<index>) is
// assigned a /30 carved deterministically out of 10.0.0.0/16, and the
// namespace's default route points at it.
//
// When [Options.Firewall] is set, Provision additionally enables
// net.ipv4.ip_forward inside the namespace and installs the per-VM
// MASQUERADE/FORWARD rules required for egress. The host-wide
// counterpart (single MASQUERADE for the entire VM pool plus the host
// ip_forward sysctl) is set up once at daemon startup via
// [EnableHostIPForward] and [firewall.Firewall.EnsureHost]. With no
// firewall configured Provision stops at L3 plumbing — guest can reach
// the host-side of its own veth, nothing further.
//
// Host prerequisites: the `tun` kernel module must be loaded and the
// process must hold CAP_NET_ADMIN (typically runs as root).
package network

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/alan-ghelardi/yaghan/daemon/pkg/network/firewall"
)

// netnsMountDir is the kernel-iproute2-compatible mount point where
// named network namespaces live. /var/run/netns symlinks here on
// systemd hosts. Used by hasStaleState to detect orphans from a
// previous daemon process that exited before Deprovision.
const netnsMountDir = "/run/netns"

// NewLinuxDriver returns a [Driver] backed by netlink and netns. The
// returned value is safe for concurrent use; mutating operations are
// serialised internally.
func NewLinuxDriver(opts ...Option) Driver {
	options := &Options{}
	for _, opt := range opts {
		opt.Apply(options)
	}
	return &linuxDriver{options: options}
}

type linuxDriver struct {
	mu      sync.Mutex
	options *Options
}

var _ Driver = (*linuxDriver)(nil)

// Provision implements [Driver].
func (d *linuxDriver) Provision(index int, egress *EgressPolicy) (NamespaceHandle, error) {
	if err := validateIndex(index); err != nil {
		return NamespaceHandle{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	meta := buildHandle(index)

	// Sweep any state left behind by a previous daemon process that
	// exited before its Deprovision could run: the named netns file
	// at /run/netns/<name> and the host-side veth both survive a
	// daemon crash, and without this sweep createNamespace + the
	// veth-add step would each fail with EEXIST. Safe to call here —
	// the reconciler picks `index` via provider.Len(), and the
	// firecracker provider's index reflects post-Recover live VMs,
	// so this index is by construction not associated with any
	// running microVM on this host.
	if d.hasStaleState(meta) {
		log.Printf("network: orphan state for %q (probable unclean shutdown of a previous run); cleaning up before re-provisioning",
			meta.NamespaceName)
		if err := d.deprovisionLocked(index); err != nil {
			return NamespaceHandle{}, fmt.Errorf("clean stale network state for index %d: %w", index, err)
		}
	}

	nsFd, err := createNamespace(meta.NamespaceName)
	if err != nil {
		return NamespaceHandle{}, fmt.Errorf("create namespace: %w", err)
	}
	// nsFd stays open for the lifetime of Provision; close after use.
	defer nsFd.Close()

	rollback := func(cause error) error {
		_ = d.deprovisionLocked(index) // best-effort cleanup on partial state
		return cause
	}

	if err := setupTAP(nsFd, d.options); err != nil {
		return NamespaceHandle{}, rollback(fmt.Errorf("setup tap: %w", err))
	}
	if err := setupVethPair(nsFd, meta); err != nil {
		return NamespaceHandle{}, rollback(fmt.Errorf("setup veth pair: %w", err))
	}
	if err := installDefaultRoute(nsFd, meta.HostVethIP); err != nil {
		return NamespaceHandle{}, rollback(fmt.Errorf("install default route: %w", err))
	}
	if d.options.Firewall != nil {
		if err := configureEgress(nsFd, d.options.Firewall, egress); err != nil {
			return NamespaceHandle{}, rollback(fmt.Errorf("configure egress: %w", err))
		}
	}
	return meta, nil
}

// Deprovision implements [Driver].
func (d *linuxDriver) Deprovision(index int) error {
	if err := validateIndex(index); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deprovisionLocked(index)
}

// hasStaleState reports whether any host-visible artifact for the
// supplied namespace handle already exists — either the named netns
// file at /run/netns/<name> or the host-side veth link. Used by
// Provision to decide whether to log (and clean up) before starting
// fresh; deprovisionLocked itself is naturally idempotent on absent
// state, so a wrong-positive here would be quiet but non-fatal.
func (d *linuxDriver) hasStaleState(meta NamespaceHandle) bool {
	if _, err := os.Stat(filepath.Join(netnsMountDir, meta.NamespaceName)); err == nil {
		return true
	}
	if _, err := netlink.LinkByName(meta.HostVethName); err == nil {
		return true
	}
	return false
}

// deprovisionLocked assumes d.mu is held. It is safe to call against a
// partially-provisioned or fully-absent topology.
func (d *linuxDriver) deprovisionLocked(index int) error {
	// Remove the host-side veth first. The kernel would auto-remove the
	// surviving peer when the namespace dies, but handling it explicitly
	// makes recovery from a half-built state idempotent.
	link, err := netlink.LinkByName(hostVethName(index))
	switch {
	case err == nil:
		if err := netlink.LinkDel(link); err != nil {
			return fmt.Errorf("delete host veth: %w", err)
		}
	case isLinkNotFound(err):
		// already gone
	default:
		return fmt.Errorf("lookup host veth: %w", err)
	}

	if err := netns.DeleteNamed(namespaceName(index)); err != nil && !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("delete netns: %w", err)
	}
	return nil
}

// createNamespace creates the named namespace and brings its loopback up.
// It returns a handle to the namespace that the caller must close.
//
// netns.NewNamed switches the calling OS thread into the new namespace
// before returning, so we runtime.LockOSThread beforehand and restore the
// original namespace on the way out. Without the lock, goroutines
// rescheduled onto the same thread would leak into the new namespace.
func createNamespace(name string) (netns.NsHandle, error) {
	runtime.LockOSThread()
	origin, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return 0, fmt.Errorf("get current netns: %w", err)
	}
	restore := func() {
		_ = netns.Set(origin)
		_ = origin.Close()
		runtime.UnlockOSThread()
	}

	nsFd, err := netns.NewNamed(name)
	if err != nil {
		restore()
		return 0, fmt.Errorf("create named netns %q: %w", name, err)
	}
	// We are now inside the new namespace. Bring up lo so anything binding
	// to 127.0.0.1 inside the guest works — NewNamed leaves lo DOWN.
	lo, err := netlink.LinkByName("lo")
	if err == nil {
		err = netlink.LinkSetUp(lo)
	}
	if err != nil {
		_ = netns.DeleteNamed(name)
		_ = nsFd.Close()
		restore()
		return 0, fmt.Errorf("bring lo up: %w", err)
	}

	restore()
	return nsFd, nil
}

// setupTAP creates vm-tap0 inside the given namespace, assigns the gateway
// IP, and brings the device up.
//
// Unlike pure rtnetlink operations, TAP creation goes through an ioctl on
// /dev/net/tun in the *current* thread's namespace, so NewHandleAt isn't
// sufficient here — the thread must actually enter the target namespace.
//
// When opts.TAPOwner / TAPGroup are non-zero they are applied as
// TUNSETOWNER / TUNSETGROUP so a non-root consumer (e.g. Firecracker
// under a jailer-dropped uid) can TUNSETIFF onto this device.
func setupTAP(nsFd netns.NsHandle, opts *Options) error {
	runtime.LockOSThread()
	origin, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("get origin netns: %w", err)
	}
	defer func() {
		_ = netns.Set(origin)
		_ = origin.Close()
		runtime.UnlockOSThread()
	}()
	if err := netns.Set(nsFd); err != nil {
		return fmt.Errorf("enter netns: %w", err)
	}

	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{Name: tapDeviceName},
		Mode:      netlink.TUNTAP_MODE_TAP,
		Flags:     netlink.TUNTAP_VNET_HDR | netlink.TUNTAP_NO_PI,
		// NonPersist defaults to false — the TAP stays alive across fd
		// close/open, which is required because Firecracker opens
		// /dev/net/tun itself. A non-persistent TAP would disappear as
		// soon as the netlink handle that created it closes.
		//
		// Owner/Group zero means "don't issue TUNSETOWNER/TUNSETGROUP",
		// which leaves the TAP attachable only by root. Non-zero values
		// authorize that uid/gid to TUNSETIFF onto this device.
		Owner: opts.TAPOwner,
		Group: opts.TAPGroup,
	}
	if err := netlink.LinkAdd(tap); err != nil {
		return fmt.Errorf("create tap %q (is the tun kernel module loaded?): %w", tapDeviceName, err)
	}
	if err := netlink.AddrAdd(tap, buildNetlinkAddr(tapIP, tapPrefixBits)); err != nil {
		return fmt.Errorf("assign tap ip: %w", err)
	}
	if err := netlink.LinkSetUp(tap); err != nil {
		return fmt.Errorf("bring tap up: %w", err)
	}
	return nil
}

// setupVethPair creates the host↔namespace veth pair, assigns IPs on both
// ends, and brings them up. The default route is installed separately so
// the caller controls ordering (the route needs both ends UP).
func setupVethPair(nsFd netns.NsHandle, meta NamespaceHandle) error {
	// Creating the pair with PeerNamespace places the peer directly inside
	// the target namespace in one atomic netlink op — no subsequent
	// LinkSetNsFd dance required.
	veth := &netlink.Veth{
		LinkAttrs:     netlink.LinkAttrs{Name: meta.HostVethName},
		PeerName:      namespaceVethName,
		PeerNamespace: netlink.NsFd(int(nsFd)),
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}

	// Host end lives in the current namespace; configure via the default
	// (unscoped) netlink functions.
	hostLink, err := netlink.LinkByName(meta.HostVethName)
	if err != nil {
		return fmt.Errorf("lookup host veth: %w", err)
	}
	if err := netlink.AddrAdd(hostLink, buildNetlinkAddr(meta.HostVethIP, vethSubnetBits)); err != nil {
		return fmt.Errorf("assign host veth ip: %w", err)
	}
	if err := netlink.LinkSetUp(hostLink); err != nil {
		return fmt.Errorf("bring host veth up: %w", err)
	}

	// Namespace end: use a handle scoped to nsFd so we don't have to
	// switch the thread in and out of the target namespace.
	nsHandle, err := netlink.NewHandleAt(nsFd)
	if err != nil {
		return fmt.Errorf("open ns netlink handle: %w", err)
	}
	defer nsHandle.Close()

	nsLink, err := nsHandle.LinkByName(namespaceVethName)
	if err != nil {
		return fmt.Errorf("lookup ns veth: %w", err)
	}
	_, nsIP, _ := vethSubnetFor(meta.Index)
	if err := nsHandle.AddrAdd(nsLink, buildNetlinkAddr(nsIP, vethSubnetBits)); err != nil {
		return fmt.Errorf("assign ns veth ip: %w", err)
	}
	if err := nsHandle.LinkSetUp(nsLink); err != nil {
		return fmt.Errorf("bring ns veth up: %w", err)
	}
	return nil
}

// configureEgress enables IPv4 forwarding inside the namespace and
// installs the per-VM MASQUERADE/FORWARD rules. Both operations
// require running inside the target namespace, so they share a single
// thread-lock + setns block to amortise the namespace dance.
//
// The sysctl write must come before the firewall rules: without
// forwarding enabled the FORWARD chain never fires, and any test
// using the rules would silently fail.
func configureEgress(nsFd netns.NsHandle, fw firewall.Firewall, egress *EgressPolicy) error {
	runtime.LockOSThread()
	origin, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("get origin netns: %w", err)
	}
	defer func() {
		_ = netns.Set(origin)
		_ = origin.Close()
		runtime.UnlockOSThread()
	}()
	if err := netns.Set(nsFd); err != nil {
		return fmt.Errorf("enter netns: %w", err)
	}

	if err := EnableNamespaceIPForward(); err != nil {
		return fmt.Errorf("enable ip_forward in netns: %w", err)
	}
	if err := fw.ConfigureNamespace(firewall.NamespaceConfig{
		NamespaceVethName: namespaceVethName,
		GuestSubnet:       GuestSubnet(),
		Egress:            egress,
	}); err != nil {
		return fmt.Errorf("install firewall rules in netns: %w", err)
	}
	return nil
}

// installDefaultRoute adds `default via gateway` inside the namespace.
// Both veth ends must already be UP, otherwise the kernel rejects the
// route with ENETUNREACH because the gateway isn't yet reachable.
func installDefaultRoute(nsFd netns.NsHandle, gateway netip.Addr) error {
	h, err := netlink.NewHandleAt(nsFd)
	if err != nil {
		return fmt.Errorf("open ns netlink handle: %w", err)
	}
	defer h.Close()

	gw := net.IP(gateway.AsSlice())
	if err := h.RouteAdd(&netlink.Route{Gw: gw}); err != nil {
		return fmt.Errorf("add default route via %s: %w", gw, err)
	}
	return nil
}

// buildNetlinkAddr adapts a netip.Addr + prefix length into the netlink
// library's wire format.
func buildNetlinkAddr(addr netip.Addr, prefixLen int) *netlink.Addr {
	slice := addr.AsSlice()
	return &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   net.IP(slice),
			Mask: net.CIDRMask(prefixLen, len(slice)*8),
		},
	}
}

// isLinkNotFound reports whether err is the netlink library's signal that
// a link does not exist. Used to keep Deprovision idempotent.
func isLinkNotFound(err error) bool {
	var nf netlink.LinkNotFoundError
	if errors.As(err, &nf) {
		return true
	}
	// Fallback: some kernels/paths return bare ENODEV.
	return errors.Is(err, unix.ENODEV) || errors.Is(err, os.ErrNotExist)
}
