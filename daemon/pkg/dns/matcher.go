// Package dns provides the daemon-embedded DNS responder that enforces
// per-sandbox domain_names egress rules.
//
// The responder owns three pieces of in-process state:
//
//   - a [PolicyStore] keyed by the source IP the daemon will see on
//     each query (the sandbox's namespace-side veth address). The
//     reconciler populates the store at sandbox provision time and
//     evicts on deprovision;
//   - a set of compiled domain matchers attached to each store entry
//     that implement the wildcard semantics documented on the proto;
//   - a [Handler] (miekg/dns) that classifies each query, forwards
//     allow-matched queries upstream, and snoops the answer to grow
//     the per-sandbox ipset the firewall ACCEPTs from.
//
// The handler does not consult the firewall's iptables state or the
// proto's `ip_addresses`/`cidr_blocks` lists — those are enforced
// statically by the firewall at boot. domain_names enforcement is
// dynamic and lives entirely in this package.
package dns

import (
	"fmt"
	"strings"
)

// DomainMatcher matches DNS query names against a single
// domain_names entry. Implementations must accept names in DNS
// canonical form (trailing dot, mixed case) and handle them as
// case-insensitive.
type DomainMatcher interface {
	// Match reports whether name (DNS canonical form, with trailing
	// dot) is covered by this matcher.
	Match(name string) bool

	// String returns the pattern this matcher was compiled from.
	// Used in logs.
	String() string
}

// CompileMatcher returns a [DomainMatcher] for pattern. Accepts the
// same syntax the proto's CEL validator enforces: a bare hostname
// (`example.net`) or a leading-wildcard label (`*.example.net`). The
// wildcard form matches the bare domain itself plus any subdomain at
// any depth — `*.example.net` matches `example.net`, `foo.example.net`,
// and `a.b.example.net`.
//
// Returns an error for empty input or shapes the CEL validator
// rejects (no embedded `*`, no trailing `*`, etc.); the api-server
// rejects these before they reach the daemon, so a failure here
// indicates the proto wire was tampered with or validation drifted.
func CompileMatcher(pattern string) (DomainMatcher, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.ToLower(pattern[2:])
		if suffix == "" || strings.Contains(suffix, "*") {
			return nil, fmt.Errorf("invalid wildcard pattern %q", pattern)
		}
		return wildcardMatcher{pattern: pattern, suffix: suffix}, nil
	}
	if strings.Contains(pattern, "*") {
		return nil, fmt.Errorf("wildcards are only valid as a leading '*.' label: %q", pattern)
	}
	return exactMatcher{pattern: strings.ToLower(pattern)}, nil
}

// canonicalName strips the trailing dot the DNS wire format uses and
// lowercases the result. Returns the empty string for the root zone
// ("."), which no policy entry can legitimately match.
func canonicalName(name string) string {
	name = strings.TrimSuffix(name, ".")
	return strings.ToLower(name)
}

type exactMatcher struct {
	pattern string // already lowercased
}

func (m exactMatcher) Match(name string) bool {
	return canonicalName(name) == m.pattern
}

func (m exactMatcher) String() string { return m.pattern }

type wildcardMatcher struct {
	pattern string // original "*.example.com" (for logs)
	suffix  string // lowercased "example.com"
}

// Match returns true when name equals the suffix or ends with
// ".suffix". The latter handles arbitrarily deep subdomains
// (`a.b.example.com`) — the matcher is intentionally greedy because
// DNS doesn't have a notion of "single-label" wildcards and a
// shallower match would surprise users who expect `*.example.com` to
// cover everything under example.com.
func (m wildcardMatcher) Match(name string) bool {
	canonical := canonicalName(name)
	if canonical == m.suffix {
		return true
	}
	return strings.HasSuffix(canonical, "."+m.suffix)
}

func (m wildcardMatcher) String() string { return m.pattern }

// CompileMatchers compiles a slice of patterns, returning the first
// compile error encountered. Callers that need partial results
// (e.g. tolerant boot from a corrupt persisted policy) should call
// [CompileMatcher] per entry instead.
func CompileMatchers(patterns []string) ([]DomainMatcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	out := make([]DomainMatcher, 0, len(patterns))
	for _, p := range patterns {
		m, err := CompileMatcher(p)
		if err != nil {
			return nil, fmt.Errorf("compile %q: %w", p, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// MatchAny reports whether any matcher in matchers covers name.
func MatchAny(matchers []DomainMatcher, name string) bool {
	for _, m := range matchers {
		if m.Match(name) {
			return true
		}
	}
	return false
}
