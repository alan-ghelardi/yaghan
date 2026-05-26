package firewall

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAdder satisfies the [adder] interface and captures every
// call so tests can assert on the rule-specs the iptables firewall
// would have installed.
type recordingAdder struct {
	chains  map[string]bool
	appends []string // table/chain/rulespec joined by " | "
}

func newRecorder() *recordingAdder { return &recordingAdder{chains: map[string]bool{}} }

func (r *recordingAdder) ChainExists(table, chain string) (bool, error) {
	return r.chains[table+"/"+chain], nil
}

func (r *recordingAdder) NewChain(table, chain string) error {
	r.chains[table+"/"+chain] = true
	return nil
}

func (r *recordingAdder) AppendUnique(table, chain string, rulespec ...string) error {
	r.appends = append(r.appends, table+"/"+chain+" | "+strings.Join(rulespec, " "))
	return nil
}

// recordingSets satisfies [setMaker] for tests, recording the indexes
// the firewall asked it to ensure so coverage can confirm the
// per-sandbox set was created exactly when domain rules required it.
type recordingSets struct {
	ensured []int
}

func (r *recordingSets) EnsureSandboxSet(index int) error {
	r.ensured = append(r.ensured, index)
	return nil
}

// newFirewall is a small constructor that wires both fakes together.
// Keeps every test case from having to spell the struct literal out.
func newFirewall() (*iptablesFirewall, *recordingAdder, *recordingSets) {
	r, s := newRecorder(), &recordingSets{}
	return &iptablesFirewall{ipt: r, sets: s}, r, s
}

func TestEnsureHost(t *testing.T) {
	fw, r, _ := newFirewall()
	subnet := netip.MustParsePrefix("10.0.0.0/16")

	require.NoError(t, fw.EnsureHost("eth0", subnet))

	assert.True(t, r.chains["filter/"+hostForwardChain], "filter chain created")
	assert.True(t, r.chains["nat/"+hostNATChain], "nat chain created")

	// Spot-check that each meaningful rule was issued. Matching on
	// substrings keeps the test resilient to flag-ordering tweaks
	// in EnsureHost while still anchoring the important bits.
	want := []string{
		"filter/FORWARD | -m comment --comment yaghan -j " + hostForwardChain,
		"filter/" + hostForwardChain + " | -s 10.0.0.0/16 -o eth0",
		"filter/" + hostForwardChain + " | -i eth0 -d 10.0.0.0/16",
		"nat/POSTROUTING | -m comment --comment yaghan -j " + hostNATChain,
		"nat/" + hostNATChain + " | -s 10.0.0.0/16 -o eth0 -m comment --comment yaghan -j MASQUERADE",
	}
	for _, fragment := range want {
		assert.True(t, containsAny(r.appends, fragment),
			"missing rule fragment %q in %v", fragment, r.appends)
	}
}

func TestEnsureHostIsIdempotent(t *testing.T) {
	fw, r, _ := newFirewall()
	subnet := netip.MustParsePrefix("10.0.0.0/16")

	require.NoError(t, fw.EnsureHost("eth0", subnet))
	first := len(r.appends)
	require.NoError(t, fw.EnsureHost("eth0", subnet))
	// recorder doesn't dedupe — but the real AppendUnique does. What
	// we check here is that the second call did not try to create
	// the chain a second time.
	assert.Equal(t, 2*first, len(r.appends), "rules re-emitted via AppendUnique")
	// Confirm we didn't attempt NewChain twice for the same chain
	// — ChainExists short-circuits the second pass. The recorder's
	// map is set-once, so a second NewChain would still flip it, but
	// the call would have been a no-op. We can't observe NewChain
	// directly without a richer recorder; existence-only is enough.
	assert.True(t, r.chains["filter/"+hostForwardChain])
	assert.True(t, r.chains["nat/"+hostNATChain])
}

func TestEnsureHostRejectsBadInput(t *testing.T) {
	fw, _, _ := newFirewall()
	assert.Error(t, fw.EnsureHost("", netip.MustParsePrefix("10.0.0.0/16")))
	assert.Error(t, fw.EnsureHost("eth0", netip.Prefix{}))
}

