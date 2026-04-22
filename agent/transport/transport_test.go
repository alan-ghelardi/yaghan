package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.nuinfra.net/agent/internal/framing"
)

// receiveTimeout bounds any single channel read or server-side
// synchronisation wait in tests. Slightly larger than handshakeTimeout
// so handshake failures surface as concrete errors, not deadlines.
const receiveTimeout = 6 * time.Second

// --- fake Firecracker UDS ------------------------------------------------

// fakeAgent listens on a UDS path and hands out one accepted conn at a
// time. Tests drive the post-accept behavior (handshake responses,
// framed exchanges) inline so each case can choose its own flavor.
type fakeAgent struct {
	path string
	ln   net.Listener
}

func newFakeAgent(t *testing.T) *fakeAgent {
	t.Helper()
	// Keep the path short — Linux sun_path is capped at 108 bytes and
	// t.TempDir paths are usually fine, but we explicitly avoid nesting
	// deeper.
	path := filepath.Join(t.TempDir(), "v.sock")
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return &fakeAgent{path: path, ln: ln}
}

// readConnectLine reads the CONNECT line the host side always sends
// first, asserting it matches the expected guest port.
func readConnectLine(t *testing.T, br *bufio.Reader, wantPort uint32) {
	t.Helper()
	line, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("CONNECT %d\n", wantPort), line)
}

// writeOK writes the canonical success reply to conn.
func writeOK(t *testing.T, conn net.Conn) {
	t.Helper()
	_, err := conn.Write([]byte("OK 54321\n"))
	assert.NoError(t, err)
}

// --- happy path ----------------------------------------------------------

func TestHandshakeAndConversationRoundtrip(t *testing.T) {
	fa := newFakeAgent(t)

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		readConnectLine(t, br, 52)
		writeOK(t, conn)

		var req dataplanev1alpha1.AgentRequest
		if !assert.NoError(t, framing.Read(br, &req)) {
			return
		}
		// Echo the request id back so the conversation's Recv routes
		// the response correctly.
		assert.NoError(t, framing.Write(conn, &dataplanev1alpha1.AgentResponse{
			Id: req.GetId(),
			Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
				ExecResponse: &dataplanev1alpha1.ExecResponse{
					Payload: &dataplanev1alpha1.ExecResponse_ProcessResult{
						ProcessResult: &dataplanev1alpha1.ProcessResult{ExitCode: 0},
					},
				},
			},
		}))
		// Keep the conn open until the client closes so the reader
		// loop observes a clean shutdown rather than an abort.
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 52)
	require.NoError(t, err)

	conv := tr.OpenConversation()
	defer conv.Close()
	require.Positive(t, conv.ID(), "conversation id must be > 0")

	ctx, cancel := context.WithTimeout(t.Context(), receiveTimeout)
	defer cancel()
	require.NoError(t, conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
		Payload: &dataplanev1alpha1.AgentRequest_ExecRequest{
			ExecRequest: &dataplanev1alpha1.ExecRequest{
				Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
					ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/true"},
				},
			},
		},
	}))

	select {
	case resp, ok := <-conv.Recv():
		require.True(t, ok, "conversation channel closed before response")
		assert.Equal(t, conv.ID(), resp.GetId(),
			"conversation must receive only its own id")
	case <-time.After(receiveTimeout):
		t.Fatal("no response within timeout")
	}

	require.NoError(t, tr.Close())
	assert.NoError(t, tr.Err(), "clean close must not record an error")
	<-serverDone
}

func TestConcurrentConversationsAreDemuxed(t *testing.T) {
	fa := newFakeAgent(t)
	const n = 8

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		readConnectLine(t, br, 11)
		writeOK(t, conn)

		// For each request received, echo a response with the same id.
		for i := 0; i < n; i++ {
			var req dataplanev1alpha1.AgentRequest
			if !assert.NoError(t, framing.Read(br, &req)) {
				return
			}
			assert.NoError(t, framing.Write(conn, &dataplanev1alpha1.AgentResponse{
				Id: req.GetId(),
				Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
					ExecResponse: &dataplanev1alpha1.ExecResponse{
						Payload: &dataplanev1alpha1.ExecResponse_ProcessResult{
							ProcessResult: &dataplanev1alpha1.ProcessResult{ExitCode: int32(i)},
						},
					},
				},
			}))
		}
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 11)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	convs := make([]Conversation, n)
	for i := range convs {
		convs[i] = tr.OpenConversation()
	}

	ctx, cancel := context.WithTimeout(t.Context(), receiveTimeout)
	defer cancel()

	// Fire all sends; ids are deterministic so the server's responses
	// land on the matching conversation.
	for _, c := range convs {
		require.NoError(t, c.Send(ctx, &dataplanev1alpha1.AgentRequest{}))
	}

	for i, c := range convs {
		select {
		case resp, ok := <-c.Recv():
			require.True(t, ok, "conv %d closed early", i)
			assert.Equal(t, c.ID(), resp.GetId(),
				"conv %d received a frame with mismatched id", i)
		case <-time.After(receiveTimeout):
			t.Fatalf("conv %d did not receive its frame in time", i)
		}
	}
}

// --- handshake failure modes --------------------------------------------

func TestHandshakeRejectsMalformedOK(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 1)
		_, _ = conn.Write([]byte("NOPE\n"))
	}()

	_, err := New(fa.path, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected handshake reply")
}

