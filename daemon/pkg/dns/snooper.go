package dns

import (
	"net/netip"
	"time"

	"github.com/alan-ghelardi/yaghan/daemon/pkg/network/firewall"
)

// ipsetSnooper is the production [Snooper] that translates A/AAAA
// observations into per-sandbox ipset entries inside the sandbox's
// network namespace. The firewall package already knows how to talk
// to the kernel; this adapter just plumbs the policy → (nsName,
// index) coordinates.
type ipsetSnooper struct {
	sets firewall.IPSet
}

// NewIPSetSnooper returns a [Snooper] backed by the supplied IPSet
// client. The IPSet client is shared with the firewall layer so the
// set lifecycle (create on Provision, destroyed-with-namespace on
// Deprovision) and the entry lifecycle (added per resolution)
// converge through the same backend.
func NewIPSetSnooper(sets firewall.IPSet) Snooper {
	return &ipsetSnooper{sets: sets}
}

// Note implements [Snooper].
//
// The implementation is best-effort by design: returning an error
// signals "the firewall did not learn this IP". The handler logs the
// failure but still sends the DNS answer to the guest, on the
// principle that the next query will retry. A persistent failure
// (e.g. ipset binary missing) surfaces in logs and metrics rather
// than blocking the DNS path.
func (s *ipsetSnooper) Note(policy SandboxDNSPolicy, ip netip.Addr, ttl time.Duration) error {
	return s.sets.AddSandboxEntry(policy.NamespaceName, policy.Index, ip, ttl)
}
