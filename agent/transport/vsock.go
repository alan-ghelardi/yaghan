package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"golang.nuinfra.net/agent/internal/framing"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
)

const (
	// handshakeTimeout bounds the CONNECT/OK exchange. Firecracker
	// typically answers sub-millisecond; a stalled peer should fail fast
	// rather than blocking New indefinitely.
	handshakeTimeout = 5 * time.Second

	// maxHandshakeLine caps the OK response length. Firecracker replies
	// with "OK <host_port>\n" — at most "OK 65535\n" (9 bytes); 32 is
	// generous without letting a hostile peer grow our read.
	maxHandshakeLine = 32

	// conversationBuffer smooths small bursts the agent can emit on a
	// single conversation without unbounded memory growth. 16 is the
	// same order of magnitude as the guest's streaming chunk cadence.
	conversationBuffer = 16
)

// aLongTimeAgo is used to abort an in-flight write by pushing the
// deadline into the past. Stdlib net/http uses the same trick.
var aLongTimeAgo = time.Unix(1, 0)

// okResponsePattern matches the handshake reply Firecracker sends after
// a successful CONNECT. The host-side port (group 1) is informational.
var okResponsePattern = regexp.MustCompile(`^OK \d+$`)

// New dials Firecracker's vsock UDS at udsPath, performs the CONNECT
// handshake for guestPort, and returns a ready-to-use Transport. The
// connection is torn down and an error returned if any step of the
// handshake fails, so a non-nil Transport is always usable.
func New(udsPath string, guestPort uint32) (Transport, error) {
	conn, err := net.Dial("unix", udsPath)
	if err != nil {
		return nil, fmt.Errorf("transport: dial %q: %w", udsPath, err)
	}
	br, err := handshake(conn, guestPort)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	t := &vsockTransport{
		conn:          conn,
		reader:        br,
		closed:        make(chan struct{}),
		conversations: make(map[uint64]*vsockConversation),
	}
	go t.readLoop()
	return t, nil
}

// handshake drives the ASCII CONNECT/OK exchange and returns a buffered
// reader wrapping conn. Reads go through that wrapper so any bytes the
// peer prefetched after the OK line are preserved for the framing layer.
func handshake(conn net.Conn, guestPort uint32) (*bufio.Reader, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, fmt.Errorf("transport: set handshake deadline: %w", err)
	}
	defer func() {
		// Clear the deadline before the reader goroutine takes over.
		// SetDeadline after Close returns an error we intentionally
		// ignore.
		_ = conn.SetDeadline(time.Time{})
	}()

	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", guestPort); err != nil {
		return nil, fmt.Errorf("transport: write CONNECT: %w", err)
	}

	br := bufio.NewReader(conn)
	line, err := readHandshakeLine(br)
	if err != nil {
		return nil, err
	}
	if !okResponsePattern.MatchString(line) {
		return nil, fmt.Errorf("transport: unexpected handshake reply %q", line)
	}
	return br, nil
}

// readHandshakeLine reads up to maxHandshakeLine bytes ending in '\n',
// returning the line without the terminator. A peer that never sends
// '\n' or that floods us with a long line is rejected — we never grow
// the read past the cap.
func readHandshakeLine(br *bufio.Reader) (string, error) {
	buf := make([]byte, 0, maxHandshakeLine)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return "", fmt.Errorf("transport: read OK: %w", err)
		}
		if b == '\n' {
			return string(buf), nil
		}
		buf = append(buf, b)
		if len(buf) >= maxHandshakeLine {
			return "", fmt.Errorf("transport: handshake reply exceeds %d bytes", maxHandshakeLine)
		}
	}
}

type vsockTransport struct {
	conn   net.Conn
	reader *bufio.Reader

	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    chan struct{}

	errMu sync.Mutex
	err   error

	// nextID hands out request ids monotonically. Starts at 1 so a
	// zero AgentResponse.Id (a malformed frame) cannot match a real
	// conversation.
	nextID atomic.Uint64

	convMu        sync.Mutex
	conversations map[uint64]*vsockConversation
}

// OpenConversation implements [Transport].
func (t *vsockTransport) OpenConversation() Conversation {
	id := t.nextID.Add(1)
	conv := &vsockConversation{
		id:        id,
		transport: t,
		responses: make(chan *dataplanev1alpha1.AgentResponse, conversationBuffer),
	}

	t.convMu.Lock()
	if t.isClosed() {
		// The transport already shut down; surface a closed channel so
		// the caller's range over Recv exits immediately.
		t.convMu.Unlock()
		close(conv.responses)
		return conv
	}
	t.conversations[id] = conv
	t.convMu.Unlock()
	return conv
}

// Err implements [Transport].
func (t *vsockTransport) Err() error {
	t.errMu.Lock()
	defer t.errMu.Unlock()
	return t.err
}

