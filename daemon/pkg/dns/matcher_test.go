package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileMatcher_Exact(t *testing.T) {
	m, err := CompileMatcher("Example.COM")
	require.NoError(t, err)

	// Canonical form (trailing dot, lowercase) is the input the
	// handler will receive from miekg/dns.
	assert.True(t, m.Match("example.com."))
	assert.True(t, m.Match("EXAMPLE.com."))
	assert.False(t, m.Match("foo.example.com."))
	assert.False(t, m.Match("examplecom."))
	assert.False(t, m.Match("notexample.com."))
}

func TestCompileMatcher_Wildcard(t *testing.T) {
	m, err := CompileMatcher("*.example.com")
	require.NoError(t, err)

	// Matches the bare domain and arbitrary subdomains. The depth
	// is unbounded by design — see the matcher's doc comment.
	for _, name := range []string{
		"example.com.",
		"foo.example.com.",
		"a.b.example.com.",
		"deep.deeper.deepest.example.com.",
	} {
		assert.True(t, m.Match(name), "%s should match", name)
	}

	// Negative cases that look superficially close.
	for _, name := range []string{
		"examplecom.",
		"evilexample.com.",
		"example.com.evil.",
		"com.",
	} {
		assert.False(t, m.Match(name), "%s should NOT match", name)
	}
}

func TestCompileMatcher_RejectsBadPatterns(t *testing.T) {
	cases := []string{
		"",
		"*.",
		"foo.*",
		"foo.*.bar",
		"foo*.bar",
		"**.example.com",
	}
	for _, p := range cases {
		_, err := CompileMatcher(p)
		assert.Error(t, err, "pattern %q must be rejected", p)
	}
}

func TestMatchAny(t *testing.T) {
	matchers, err := CompileMatchers([]string{"example.com", "*.internal.corp"})
	require.NoError(t, err)

	assert.True(t, MatchAny(matchers, "example.com."))
	assert.True(t, MatchAny(matchers, "vault.internal.corp."))
	assert.True(t, MatchAny(matchers, "deep.a.internal.corp."))
	assert.False(t, MatchAny(matchers, "evil.com."))
	assert.False(t, MatchAny(matchers, "foo.example.com.")) // exact match is strict
}

func TestCompileMatchers_Empty(t *testing.T) {
	out, err := CompileMatchers(nil)
	require.NoError(t, err)
	assert.Empty(t, out)
}
