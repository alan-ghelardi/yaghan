package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	miekg "github.com/miekg/dns"
)

// Upstream forwards a query to one or more recursive resolvers and
// returns the first usable answer. Implementations must be safe for
// concurrent use.
type Upstream interface {
	Exchange(ctx context.Context, req *miekg.Msg) (*miekg.Msg, error)
}

// Snooper receives every (A or AAAA) record observed in an allowed
// response so the firewall can grow the per-sandbox ipset. The
// handler invokes it synchronously per record; implementations must
// be fast (the call sits in the DNS response path) and tolerant of
// duplicate writes — the kernel itself dedupes.
type Snooper interface {
	// Note allows ip for the sandbox identified by policy with the
	// supplied ttl. Errors are logged by the handler but do not fail
	// the DNS response, on the principle that returning the right
	// answer is more important than a missed firewall update which
	// the next resolution will retry.
	Note(policy SandboxDNSPolicy, ip netip.Addr, ttl time.Duration) error
}

// Logger is the small subset of zap (or any structured logger) the
// handler emits to. Extracted as an interface so callers can plug
// in zap, log/slog, or a discard for tests.
type Logger interface {
	Warn(msg string, fields ...any)
	Debug(msg string, fields ...any)
}

// nopLogger discards every entry — handy default for tests that
// don't care about log output.
type nopLogger struct{}

func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Debug(string, ...any) {}

// HandlerOptions configures a [Handler].
type HandlerOptions struct {
	// Store is the source of per-sandbox policies. Required.
	Store *PolicyStore

	// Upstream forwards queries that survive the policy check.
	// Required.
	Upstream Upstream

	// Snooper receives observed A/AAAA records under allow-mode
	// policies so the firewall can grant access. Required.
	Snooper Snooper

	// MinEntryTTL is the floor applied to per-answer TTLs before they
	// are passed to the Snooper. Without it, very-short-TTL CDNs
	// would cause near-constant ipset churn. Default: 30 seconds.
	MinEntryTTL time.Duration

	// MaxEntryTTL is the ceiling applied to per-answer TTLs. Caps
	// how long a stale answer can keep a destination open after the
	// policy is changed or the domain stops resolving the same way.
	// Default: 5 minutes.
	MaxEntryTTL time.Duration

	// ForwardTimeout bounds the upstream call. Default: 5 seconds.
	ForwardTimeout time.Duration

	// Logger receives structured diagnostics. Defaults to a
	// no-op — supply zap.SugaredLogger or similar for production.
	Logger Logger
}

// Handler is the miekg/dns request handler that enforces per-sandbox
// domain policies and snoops answers into the firewall.
type Handler struct {
	store          *PolicyStore
	upstream       Upstream
	snooper        Snooper
	minTTL, maxTTL time.Duration
	forwardTimeout time.Duration
	logger         Logger
}

// NewHandler validates opts and returns the configured handler.
// Returns an error rather than panicking so misconfiguration surfaces
// at daemon startup rather than on the first query.
func NewHandler(opts HandlerOptions) (*Handler, error) {
	if opts.Store == nil {
		return nil, errors.New("PolicyStore is required")
	}
	if opts.Upstream == nil {
		return nil, errors.New("Upstream is required")
	}
	if opts.Snooper == nil {
		return nil, errors.New("Snooper is required")
	}
	if opts.MinEntryTTL <= 0 {
		opts.MinEntryTTL = 30 * time.Second
	}
	if opts.MaxEntryTTL <= 0 {
		opts.MaxEntryTTL = 5 * time.Minute
	}
	if opts.ForwardTimeout <= 0 {
		opts.ForwardTimeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = nopLogger{}
	}
	return &Handler{
		store:          opts.Store,
		upstream:       opts.Upstream,
		snooper:        opts.Snooper,
		minTTL:         opts.MinEntryTTL,
		maxTTL:         opts.MaxEntryTTL,
		forwardTimeout: opts.ForwardTimeout,
		logger:         opts.Logger,
	}, nil
}

// ServeDNS implements miekg/dns Handler.
//
// Flow per query:
//
//  1. Source IP is extracted from w.RemoteAddr; if the store has no
//     entry for it, the query is REFUSED. The DNS server should only
//     be reachable from sandboxes that the reconciler registered.
//  2. Policy mode dispatches: ModeAllow refuses non-matching names,
//     ModeDeny refuses matching names, ModeNone forwards
//     unconditionally.
//  3. Surviving queries are forwarded upstream within ForwardTimeout.
//     Each A/AAAA record in the response is passed to the Snooper
//     with a clamped TTL. Snooper errors are logged but do not fail
//     the response.
func (h *Handler) ServeDNS(w miekg.ResponseWriter, req *miekg.Msg) {
	// All early-exit failure modes go through this single setter so
	// the wire format stays consistent.
	respond := func(rcode int) {
		resp := new(miekg.Msg)
		resp.SetRcode(req, rcode)
		_ = w.WriteMsg(resp)
	}

	if len(req.Question) == 0 {
		respond(miekg.RcodeFormatError)
		return
	}
	srcIP, ok := remoteIP(w.RemoteAddr())
	if !ok {
		h.logger.Warn("dns: query from non-IP transport, refusing",
			"remote", w.RemoteAddr())
		respond(miekg.RcodeRefused)
		return
	}
	policy, ok := h.store.Lookup(srcIP)
	if !ok {
		// Defense in depth: bind ACLs should already prevent unknown
		// sources from reaching us. REFUSED tells the client we
		// understood the query but won't answer it.
		h.logger.Warn("dns: query from unregistered source, refusing",
			"src", srcIP.String())
		respond(miekg.RcodeRefused)
		return
	}

	qname := req.Question[0].Name
	matched := MatchAny(policy.Matchers, qname)

	switch policy.Mode {
	case ModeAllow:
		if !matched {
			h.logger.Debug("dns: refused (not in allow list)",
				"src", srcIP.String(), "name", qname)
			respond(miekg.RcodeRefused)
			return
		}
	case ModeDeny:
		if matched {
			h.logger.Debug("dns: refused (matched deny list)",
				"src", srcIP.String(), "name", qname)
			respond(miekg.RcodeRefused)
			return
		}
	case ModeNone:
		// fallthrough — plain recursive resolution.
	default:
		h.logger.Warn("dns: unknown policy mode, refusing",
			"src", srcIP.String(), "mode", policy.Mode)
		respond(miekg.RcodeRefused)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.forwardTimeout)
	defer cancel()
	resp, err := h.upstream.Exchange(ctx, req)
	if err != nil || resp == nil {
		h.logger.Warn("dns: upstream forward failed",
			"src", srcIP.String(), "name", qname, "err", err)
		respond(miekg.RcodeServerFailure)
		return
	}

	// Snoop A/AAAA records only when the answer is to a name we
	// vouched for under allow-mode — those are the only IPs the
	// firewall needs to dynamically permit. Deny-mode refusals
	// short-circuited above; forwarded queries under deny-mode
	// (non-matching) don't need the snooper either.
	if policy.Mode == ModeAllow {
		h.snoop(policy, resp)
	}

	resp.Id = req.Id // keep the client's transaction id
	_ = w.WriteMsg(resp)
}