// Close implements [Transport]. Idempotent — subsequent calls are
// no-ops and always return nil. All open conversations are closed and
// their Recv channels drained.
func (t *vsockTransport) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
		_ = t.conn.Close()
		t.closeAllConversations()
	})
	return nil
}

// closeAllConversations is invoked once when the transport itself
// closes; it tears down every still-registered conversation so callers
// blocked on Recv unblock with a closed channel.
func (t *vsockTransport) closeAllConversations() {
	t.convMu.Lock()
	convs := make([]*vsockConversation, 0, len(t.conversations))
	for _, c := range t.conversations {
		convs = append(convs, c)
	}
	t.conversations = nil
	t.convMu.Unlock()

	for _, c := range convs {
		c.shutdown()
	}
}

// removeConversation deregisters c from the conversations map. Called
// by [vsockConversation.Close]; does NOT close the response channel
// (the conversation owns that lifecycle).
func (t *vsockTransport) removeConversation(id uint64) {
	t.convMu.Lock()
	defer t.convMu.Unlock()
	delete(t.conversations, id)
}

// dispatch routes one inbound AgentResponse to the conversation that
// owns its id. Frames whose id is unknown are dropped silently — the
// conversation may have already closed (race) or the peer is
// misbehaving; either way we don't want to block the read loop.
func (t *vsockTransport) dispatch(resp *dataplanev1alpha1.AgentResponse) {
	t.convMu.Lock()
	conv, ok := t.conversations[resp.GetId()]
	t.convMu.Unlock()
	if !ok {
		return
	}
	select {
	case conv.responses <- resp:
	case <-t.closed:
	}
}

// sendFrame is invoked by a conversation's Send. The mutex serialises
// concurrent writes to the same connection.
func (t *vsockTransport) sendFrame(ctx context.Context, req *dataplanev1alpha1.AgentRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	deadline := time.Time{}
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	if err := t.conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("transport: set write deadline: %w", err)
	}

	// Translate a mid-write ctx cancellation into an immediate write
	// deadline. The watcher is stopped before we clear the deadline on
	// exit, so a subsequent Send does not observe a stale deadline.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			_ = t.conn.SetWriteDeadline(aLongTimeAgo)
		case <-done:
		}
	}()
	defer func() {
		close(done)
		wg.Wait()
		_ = t.conn.SetWriteDeadline(time.Time{})
	}()

	if err := framing.Write(t.conn, req); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("transport: send: %w", err)
	}
	return nil
}

// readLoop is the sole reader on the conn. Each successfully decoded
// AgentResponse is dispatched to the conversation that owns its id.
// On error the loop stops, the error is stashed for [Transport.Err],
// and the transport is fully closed — closing t.closed ensures any
// later OpenConversation observes the closed state instead of racing
// against the now-nil conversations map.
func (t *vsockTransport) readLoop() {
	defer func() { _ = t.Close() }()
	for {
		resp := new(dataplanev1alpha1.AgentResponse)
		if err := framing.Read(t.reader, resp); err != nil {
			if !t.isClosed() {
				t.setErr(err)
			}
			return
		}
		t.dispatch(resp)
	}
}

func (t *vsockTransport) isClosed() bool {
	select {
	case <-t.closed:
		return true
	default:
		return false
	}
}

func (t *vsockTransport) setErr(err error) {
	// Never overwrite a non-nil error with io.EOF from a later read —
	// the first real failure is the most informative one.
	t.errMu.Lock()
	defer t.errMu.Unlock()
	if t.err == nil && !errors.Is(err, net.ErrClosed) {
		t.err = err
	}
}

// vsockConversation is a per-RPC view of the underlying transport.
// Send writes through the transport's serialised pipe; Recv yields
// only frames whose id matches the conversation's own.
type vsockConversation struct {
	id        uint64
	transport *vsockTransport
	responses chan *dataplanev1alpha1.AgentResponse

	closeOnce sync.Once
}

// ID implements [Conversation].
func (c *vsockConversation) ID() uint64 {
	return c.id
}

// Send implements [Conversation]. Stamps the conversation's id onto
// req before writing so callers cannot accidentally route a frame to
// a different conversation.
func (c *vsockConversation) Send(ctx context.Context, req *dataplanev1alpha1.AgentRequest) error {
	req.Id = c.id
	return c.transport.sendFrame(ctx, req)
}

// Recv implements [Conversation].
func (c *vsockConversation) Recv() <-chan *dataplanev1alpha1.AgentResponse {
	return c.responses
}

// Close implements [Conversation]. Idempotent — subsequent calls are
// no-ops.
func (c *vsockConversation) Close() error {
	c.shutdown()
	return nil
}

// shutdown deregisters the conversation from the transport and closes
// its response channel. Safe to call multiple times; safe to call from
// either the owner (Close) or the read loop (close-cascade).
func (c *vsockConversation) shutdown() {
	c.closeOnce.Do(func() {
		c.transport.removeConversation(c.id)
		close(c.responses)
	})
}
