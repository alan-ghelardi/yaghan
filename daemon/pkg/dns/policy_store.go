package dns

import (
	"net/netip"
	"sync"
)

// EgressMode classifies a sandbox's domain_names policy verdict.
// Mirrors firewall.EgressMode but is duplicated here to keep this
// package importable from the dns layer without dragging in the
// firewall types.
type EgressMode int

const (
	// ModeNone signals "no domain_names policy registered for this
	// sandbox" — the handler falls back to forwarding upstream as a
	// plain recursive resolver.
	ModeNone EgressMode = iota

	// ModeAllow allows queries that match Matchers and refuses
	// everything else.
	ModeAllow

	// ModeDeny refuses queries that match Matchers and forwards
	// everything else.
	ModeDeny
)

// SandboxDNSPolicy is the per-sandbox state the handler consults on
// every query. It is registered by the reconciler at sandbox
// Provision and removed at Deprovision.
type SandboxDNSPolicy struct {
	// Index identifies the sandbox in the daemon. Used to compose
	// the per-sandbox ipset name and to log meaningful diagnostics.
	Index int

	// NamespaceName is the linux network namespace the sandbox lives
	// in (e.g. "vm-ns7"). The handler shells into it via
	// `ip netns exec` when populating the per-sandbox ipset.
	NamespaceName string

	// Mode is the verdict applied to matching names. See [EgressMode].
	Mode EgressMode

	// Matchers are the compiled domain patterns the policy referenced.
	// Empty under ModeNone.
	Matchers []DomainMatcher
}

// PolicyStore holds the live SandboxDNSPolicy entries the handler
// consults on each query. Lookups are read-heavy and a RWMutex keeps
// the slow path (Upsert/Remove on sandbox lifecycle events) from
// blocking the hot path.
type PolicyStore struct {
	mu sync.RWMutex
	// bySrc is keyed by the source IP the daemon sees on incoming
	// queries — the sandbox's namespace-side veth address, after the
	// per-namespace MASQUERADE.
	bySrc map[netip.Addr]SandboxDNSPolicy
}

// NewPolicyStore returns an empty store ready for use.
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{bySrc: map[netip.Addr]SandboxDNSPolicy{}}
}

// Upsert registers or replaces the policy for srcIP. Idempotent —
// the reconciler may call it multiple times across reconciles for
// the same sandbox.
func (s *PolicyStore) Upsert(srcIP netip.Addr, policy SandboxDNSPolicy) {
	s.mu.Lock()
	s.bySrc[srcIP] = policy
	s.mu.Unlock()
}

// Remove drops the entry for srcIP. Safe to call on absent keys.
func (s *PolicyStore) Remove(srcIP netip.Addr) {
	s.mu.Lock()
	delete(s.bySrc, srcIP)
	s.mu.Unlock()
}

// Lookup returns the policy registered for srcIP. The second result
// is false when no entry exists — the handler treats that as
// "unknown source" and refuses the query.
func (s *PolicyStore) Lookup(srcIP netip.Addr) (SandboxDNSPolicy, bool) {
	s.mu.RLock()
	p, ok := s.bySrc[srcIP]
	s.mu.RUnlock()
	return p, ok
}

// Len reports how many entries the store holds. Cheap; intended for
// readiness probes and metrics.
func (s *PolicyStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bySrc)
}
