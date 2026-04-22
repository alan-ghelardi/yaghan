package network

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIndex(t *testing.T) {
	cases := []struct {
		name    string
		index   int
		wantErr bool
	}{
		{"negative", -1, true},
		{"zero", 0, false},
		{"one", 1, false},
		{"last valid", maxIndex - 1, false},
		{"too large", maxIndex, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIndex(tc.index)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNamespaceName(t *testing.T) {
	cases := []struct {
		index int
		want  string
	}{
		{0, "vm-ns0"},
		{42, "vm-ns42"},
		{16383, "vm-ns16383"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, namespaceName(tc.index))
	}
}

func TestHostVethName(t *testing.T) {
	cases := []struct {
		index int
		want  string
	}{
		{0, "host-veth0"},
		{7, "host-veth7"},
		{16383, "host-veth16383"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, hostVethName(tc.index))
	}
}

func TestVethSubnetFor(t *testing.T) {
	cases := []struct {
		name          string
		index         int
		wantHost      string
		wantNamespace string
		wantPrefix    string
	}{
		{"index 0", 0, "10.0.0.1", "10.0.0.2", "10.0.0.0/30"},
		{"index 1", 1, "10.0.0.5", "10.0.0.6", "10.0.0.4/30"},
		{"index 2", 2, "10.0.0.9", "10.0.0.10", "10.0.0.8/30"},
		// 63*4 = 252 — last /30 before the third octet rolls over.
		{"before rollover", 63, "10.0.0.253", "10.0.0.254", "10.0.0.252/30"},
		// 64*4 = 256 — this index is the one that breaks a naive
		// fmt.Sprintf("10.0.0.%d") allocator. Keep this case.
		{"after rollover", 64, "10.0.1.1", "10.0.1.2", "10.0.1.0/30"},
		{"last valid", maxIndex - 1, "10.0.255.253", "10.0.255.254", "10.0.255.252/30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, ns, pfx := vethSubnetFor(tc.index)
			assert.Equal(t, tc.wantHost, host.String(), "host ip")
			assert.Equal(t, tc.wantNamespace, ns.String(), "namespace ip")
			assert.Equal(t, tc.wantPrefix, pfx.String(), "prefix")
			assert.True(t, pfx.Contains(host), "host %s must lie inside %s", host, pfx)
			assert.True(t, pfx.Contains(ns), "namespace %s must lie inside %s", ns, pfx)
		})
	}
}

// TestVethSubnetsDisjoint checks that adjacent indexes yield fully
// disjoint /30s — the property the allocator exists to guarantee.
func TestVethSubnetsDisjoint(t *testing.T) {
	for i := 0; i < 200; i++ {
		_, ns1, _ := vethSubnetFor(i)
		host2, _, _ := vethSubnetFor(i + 1)
		assert.NotEqual(t, ns1, host2, "indexes %d and %d must not share an address", i, i+1)
	}
}

func TestBuildHandle(t *testing.T) {
	want := NamespaceHandle{
		Index:         5,
		NamespaceName: "vm-ns5",
		TapDeviceName: "vm-tap0",
		TapIP:         netip.MustParseAddr("172.16.0.1"),
		GuestIP:       netip.MustParseAddr("172.16.0.2"),
		HostVethName:  "host-veth5",
		HostVethIP:    netip.MustParseAddr("10.0.0.21"), // 5*4 + 1
	}
	assert.Equal(t, want, buildHandle(5))
}
