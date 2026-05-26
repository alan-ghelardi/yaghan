package dns

import (
	"net/netip"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyStore_UpsertLookupRemove(t *testing.T) {
	s := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.2")

	_, ok := s.Lookup(src)
	assert.False(t, ok)

	matchers, err := CompileMatchers([]string{"example.com"})
	require.NoError(t, err)
	s.Upsert(src, SandboxDNSPolicy{
		Index:         0,
		NamespaceName: "vm-ns0",
		Mode:          ModeAllow,
		Matchers:      matchers,
	})

	got, ok := s.Lookup(src)
	require.True(t, ok)
	assert.Equal(t, "vm-ns0", got.NamespaceName)
	assert.Equal(t, ModeAllow, got.Mode)
	assert.Equal(t, 1, s.Len())

	// Upsert replaces — Len stays 1, Mode updates.
	s.Upsert(src, SandboxDNSPolicy{Index: 0, NamespaceName: "vm-ns0", Mode: ModeDeny})
	got, _ = s.Lookup(src)
	assert.Equal(t, ModeDeny, got.Mode)
	assert.Equal(t, 1, s.Len())

	s.Remove(src)
	_, ok = s.Lookup(src)
	assert.False(t, ok)
	assert.Equal(t, 0, s.Len())

	// Remove on absent key is a no-op.
	s.Remove(src)
}

func TestPolicyStore_ConcurrentAccess(_ *testing.T) {
	// Stress the RWMutex contract: many readers and writers
	// hammering different keys must not race or panic.
	s := NewPolicyStore()
	const writers = 8
	const readers = 16
	const ops = 200

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := netip.AddrFrom4([4]byte{10, 0, 0, byte(id)})
			for i := 0; i < ops; i++ {
				s.Upsert(ip, SandboxDNSPolicy{Index: id})
				if i%4 == 0 {
					s.Remove(ip)
				}
			}
		}(w)
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := netip.AddrFrom4([4]byte{10, 0, 0, byte(id % writers)})
			for i := 0; i < ops; i++ {
				_, _ = s.Lookup(ip)
				_ = s.Len()
			}
		}(r)
	}
	wg.Wait()
}