func TestHandshakeRejectsOversizeLine(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 1)
		// Write maxHandshakeLine+1 non-newline bytes so the reader caps
		// out before ever seeing '\n'.
		garbage := make([]byte, maxHandshakeLine+16)
		for i := range garbage {
			garbage[i] = 'x'
		}
		_, _ = conn.Write(garbage)
	}()

	_, err := New(fa.path, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestHandshakeTimesOutOnSilentPeer(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		// Read the CONNECT so the client's write completes, then stall
		// — never reply.
		_, _ = bufio.NewReader(conn).ReadString('\n')
		t.Cleanup(func() { _ = conn.Close() })
	}()

	done := make(chan error, 1)
	go func() {
		_, err := New(fa.path, 1)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(handshakeTimeout + 2*time.Second):
		t.Fatal("New did not return within handshake timeout")
	}
}

func TestDialFailsOnMissingSocket(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.sock")
	_, err := New(missing, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial")
}

// --- Close and error surfacing ------------------------------------------

func TestCloseClosesAllConversationsAndIsIdempotent(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 9)
		writeOK(t, conn)
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 9)
	require.NoError(t, err)

	conv := tr.OpenConversation()
	require.NoError(t, tr.Close())
	require.NoError(t, tr.Close(), "Close must be idempotent")

	select {
	case _, ok := <-conv.Recv():
		assert.False(t, ok, "conversation channel must be closed after Transport.Close")
	case <-time.After(receiveTimeout):
		t.Fatal("conversation channel not closed after Transport.Close")
	}
	assert.NoError(t, tr.Err(), "Close-triggered shutdown must not record an error")
}

func TestOpenConversationAfterCloseReturnsClosedRecv(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 1)
		writeOK(t, conn)
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 1)
	require.NoError(t, err)
	require.NoError(t, tr.Close())

	conv := tr.OpenConversation()
	select {
	case _, ok := <-conv.Recv():
		assert.False(t, ok, "conversations opened after Close must come pre-closed")
	case <-time.After(receiveTimeout):
		t.Fatal("conversation channel not closed for post-Close OpenConversation")
	}
}

func TestSendAfterCloseErrors(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 1)
		writeOK(t, conn)
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 1)
	require.NoError(t, err)
	conv := tr.OpenConversation()
	require.NoError(t, tr.Close())

	err = conv.Send(t.Context(), &dataplanev1alpha1.AgentRequest{})
	require.Error(t, err, "Send on a closed transport must fail")
}

func TestPeerDropClosesConversationsAndRecordsError(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 2)
		writeOK(t, conn)
		// Write a half-frame (length prefix declaring 16 bytes, body
		// of 0) then close — the reader should surface a framing error
		// rather than a clean EOF.
		_, _ = conn.Write([]byte{0x00, 0x00, 0x00, 0x10})
		_ = conn.Close()
	}()

	tr, err := New(fa.path, 2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	conv := tr.OpenConversation()
	select {
	case _, ok := <-conv.Recv():
		assert.False(t, ok, "conversation channel must close after peer drop")
	case <-time.After(receiveTimeout):
		t.Fatal("conversation channel did not close after peer drop")
	}
	assert.Error(t, tr.Err(), "a dirty close must be reflected in Err")
}

// --- concurrency --------------------------------------------------------

func TestConcurrentSendsSerializeFrames(t *testing.T) {
	fa := newFakeAgent(t)
	const n = 32

	var serverErr atomic.Value
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := fa.ln.Accept()
		if err != nil {
			serverErr.Store(err)
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		readConnectLine(t, br, 3)
		writeOK(t, conn)

		// Read n frames — if concurrent writes interleave, framing.Read
		// will surface it as a size/unmarshal error and we fail fast.
		for i := 0; i < n; i++ {
			var req dataplanev1alpha1.AgentRequest
			if err := framing.Read(br, &req); err != nil {
				serverErr.Store(fmt.Errorf("frame %d: %w", i, err))
				return
			}
		}
	}()

	tr, err := New(fa.path, 3)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			conv := tr.OpenConversation()
			defer conv.Close()
			err := conv.Send(t.Context(), &dataplanev1alpha1.AgentRequest{
				Payload: &dataplanev1alpha1.AgentRequest_ExecRequest{
					ExecRequest: &dataplanev1alpha1.ExecRequest{
						Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
							ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/true"},
						},
					},
				},
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	select {
	case <-serverDone:
	case <-time.After(receiveTimeout):
		t.Fatal("server did not finish reading frames in time")
	}
	if v := serverErr.Load(); v != nil {
		t.Fatalf("server error (implies frame interleaving): %v", v)
	}
}

// --- ctx plumbing on Send -----------------------------------------------

func TestSendReturnsCtxErrWhenAlreadyCancelled(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 4)
		writeOK(t, conn)
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 4)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	conv := tr.OpenConversation()
	defer conv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = conv.Send(ctx, &dataplanev1alpha1.AgentRequest{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled),
		"pre-cancelled ctx should surface as context.Canceled, got %v", err)
}

// --- conversation lifecycle --------------------------------------------

func TestConversationCloseIsIdempotent(t *testing.T) {
	fa := newFakeAgent(t)

	go func() {
		conn, err := fa.ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		readConnectLine(t, br, 5)
		writeOK(t, conn)
		_, _ = io.Copy(io.Discard, conn)
	}()

	tr, err := New(fa.path, 5)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	conv := tr.OpenConversation()
	require.NoError(t, conv.Close())
	require.NoError(t, conv.Close(), "second Close must be a no-op")

	select {
	case _, ok := <-conv.Recv():
		assert.False(t, ok, "Recv must be closed after Conversation.Close")
	case <-time.After(receiveTimeout):
		t.Fatal("Recv did not close after Conversation.Close")
	}
}