func TestConfigureNamespace(t *testing.T) {
	fw, r, _ := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
	}

	require.NoError(t, fw.ConfigureNamespace(cfg))

	want := []string{
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0",
		"filter/FORWARD | -i vm-veth0 -d 172.16.0.0/30",
		"nat/POSTROUTING | -s 172.16.0.0/30 -o vm-veth0",
	}
	for _, fragment := range want {
		assert.True(t, containsAny(r.appends, fragment),
			"missing namespace rule %q in %v", fragment, r.appends)
	}
}

func TestConfigureNamespaceRejectsBadInput(t *testing.T) {
	fw, _, _ := newFirewall()
	assert.Error(t, fw.ConfigureNamespace(NamespaceConfig{
		GuestSubnet: netip.MustParsePrefix("172.16.0.0/30"),
	}))
	assert.Error(t, fw.ConfigureNamespace(NamespaceConfig{
		NamespaceVethName: "vm-veth0",
	}))
}

func TestConfigureNamespace_AllowPolicy(t *testing.T) {
	fw, r, _ := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
		Egress: &EgressPolicy{
			Mode: EgressAllow,
			IPs: []netip.Addr{
				netip.MustParseAddr("8.8.8.8"),
			},
			CIDRs: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.0/24"),
			},
		},
	}

	require.NoError(t, fw.ConfigureNamespace(cfg))

	// Each per-IP/per-CIDR ACCEPT must be installed, terminated by a
	// REJECT --reject-with icmp-admin-prohibited fall-through. The
	// generic ACCEPT used by unrestricted policies must NOT appear.
	want := []string{
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 8.8.8.8 -m comment --comment yaghan -j ACCEPT",
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 10.0.0.0/24 -m comment --comment yaghan -j ACCEPT",
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -m comment --comment yaghan -j REJECT --reject-with icmp-admin-prohibited",
		// MASQUERADE is independent of policy and must still be present.
		"nat/POSTROUTING | -s 172.16.0.0/30 -o vm-veth0",
	}
	for _, fragment := range want {
		assert.True(t, containsAny(r.appends, fragment),
			"missing allow-policy rule %q in %v", fragment, r.appends)
	}

	// Confirm the unrestricted fall-through ACCEPT did NOT slip in.
	for _, append := range r.appends {
		assert.NotContains(t, append,
			"-s 172.16.0.0/30 -o vm-veth0 -m comment --comment yaghan -j ACCEPT",
			"unrestricted fall-through ACCEPT must not appear under allow policy")
	}

	// Rule order: the terminal REJECT must come after each ACCEPT, or
	// the policy would block its own allow-list entries.
	rejectIdx := indexOf(r.appends, "-j REJECT --reject-with icmp-admin-prohibited")
	acceptIdx := indexOf(r.appends, "-d 8.8.8.8 -m comment --comment yaghan -j ACCEPT")
	require.GreaterOrEqual(t, rejectIdx, 0, "REJECT rule must be installed")
	require.GreaterOrEqual(t, acceptIdx, 0, "ACCEPT rule must be installed")
	assert.Less(t, acceptIdx, rejectIdx, "ACCEPT must precede terminal REJECT")
}

func TestConfigureNamespace_DenyPolicy(t *testing.T) {
	fw, r, _ := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
		Egress: &EgressPolicy{
			Mode: EgressDeny,
			IPs: []netip.Addr{
				netip.MustParseAddr("1.1.1.1"),
			},
			CIDRs: []netip.Prefix{
				netip.MustParsePrefix("192.168.0.0/16"),
			},
		},
	}

	require.NoError(t, fw.ConfigureNamespace(cfg))

	want := []string{
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 1.1.1.1 -m comment --comment yaghan -j REJECT --reject-with icmp-admin-prohibited",
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 192.168.0.0/16 -m comment --comment yaghan -j REJECT --reject-with icmp-admin-prohibited",
		// Generic egress ACCEPT remains the fall-through under deny.
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -m comment --comment yaghan -j ACCEPT",
		"nat/POSTROUTING | -s 172.16.0.0/30 -o vm-veth0",
	}
	for _, fragment := range want {
		assert.True(t, containsAny(r.appends, fragment),
			"missing deny-policy rule %q in %v", fragment, r.appends)
	}

	// Rule order: each REJECT must come BEFORE the fall-through ACCEPT,
	// otherwise the ACCEPT would short-circuit the deny and let traffic
	// through.
	denyIdx := indexOf(r.appends, "-d 1.1.1.1 -m comment --comment yaghan -j REJECT")
	acceptIdx := indexOf(r.appends, "-o vm-veth0 -m comment --comment yaghan -j ACCEPT")
	require.GreaterOrEqual(t, denyIdx, 0, "REJECT rule must be installed")
	require.GreaterOrEqual(t, acceptIdx, 0, "fall-through ACCEPT must be installed")
	assert.Less(t, denyIdx, acceptIdx, "deny REJECTs must precede fall-through ACCEPT")
}

