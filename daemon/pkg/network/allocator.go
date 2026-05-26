package network

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// Naming and addressing policy for per-VM networks.
//
// The design is deterministic and stateless: every output is a pure
// function of the caller-supplied index. That keeps the driver free of a
// registry and makes failures recoverable just by replaying with the same
// index.
const (
	namespacePrefix   = "vm-ns"     // namespace: vm-ns<index>
	tapDeviceName     = "vm-tap0"   // fixed name inside every namespace
	namespaceVethName = "vm-veth0"  // namespace-side veth end
	hostVethPrefix    = "host-veth" // host-side veth: host-veth<index>

	// vethSubnetBits is the prefix length of each per-VM veth subnet. /30
	// gives exactly the two addresses we need (host end, ns end) and bounds
	// wasted space to 2 addresses (network, broadcast) per VM.
	vethSubnetBits = 30
	vethSubnetSize = 1 << (32 - vethSubnetBits) // 4

	// tapPrefixBits matches vethSubnetBits for symmetry — each namespace's
	// TAP/guest pair also needs exactly two addresses.
	tapPrefixBits = 30

	// maxIndex bounds the allocator. 10.0.0.0/16 holds 65536 addresses and
	// each /30 consumes 4, so 16384 VMs is the ceiling before we'd cross
	// out of the base network.
	maxIndex = (1 << 16) / vethSubnetSize // 16384
)

// vethBaseNetwork is the first address of the veth allocator pool.
var vethBaseNetwork = netip.MustParseAddr("10.0.0.0")

// VMSubnet returns the IPv4 CIDR that covers every per-VM veth subnet
// the allocator can hand out. Stable across processes — the firewall
// package uses it as the source-match for the host-wide MASQUERADE
// rule, so it must encompass the entire allocator pool regardless of
// how many indexes are currently live.
func VMSubnet() netip.Prefix {
	// vethBaseNetwork is 10.0.0.0; the allocator consumes
	// maxIndex * vethSubnetSize = 16384 * 4 = 65536 addresses,
	// exactly one /16.
	return netip.PrefixFrom(vethBaseNetwork, 16)
}

// GuestSubnet returns the CIDR every guest's virtio-net interface
// occupies inside its namespace. Identical for all VMs (isolation
// comes from the namespace boundary, not addressing). Used as the
// source-match for the per-namespace MASQUERADE rule.
func GuestSubnet() netip.Prefix {
	// tapIP is 172.16.0.1/30 by design — the /30 holds tapIP and
	// guestIP exclusively.
	return netip.PrefixFrom(
		netip.AddrFrom4([4]byte{172, 16, 0, 0}),
		tapPrefixBits,
	)
}

// Every namespace uses the same TAP subnet. Isolation is enforced by the
// namespace boundary, so identical addressing across VMs is safe and lets
// the guest userspace hard-code the gateway.
var (
	tapIP   = netip.MustParseAddr("172.16.0.1")
	guestIP = netip.MustParseAddr("172.16.0.2")
)

func validateIndex(index int) error {
	if index < 0 || index >= maxIndex {
		return fmt.Errorf("network index %d out of range [0,%d)", index, maxIndex)
	}
	return nil
}

func namespaceName(index int) string {
	return fmt.Sprintf("%s%d", namespacePrefix, index)
}

func hostVethName(index int) string {
	return fmt.Sprintf("%s%d", hostVethPrefix, index)
}

// vethSubnetFor returns the host IP, namespace IP, and /30 prefix for a
// given index. IPv4 arithmetic is done on the uint32 form rather than via
// string concatenation so the result stays correct when the third octet
// rolls over (which happens at index 64: 64*4 = 256).
func vethSubnetFor(index int) (host, namespace netip.Addr, prefix netip.Prefix) {
	baseBytes := vethBaseNetwork.As4()
	base := binary.BigEndian.Uint32(baseBytes[:])
	network := base + uint32(index)*uint32(vethSubnetSize)

	var netB, hostB, nsB [4]byte
	binary.BigEndian.PutUint32(netB[:], network)
	binary.BigEndian.PutUint32(hostB[:], network+1)
	binary.BigEndian.PutUint32(nsB[:], network+2)

	host = netip.AddrFrom4(hostB)
	namespace = netip.AddrFrom4(nsB)
	prefix = netip.PrefixFrom(netip.AddrFrom4(netB), vethSubnetBits)
	return
}

// buildHandle returns the metadata for index without issuing any syscall.
// It's the canonical source of truth for the names and IPs a provisioned
// namespace will expose.
func buildHandle(index int) NamespaceHandle {
	hostIP, nsIP, _ := vethSubnetFor(index)
	return NamespaceHandle{
		Index:           index,
		NamespaceName:   namespaceName(index),
		TapDeviceName:   tapDeviceName,
		TapIP:           tapIP,
		GuestIP:         guestIP,
		HostVethName:    hostVethName(index),
		HostVethIP:      hostIP,
		NamespaceVethIP: nsIP,
	}
}

// NamespaceVethIPFor returns the namespace-side veth IP for the
// given index. Pure function — no kernel state is touched. Used by
// the reconciler's delete path, where only the index is known and
// the NamespaceHandle is no longer available.
func NamespaceVethIPFor(index int) (netip.Addr, error) {
	if err := validateIndex(index); err != nil {
		return netip.Addr{}, err
	}
	_, nsIP, _ := vethSubnetFor(index)
	return nsIP, nil
}
