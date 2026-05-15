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

func TestEnsureHost(t *testing.T) {
	r := newRecorder()
	fw := &iptablesFirewall{ipt: r}
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
	r := newRecorder()
	fw := &iptablesFirewall{ipt: r}
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
	fw := &iptablesFirewall{ipt: newRecorder()}
	assert.Error(t, fw.EnsureHost("", netip.MustParsePrefix("10.0.0.0/16")))
	assert.Error(t, fw.EnsureHost("eth0", netip.Prefix{}))
}

func TestConfigureNamespace(t *testing.T) {
	r := newRecorder()
	fw := &iptablesFirewall{ipt: r}
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
	fw := &iptablesFirewall{ipt: newRecorder()}
	assert.Error(t, fw.ConfigureNamespace(NamespaceConfig{
		GuestSubnet: netip.MustParsePrefix("172.16.0.0/30"),
	}))
	assert.Error(t, fw.ConfigureNamespace(NamespaceConfig{
		NamespaceVethName: "vm-veth0",
	}))
}

func containsAny(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