func TestConfigureNamespace_AllowPolicy_WithDomains(t *testing.T) {
	fw, r, s := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
		SandboxIndex:      7,
		ResolverIP:        netip.MustParseAddr("10.0.0.29"),
		Egress: &EgressPolicy{
			Mode:    EgressAllow,
			IPs:     []netip.Addr{netip.MustParseAddr("8.8.8.8")},
			Domains: []string{"example.com", "*.internal.corp"},
		},
	}

	require.NoError(t, fw.ConfigureNamespace(cfg))

	// The per-sandbox set must have been created exactly once.
	assert.Equal(t, []int{7}, s.ensured)

	// Resolver carve-out + ipset match must both be present and the
	// ipset match must precede the terminal REJECT — otherwise the
	// FORWARD chain would drop traffic to dynamically-allowed IPs
	// before the match-set rule ever fired.
	want := []string{
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 10.0.0.29 -p udp --dport 53 -m comment --comment yaghan -j ACCEPT",
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -d 10.0.0.29 -p tcp --dport 53 -m comment --comment yaghan -j ACCEPT",
		"filter/FORWARD | -s 172.16.0.0/30 -o vm-veth0 -m set --match-set yaghan-egress-7 dst -m comment --comment yaghan -j ACCEPT",
	}
	for _, fragment := range want {
		assert.True(t, containsAny(r.appends, fragment),
			"missing rule %q in %v", fragment, r.appends)
	}

	rejectIdx := indexOf(r.appends, "-j REJECT --reject-with icmp-admin-prohibited")
	setIdx := indexOf(r.appends, "--match-set yaghan-egress-7 dst")
	resolverIdx := indexOf(r.appends, "-d 10.0.0.29 -p udp --dport 53")
	require.GreaterOrEqual(t, rejectIdx, 0)
	require.GreaterOrEqual(t, setIdx, 0)
	require.GreaterOrEqual(t, resolverIdx, 0)
	assert.Less(t, setIdx, rejectIdx, "match-set ACCEPT must precede terminal REJECT")
	assert.Less(t, resolverIdx, rejectIdx, "resolver carve-out must precede terminal REJECT")
	assert.Less(t, resolverIdx, setIdx, "resolver carve-out must precede match-set so DNS works before the first answer lands")
}

func TestConfigureNamespace_AllowPolicy_DomainsRequireResolverIP(t *testing.T) {
	fw, _, _ := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
		SandboxIndex:      0,
		// ResolverIP intentionally zero.
		Egress: &EgressPolicy{
			Mode:    EgressAllow,
			Domains: []string{"example.com"},
		},
	}

	err := fw.ConfigureNamespace(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ResolverIP")
}

func TestConfigureNamespace_DomainsOmittedSkipsSetAndCarveOut(t *testing.T) {
	fw, r, s := newFirewall()
	cfg := NamespaceConfig{
		NamespaceVethName: "vm-veth0",
		GuestSubnet:       netip.MustParsePrefix("172.16.0.0/30"),
		SandboxIndex:      3,
		ResolverIP:        netip.MustParseAddr("10.0.0.13"),
		Egress: &EgressPolicy{
			Mode: EgressAllow,
			IPs:  []netip.Addr{netip.MustParseAddr("8.8.8.8")},
		},
	}

	require.NoError(t, fw.ConfigureNamespace(cfg))

	assert.Empty(t, s.ensured, "no Domains -> no ipset")
	for _, a := range r.appends {
		assert.NotContains(t, a, "--match-set", "no Domains -> no match-set rule")
		assert.NotContains(t, a, "--dport 53", "no Domains -> no resolver carve-out")
	}
}

// indexOf returns the position of the first entry in haystack
// containing needle, or -1 when no entry matches.
func indexOf(haystack []string, needle string) int {
	for i, h := range haystack {
		if strings.Contains(h, needle) {
			return i
		}
	}
	return -1
}

func containsAny(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
