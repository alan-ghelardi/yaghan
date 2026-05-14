package network

import (
	"fmt"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// DefaultUpstreamDevice resolves the host interface that the default
// IPv4 route currently points out of. Equivalent to
//
//	ip -j -4 route list default | jq -r '.[0].dev'
//
// but implemented with netlink so the daemon doesn't depend on iproute2
// or jq. The first default route wins — multi-default-route hosts are
// rare in our target environments (single-NIC EC2, single-NIC bare
// metal); operators who need a specific egress device on a multi-route
// host should set [config.Network.UpstreamDevice] explicitly.
func DefaultUpstreamDevice() (string, error) {
	routes, err := netlink.RouteListFiltered(unix.AF_INET, &netlink.Route{Dst: nil}, netlink.RT_FILTER_DST)
	if err != nil {
		return "", fmt.Errorf("list default routes: %w", err)
	}
	for _, r := range routes {
		// Filter the actual default route: Dst==nil already restricted
		// by RT_FILTER_DST, but only routes with a non-nil Gw and a
		// resolvable LinkIndex are usable for egress.
		if r.Gw == nil || r.LinkIndex == 0 {
			continue
		}
		link, err := netlink.LinkByIndex(r.LinkIndex)
		if err != nil {
			return "", fmt.Errorf("resolve link %d for default route: %w", r.LinkIndex, err)
		}
		return link.Attrs().Name, nil
	}
	return "", fmt.Errorf("no IPv4 default route found")
}
