package dns

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	miekg "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWriter is a minimal miekg.ResponseWriter that captures the
// response and reports a configurable remote address.
type fakeWriter struct {
	remote net.Addr
	resp   *miekg.Msg
}

func (w *fakeWriter) LocalAddr() net.Addr         { return nil }
func (w *fakeWriter) RemoteAddr() net.Addr        { return w.remote }
func (w *fakeWriter) WriteMsg(m *miekg.Msg) error { w.resp = m; return nil }
func (w *fakeWriter) Write([]byte) (int, error)   { return 0, nil }
func (w *fakeWriter) Close() error                { return nil }
func (w *fakeWriter) TsigStatus() error           { return nil }
func (w *fakeWriter) TsigTimersOnly(bool)         {}
func (w *fakeWriter) Hijack()                     {}

// fakeUpstream returns a canned answer (or error) per query.
type fakeUpstream struct {
	mu       sync.Mutex
	answer   *miekg.Msg
	err      error
	requests []*miekg.Msg
}

func (u *fakeUpstream) Exchange(_ context.Context, req *miekg.Msg) (*miekg.Msg, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, req)
	if u.err != nil {
		return nil, u.err
	}
	resp := u.answer.Copy()
	resp.SetReply(req)
	return resp, nil
}

// fakeSnooper records every (policy, ip, ttl) tuple the handler
// hands it.
type fakeSnooper struct {
	mu   sync.Mutex
	hits []snoopHit
}

type snoopHit struct {
	Index int
	IP    netip.Addr
	TTL   time.Duration
}

func (s *fakeSnooper) Note(p SandboxDNSPolicy, ip netip.Addr, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits = append(s.hits, snoopHit{Index: p.Index, IP: ip, TTL: ttl})
	return nil
}

// newAQuery builds an A-record question. Every test in this file
// uses A; AAAA paths share the same code branch in the handler
// (verified by inspection — both miekg.A and miekg.AAAA are
// extracted by the same switch in snoop), so single-type coverage
// is enough.
func newAQuery(name string) *miekg.Msg {
	req := new(miekg.Msg)
	req.SetQuestion(name, miekg.TypeA)
	return req
}

func newAAnswer(name, ip string, ttl uint32) *miekg.Msg {
	m := new(miekg.Msg)
	rr := &miekg.A{
		Hdr: miekg.RR_Header{Name: name, Rrtype: miekg.TypeA, Class: miekg.ClassINET, Ttl: ttl},
		A:   net.ParseIP(ip).To4(),
	}
	m.Answer = []miekg.RR{rr}
	return m
}

func srcUDP(addr string) net.Addr {
	return &net.UDPAddr{IP: net.ParseIP(addr), Port: 5353}
}

// newTestHandler is the shared fixture builder.
func newTestHandler(t *testing.T, store *PolicyStore, upstream Upstream, snooper Snooper) *Handler {
	t.Helper()
	h, err := NewHandler(HandlerOptions{
		Store:    store,
		Upstream: upstream,
		Snooper:  snooper,
		// Tight TTL bounds so test assertions are deterministic.
		MinEntryTTL: 1 * time.Second,
		MaxEntryTTL: 60 * time.Second,
	})
	require.NoError(t, err)
	return h
}