// snoop walks the answer section, clamps each TTL into the
// [MinEntryTTL, MaxEntryTTL] window, and hands the (ip, ttl) pair to
// the Snooper. CNAMEs and other RR types are ignored: only A/AAAA
// records translate into firewall destinations.
func (h *Handler) snoop(policy SandboxDNSPolicy, resp *miekg.Msg) {
	for _, rr := range resp.Answer {
		var addr netip.Addr
		var rawTTL uint32
		switch a := rr.(type) {
		case *miekg.A:
			ip, ok := netip.AddrFromSlice(a.A.To4())
			if !ok {
				continue
			}
			addr = ip.Unmap()
			rawTTL = a.Hdr.Ttl
		case *miekg.AAAA:
			ip, ok := netip.AddrFromSlice(a.AAAA.To16())
			if !ok {
				continue
			}
			addr = ip
			rawTTL = a.Hdr.Ttl
		default:
			continue
		}
		ttl := clampTTL(time.Duration(rawTTL)*time.Second, h.minTTL, h.maxTTL)
		if err := h.snooper.Note(policy, addr, ttl); err != nil {
			h.logger.Warn("dns: snoop failed",
				"sandbox", policy.Index, "ip", addr.String(), "err", err)
		}
	}
}

// clampTTL bounds ttl into [floor, ceiling]. Tolerates ttl <= 0
// (treated as floor) so an answer with a zero or absurd TTL doesn't
// generate a permanent ipset entry.
func clampTTL(ttl, floor, ceiling time.Duration) time.Duration {
	if ttl < floor {
		return floor
	}
	if ttl > ceiling {
		return ceiling
	}
	return ttl
}

// remoteIP extracts an IPv4 or IPv6 address from a net.Addr returned
// by miekg/dns. Returns ok=false for transports we don't expect
// (unix sockets, etc.) so callers can REFUSE rather than guess.
func remoteIP(addr net.Addr) (netip.Addr, bool) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		ip, ok := netip.AddrFromSlice(a.IP)
		return ip.Unmap(), ok
	case *net.TCPAddr:
		ip, ok := netip.AddrFromSlice(a.IP)
		return ip.Unmap(), ok
	default:
		return netip.Addr{}, false
	}
}

// DefaultUpstream wraps miekg's `dns.Client` with retry across a list
// of upstream resolvers. The first responsive resolver wins; the
// rest serve as fallbacks. Resolvers are tried in order so an
// operator can put preferred / faster nameservers first.
type DefaultUpstream struct {
	resolvers []string // "host:port" form
	udp, tcp  *miekg.Client
}

// NewDefaultUpstream constructs an [Upstream] backed by the canonical
// miekg/dns Client. Each resolver must be a "host:port" string —
// callers that have a list of bare IPs should append ":53".
func NewDefaultUpstream(resolvers []string, timeout time.Duration) (*DefaultUpstream, error) {
	if len(resolvers) == 0 {
		return nil, errors.New("at least one upstream resolver required")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &DefaultUpstream{
		resolvers: append([]string(nil), resolvers...),
		udp:       &miekg.Client{Net: "udp", Timeout: timeout},
		tcp:       &miekg.Client{Net: "tcp", Timeout: timeout},
	}, nil
}

// Exchange implements [Upstream]. UDP is tried first; on truncation
// the same query is re-issued over TCP per RFC 5966. On any other
// error the next resolver in the list is tried.
func (u *DefaultUpstream) Exchange(ctx context.Context, req *miekg.Msg) (*miekg.Msg, error) {
	_ = ctx // miekg's Client.Timeout handles the deadline; ctx is reserved for future use.
	var lastErr error
	for _, server := range u.resolvers {
		resp, _, err := u.udp.Exchange(req, server)
		if err == nil && resp != nil && resp.Truncated {
			resp, _, err = u.tcp.Exchange(req, server)
		}
		if err == nil && resp != nil {
			return resp, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no usable upstream")
	}
	return nil, fmt.Errorf("upstream exchange: %w", lastErr)
}
