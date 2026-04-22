package network

import "net/netip"

// Driver provisions isolated per-VM network topologies. Each Provision call
// yields a fully-configured network namespace: a TAP device for Firecracker
// to attach to, a veth pair back to the host, and a default route inside the
// namespace pointing at the host side.
type Driver interface {
	// Provision sets up the network infrastructure for the VM identified by
	// index. The caller must ensure index is unique on this host. On any
	// error Provision rolls back its partial state before returning, so the
	// zero-value handle never corresponds to live kernel state.
	Provision(index int) (NamespaceHandle, error)

	// Deprovision tears down everything Provision(index) created. It is
	// idempotent — unknown or already-removed indexes are not an error.
	Deprovision(index int) error
}

// NamespaceHandle is the read-only metadata returned by [Driver.Provision].
// It carries the names and IP addresses the caller needs to wire a
// Firecracker guest into the provisioned namespace. It has no behavior;
// Deprovisioning is done via [Driver.Deprovision] using the index.
type NamespaceHandle struct {
	// Index is the per-host unique identifier supplied to Provision.
	Index int

	// NamespaceName is the linux network namespace name, e.g. "vm-ns7".
	NamespaceName string

	// TapDeviceName is the name of the TAP interface inside the namespace.
	TapDeviceName string

	// TapIP is the gateway the guest should use as its default route.
	TapIP netip.Addr

	// GuestIP is the IP the guest kernel should assign to its virtio-net
	// interface.
	GuestIP netip.Addr

	// HostVethName is the host-side end of the veth pair.
	HostVethName string

	// HostVethIP is the host-side veth address. The namespace's default
	// route points here.
	HostVethIP netip.Addr
}