func TestHandler_RefusesUnregisteredSource(t *testing.T) {
	store := NewPolicyStore()
	up := &fakeUpstream{answer: newAAnswer("example.com.", "1.2.3.4", 60)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.99")}
	h.ServeDNS(w, newAQuery("example.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeRefused, w.resp.Rcode)
	assert.Empty(t, up.requests, "must not forward when source is unknown")
}

func TestHandler_AllowMode_MatchedQueryForwardsAndSnoops(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.2")
	matchers, _ := CompileMatchers([]string{"example.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 0, NamespaceName: "vm-ns0",
		Mode: ModeAllow, Matchers: matchers,
	})
	up := &fakeUpstream{answer: newAAnswer("example.com.", "93.184.216.34", 300)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.2")}
	h.ServeDNS(w, newAQuery("example.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeSuccess, w.resp.Rcode)
	require.Len(t, up.requests, 1)
	require.Len(t, sn.hits, 1)
	assert.Equal(t, "93.184.216.34", sn.hits[0].IP.String())
	// 300s is inside our [1s, 60s] test window → clamped to max.
	assert.Equal(t, 60*time.Second, sn.hits[0].TTL)
}

func TestHandler_AllowMode_UnmatchedQueryRefused(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.2")
	matchers, _ := CompileMatchers([]string{"example.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 0, NamespaceName: "vm-ns0",
		Mode: ModeAllow, Matchers: matchers,
	})
	up := &fakeUpstream{answer: newAAnswer("evil.com.", "6.6.6.6", 60)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.2")}
	h.ServeDNS(w, newAQuery("evil.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeRefused, w.resp.Rcode)
	assert.Empty(t, up.requests, "refused queries must not be forwarded")
	assert.Empty(t, sn.hits, "refused queries must not be snooped")
}

func TestHandler_DenyMode_MatchedQueryRefused(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.6")
	matchers, _ := CompileMatchers([]string{"*.evil.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 1, NamespaceName: "vm-ns1",
		Mode: ModeDeny, Matchers: matchers,
	})
	up := &fakeUpstream{answer: newAAnswer("c2.evil.com.", "6.6.6.6", 60)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.6")}
	h.ServeDNS(w, newAQuery("c2.evil.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeRefused, w.resp.Rcode)
	assert.Empty(t, up.requests)
	assert.Empty(t, sn.hits, "deny mode never snoops")
}

func TestHandler_DenyMode_UnmatchedQueryForwardsNoSnoop(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.6")
	matchers, _ := CompileMatchers([]string{"*.evil.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 1, NamespaceName: "vm-ns1",
		Mode: ModeDeny, Matchers: matchers,
	})
	up := &fakeUpstream{answer: newAAnswer("example.com.", "1.1.1.1", 60)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.6")}
	h.ServeDNS(w, newAQuery("example.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeSuccess, w.resp.Rcode)
	require.Len(t, up.requests, 1)
	assert.Empty(t, sn.hits, "deny mode never snoops, even on a forwarded answer")
}

func TestHandler_UpstreamFailureReturnsServerFailure(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.2")
	matchers, _ := CompileMatchers([]string{"example.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 0, NamespaceName: "vm-ns0",
		Mode: ModeAllow, Matchers: matchers,
	})
	up := &fakeUpstream{err: assert.AnError}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.2")}
	h.ServeDNS(w, newAQuery("example.com."))

	require.NotNil(t, w.resp)
	assert.Equal(t, miekg.RcodeServerFailure, w.resp.Rcode)
	assert.Empty(t, sn.hits)
}

func TestHandler_TTLClampedToFloor(t *testing.T) {
	store := NewPolicyStore()
	src := netip.MustParseAddr("10.0.0.2")
	matchers, _ := CompileMatchers([]string{"example.com"})
	store.Upsert(src, SandboxDNSPolicy{
		Index: 0, NamespaceName: "vm-ns0",
		Mode: ModeAllow, Matchers: matchers,
	})
	// 0 TTL from upstream: clamp to MinEntryTTL (1s in this fixture)
	// so the firewall rule has at least one positive second of life.
	up := &fakeUpstream{answer: newAAnswer("example.com.", "1.2.3.4", 0)}
	sn := &fakeSnooper{}
	h := newTestHandler(t, store, up, sn)

	w := &fakeWriter{remote: srcUDP("10.0.0.2")}
	h.ServeDNS(w, newAQuery("example.com."))

	require.Len(t, sn.hits, 1)
	assert.Equal(t, 1*time.Second, sn.hits[0].TTL)
}
