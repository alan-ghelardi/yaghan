package guest

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	dataplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/data_plane/v1alpha1"

	"github.com/alan-ghelardi/yaghan/agent/internal/framing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

// responseDeadline caps any individual Response read during tests. If a
// test would otherwise hang waiting for the next Response, this surfaces
// it as a test failure rather than a suite-wide timeout.
const responseDeadline = 5 * time.Second

// --- helpers --------------------------------------------------------------

func newTestServer(t *testing.T) *server {
	t.Helper()
	return &server{logger: log.New(io.Discard, "", 0)}
}

// dialHandle wires one end of a net.Pipe to srv.handle and returns the
// other end for the test to drive. Both ends are closed on cleanup; the
// handle goroutine is waited on so a failing test can't leak it. The
// connection ctx is derived from t.Context so suite-level cancellation
// (e.g. -timeout) reaches the handler.
func dialHandle(t *testing.T, srv *server) net.Conn {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handle(t.Context(), serverConn)
	}()
	t.Cleanup(func() {
		_ = clientConn.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("handle goroutine did not exit within 5s of client close")
		}
	})
	return clientConn
}

func sendRequest(t *testing.T, conn net.Conn, req *dataplanev1alpha1.AgentRequest) {
	t.Helper()
	require.NoError(t, framing.Write(conn, req))
}

func readResponse(t *testing.T, conn net.Conn) *dataplanev1alpha1.AgentResponse {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(responseDeadline)))
	var resp dataplanev1alpha1.AgentResponse
	require.NoError(t, framing.Read(conn, &resp))
	return &resp
}

// execResult captures the responses a single Exec produces: accumulated
// stdout / stderr bytes, per-stream EOF flags, the host-visible pid
// seen on any stream chunk (must be consistent), and the terminal
// ProcessResult unwrapped from the closing ExecResponse.
type execResult struct {
	stdout, stderr       []byte
	stdoutEOF, stderrEOF bool
	pid                  int32
	// Exactly one of result / errStatus is populated per exec — the
	// terminal payload is either a ProcessResult (process ran) or the
	// google.rpc.Status error variant (couldn't start, etc.).
	result    *dataplanev1alpha1.ProcessResult
	errStatus *status.Status
}

// readExec drains responses for id until a terminal payload arrives
// (ProcessResult or Error).
func readExec(t *testing.T, conn net.Conn, id uint64) execResult {
	t.Helper()
	var res execResult
	for {
		resp := readResponse(t, conn)
		if resp.Id != id {
			t.Fatalf("unexpected response id %d (want %d)", resp.Id, id)
		}
		switch p := resp.Payload.(type) {
		case *dataplanev1alpha1.AgentResponse_ExecResponse:
			switch inner := p.ExecResponse.GetPayload().(type) {
			case *dataplanev1alpha1.ExecResponse_StreamChunk:
				chunk := inner.StreamChunk
				// Every stream chunk should carry the same pid for the
				// duration of an Exec.
				if res.pid == 0 {
					res.pid = chunk.Pid
				} else if chunk.Pid != 0 && chunk.Pid != res.pid {
					t.Fatalf("stream pid changed mid-exec: %d → %d", res.pid, chunk.Pid)
				}
				switch chunk.Stream {
				case dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT:
					res.stdout = append(res.stdout, chunk.Data...)
					if chunk.Eof {
						res.stdoutEOF = true
					}
				case dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDERR:
					res.stderr = append(res.stderr, chunk.Data...)
					if chunk.Eof {
						res.stderrEOF = true
					}
				}
			case *dataplanev1alpha1.ExecResponse_ProcessResult:
				res.result = inner.ProcessResult
				return res
			default:
				t.Fatalf("unexpected ExecResponse payload %T", inner)
			}
		case *dataplanev1alpha1.AgentResponse_Error:
			res.errStatus = p.Error
			return res
		default:
			t.Fatalf("unexpected response payload %T", resp.Payload)
		}
	}
}

func execRequest(id uint64, proc *dataplanev1alpha1.ExecProcess) *dataplanev1alpha1.AgentRequest {
	return &dataplanev1alpha1.AgentRequest{
		Id: id,
		Payload: &dataplanev1alpha1.AgentRequest_ExecRequest{
			ExecRequest: &dataplanev1alpha1.ExecRequest{
				Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{ExecProcess: proc},
			},
		},
	}
}

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not on PATH: %v", name, err)
	}
}

// --- makeEnviron ----------------------------------------------------------

func TestMakeEnviron(t *testing.T) {
	t.Setenv("__AGENT_TEST_EXISTING", "from-parent")

	t.Run("request vars come before parent", func(t *testing.T) {
		out := makeEnviron(&dataplanev1alpha1.ExecProcess{
			Env: map[string]string{"FOO": "bar"},
		})
		parent := os.Environ()
		require.GreaterOrEqual(t, len(out), len(parent)+1)
		assert.Equal(t, "FOO=bar", out[0], "request var must precede parent env")
		assert.Contains(t, out, "__AGENT_TEST_EXISTING=from-parent",
			"parent vars must still be present")
	})

	t.Run("request override wins for getenv semantics", func(t *testing.T) {
		out := makeEnviron(&dataplanev1alpha1.ExecProcess{
			Env: map[string]string{"__AGENT_TEST_EXISTING": "from-request"},
		})
		first := firstValue(out, "__AGENT_TEST_EXISTING")
		assert.Equal(t, "from-request", first,
			"getenv returns the first match; request must be first")
	})

	t.Run("nil env is safe", func(t *testing.T) {
		out := makeEnviron(&dataplanev1alpha1.ExecProcess{})
		assert.Equal(t, os.Environ(), out)
	})
}

func firstValue(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

// --- exec behavior --------------------------------------------------------

func TestExecSucceeds(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(1, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo hello"},
	}))
	res := readExec(t, conn, 1)

	assert.Equal(t, "hello\n", string(res.stdout))
	assert.Empty(t, res.stderr)
	assert.True(t, res.stdoutEOF, "stdout must end with Eof=true marker")
	assert.True(t, res.stderrEOF, "stderr must end with Eof=true marker")
	require.NotNil(t, res.result)
	assert.Equal(t, int32(0), res.result.ExitCode)
	assert.Nil(t, res.errStatus, "success must not carry an error status")
}

func TestExecStderr(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(2, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo err >&2"},
	}))
	res := readExec(t, conn, 2)

	assert.Empty(t, res.stdout)
	assert.Equal(t, "err\n", string(res.stderr))
	assert.True(t, res.stdoutEOF)
	assert.True(t, res.stderrEOF)
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func TestExecNonzeroExit(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(3, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 7"},
	}))
	res := readExec(t, conn, 3)

	// Non-zero exit is a successful RPC in the new design — only the
	// exit code is reported; Response.error stays unset.
	require.NotNil(t, res.result)
	assert.Equal(t, int32(7), res.result.ExitCode)
	assert.Nil(t, res.errStatus)
}

func TestExecCommandNotFound(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(4, &dataplanev1alpha1.ExecProcess{
		Command: "/does/not/exist/" + strconv.Itoa(os.Getpid()),
	}))
	res := readExec(t, conn, 4)

	assert.Empty(t, res.stdout)
	assert.Empty(t, res.stderr)
	assert.False(t, res.stdoutEOF, "no stream goroutine runs when start fails")
	assert.Nil(t, res.result, "failed start must not produce an ExecResponse")
	require.NotNil(t, res.errStatus)
	assert.Equal(t, int32(codes.NotFound), res.errStatus.Code,
		"missing binary should map to NOT_FOUND")
	assert.NotEmpty(t, res.errStatus.Message)
}

func TestExecCwdAndEnv(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	dir := t.TempDir()
	sendRequest(t, conn, execRequest(5, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "pwd; echo $AGENT_TEST_FOO"},
		Cwd:     dir,
		Env:     map[string]string{"AGENT_TEST_FOO": "bar"},
	}))
	res := readExec(t, conn, 5)

	// pwd may follow symlinks; accept either the raw or resolved path.
	assert.Contains(t, string(res.stdout), "bar\n")
	assert.True(t, strings.Contains(string(res.stdout), dir) ||
		strings.Contains(string(res.stdout), mustEvalSymlinks(t, dir)),
		"pwd output %q should contain %q", string(res.stdout), dir)
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func mustEvalSymlinks(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return resolved
}

func TestExecStdin(t *testing.T) {
	requireBinary(t, "cat")
	srv := newTestServer(t)
	conn := dialHandle(t, srv)

	// cat reads stdin until EOF and writes to stdout. We push data and
	// signal EOF in a single StdinChunk, exercising the proto's new
	// eof field end to end.
	sendRequest(t, conn, execRequest(6, &dataplanev1alpha1.ExecProcess{
		Command: "cat",
	}))

	// StdinChunks arriving before execCommand publishes the stdin
	// writer are silently dropped by design.
	waitForStdin(t, srv, 6)

	sendRequest(t, conn, &dataplanev1alpha1.AgentRequest{
		Id: 6,
		Payload: &dataplanev1alpha1.AgentRequest_Stdin{
			Stdin: &dataplanev1alpha1.StdinChunk{
				Data: []byte("helloworld"),
				Eof:  true,
			},
		},
	})
	res := readExec(t, conn, 6)

	assert.Equal(t, "helloworld", string(res.stdout))
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func TestExecStdinEofClosesPipe(t *testing.T) {
	requireBinary(t, "cat")
	srv := newTestServer(t)
	conn := dialHandle(t, srv)

	// An EOF-only chunk with no data must still close the child's
	// stdin, letting `cat` observe end-of-file and exit — otherwise
	// the process would block forever and hit the read deadline.
	sendRequest(t, conn, execRequest(11, &dataplanev1alpha1.ExecProcess{
		Command: "cat",
	}))
	waitForStdin(t, srv, 11)

	sendRequest(t, conn, &dataplanev1alpha1.AgentRequest{
		Id: 11,
		Payload: &dataplanev1alpha1.AgentRequest_Stdin{
			Stdin: &dataplanev1alpha1.StdinChunk{Eof: true},
		},
	})
	res := readExec(t, conn, 11)

	assert.Empty(t, res.stdout)
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func waitForStdin(t *testing.T, srv *server, id uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		v, ok := srv.procs.Load(id)
		if !ok {
			return false
		}
		p := v.(*proc)
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.stdin != nil
	}, time.Second, 5*time.Millisecond, "stdin never attached for id %d", id)
}

func TestExecCancel(t *testing.T) {
	requireBinary(t, "sleep")
	srv := newTestServer(t)
	conn := dialHandle(t, srv)

	sendRequest(t, conn, execRequest(7, &dataplanev1alpha1.ExecProcess{
		Command: "sleep",
		Args:    []string{"30"},
	}))

	// Wait for cmd.Start to have succeeded — stdin is only published
	// after that. Cancelling before start produces a different failure
	// shape (Error set, ExitCode still zero) and we want to exercise
	// the kill-mid-run path instead.
	waitForStdin(t, srv, 7)

	sendRequest(t, conn, &dataplanev1alpha1.AgentRequest{
		Id:      7,
		Payload: &dataplanev1alpha1.AgentRequest_Cancel{Cancel: &dataplanev1alpha1.CancelRequest{}},
	})

	res := readExec(t, conn, 7)
	// A killed process still has a ProcessState (it did run), so we
	// get an ExecResponse with a non-zero exit code, not an Error.
	require.NotNil(t, res.result)
	assert.NotEqual(t, int32(0), res.result.ExitCode, "killed process should not exit zero")
	assert.Nil(t, res.errStatus)
}

func TestExecTTY(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(8, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "tty"},
		Tty:     true,
	}))
	res := readExec(t, conn, 8)

	assert.Contains(t, string(res.stdout), "/dev/pts/",
		"tty(1) under PTY should report a pts device")
	assert.True(t, res.stdoutEOF)
	assert.False(t, res.stderrEOF, "TTY mode merges stderr into master; no separate stream")
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func TestExecResizePTY(t *testing.T) {
	requireBinary(t, "/bin/sh")
	requireBinary(t, "stty")
	srv := newTestServer(t)
	conn := dialHandle(t, srv)

	// Loop `stty size` for a while; the TTY is resized after the pty
	// handle is published, so we read multiple reports and assert that
	// *some* report carries the new size — robust against the initial
	// default reading first.
	sendRequest(t, conn, execRequest(9, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "for i in 1 2 3 4 5; do stty size; sleep 0.1; done"},
		Tty:     true,
	}))

	// Only send the Resize once the proc has a live pty master fd,
	// otherwise resizeTerm no-ops.
	require.Eventually(t, func() bool {
		v, ok := srv.procs.Load(uint64(9))
		if !ok {
			return false
		}
		p := v.(*proc)
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.pty != nil
	}, time.Second, 5*time.Millisecond, "pty never attached")

	sendRequest(t, conn, &dataplanev1alpha1.AgentRequest{
		Id: 9,
		Payload: &dataplanev1alpha1.AgentRequest_Resize{
			Resize: &dataplanev1alpha1.ResizePTY{Rows: 40, Cols: 100},
		},
	})

	res := readExec(t, conn, 9)
	assert.Contains(t, string(res.stdout), "40 100",
		"at least one stty size report should carry the new dims")
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func TestStreamChunkCarriesPID(t *testing.T) {
	requireBinary(t, "/bin/sh")
	conn := dialHandle(t, newTestServer(t))

	sendRequest(t, conn, execRequest(12, &dataplanev1alpha1.ExecProcess{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo hi; echo err >&2"},
	}))
	res := readExec(t, conn, 12)

	assert.Positive(t, res.pid, "StreamChunk.pid must be populated for live procs")
	assert.True(t, res.stdoutEOF)
	assert.True(t, res.stderrEOF)
	assert.Equal(t, int32(0), res.result.ExitCode)
}

func TestClientDisconnectCancelsProcess(t *testing.T) {
	requireBinary(t, "sleep")
	srv := newTestServer(t)

	// handle owns the serverConn; we can't use dialHandle here because
	// we need to drive the client close manually during the test body.
	clientConn, serverConn := net.Pipe()
	handleDone := make(chan struct{})
	go func() {
		defer close(handleDone)
		srv.handle(t.Context(), serverConn)
	}()
	t.Cleanup(func() {
		_ = clientConn.Close()
		<-handleDone
	})

	// sleep keeps running until signalled; we learn its pid from the
	// first StreamChunk we read (which the server emits for the EOF
	// marker even though sleep writes no bytes).
	sendRequest(t, clientConn, execRequest(10, &dataplanev1alpha1.ExecProcess{
		Command: "sleep",
		Args:    []string{"60"},
	}))
	waitForStdin(t, srv, 10)

	pid := streamPID(t, srv, 10)
	require.NoError(t, clientConn.Close(), "disconnect")

	// Handle observes the closed conn, per-conn ctx cancels, and
	// CommandContext sends SIGKILL to sleep.
	require.Eventually(t, func() bool {
		err := syscall.Kill(pid, 0)
		return errors.Is(err, syscall.ESRCH)
	}, 5*time.Second, 50*time.Millisecond, "process %d did not die", pid)
}

// streamPID reads the proc's host-visible pid directly off the server
// state — avoids relying on the child to print its own pid.
func streamPID(t *testing.T, srv *server, id uint64) int {
	t.Helper()
	v, ok := srv.procs.Load(id)
	require.True(t, ok, "proc %d not registered", id)
	p := v.(*proc)
	p.mu.Lock()
	defer p.mu.Unlock()
	require.NotZero(t, p.pid, "proc %d has no pid yet", id)
	return int(p.pid)
}

// --- serve / listener lifecycle ------------------------------------------

func TestServeShutdownOnCtxCancel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := newTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.serve(ctx, listener) }()

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return within 2s of ctx cancel")
	}
	// Listener must be closed so reusing the port doesn't race.
	assert.Error(t, listener.Close(), "listener should already be closed by serve's watcher")
}

func TestServeAcceptsMultipleConnections(t *testing.T) {
	requireBinary(t, "/bin/sh")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	srv := newTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() { _ = srv.serve(ctx, listener) }()

	runOne := func(id uint64, expected string, wg *sync.WaitGroup) {
		defer wg.Done()
		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()
		sendRequest(t, conn, execRequest(id, &dataplanev1alpha1.ExecProcess{
			Command: "/bin/sh",
			Args:    []string{"-c", "printf " + expected},
		}))
		res := readExec(t, conn, id)
		assert.Equal(t, expected, string(res.stdout))
		assert.Equal(t, int32(0), res.result.ExitCode)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go runOne(100, "one", &wg)
	go runOne(200, "two", &wg)
	wg.Wait()
}

// --- upload / download ---------------------------------------------------

// uploadResult / downloadResult capture either the success payload or
// the terminal error Status for an upload/download — the new envelope
// carries one or the other, never both.
type uploadResult struct {
	resp      *dataplanev1alpha1.UploadFileResponse
	errStatus *status.Status
}

type downloadResult struct {
	resp      *dataplanev1alpha1.DownloadFileResponse
	errStatus *status.Status
}

func readUploadResult(t *testing.T, conn net.Conn, id uint64) uploadResult {
	t.Helper()
	resp := readResponse(t, conn)
	require.Equal(t, id, resp.Id)
	switch p := resp.Payload.(type) {
	case *dataplanev1alpha1.AgentResponse_UploadFile:
		return uploadResult{resp: p.UploadFile}
	case *dataplanev1alpha1.AgentResponse_Error:
		return uploadResult{errStatus: p.Error}
	default:
		t.Fatalf("unexpected payload %T", resp.Payload)
		return uploadResult{}
	}
}

func readDownloadResult(t *testing.T, conn net.Conn, id uint64) downloadResult {
	t.Helper()
	resp := readResponse(t, conn)
	require.Equal(t, id, resp.Id)
	switch p := resp.Payload.(type) {
	case *dataplanev1alpha1.AgentResponse_DownloadFile:
		return downloadResult{resp: p.DownloadFile}
	case *dataplanev1alpha1.AgentResponse_Error:
		return downloadResult{errStatus: p.Error}
	default:
		t.Fatalf("unexpected payload %T", resp.Payload)
		return downloadResult{}
	}
}

func uploadRequest(id uint64, dest string, data []byte) *dataplanev1alpha1.AgentRequest {
	return &dataplanev1alpha1.AgentRequest{
		Id: id,
		Payload: &dataplanev1alpha1.AgentRequest_UploadFile{
			UploadFile: &dataplanev1alpha1.UploadFileRequest{
				SandboxId: "sb",
				Source:    data,
				Dest:      dest,
			},
		},
	}
}

func downloadRequest(id uint64, source string) *dataplanev1alpha1.AgentRequest {
	return &dataplanev1alpha1.AgentRequest{
		Id: id,
		Payload: &dataplanev1alpha1.AgentRequest_DownloadFile{
			DownloadFile: &dataplanev1alpha1.DownloadFileRequest{
				SandboxId: "sb",
				Source:    source,
			},
		},
	}
}

func TestUploadFileSucceeds(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()
	dest := filepath.Join(dir, "payload.txt")
	body := []byte("hello from the host\n")

	sendRequest(t, conn, uploadRequest(20, dest, body))
	got := readUploadResult(t, conn, 20)
	assert.Nil(t, got.errStatus)
	require.NotNil(t, got.resp)

	data, err := os.ReadFile(dest) // #nosec G304 -- t.TempDir path
	require.NoError(t, err)
	assert.Equal(t, body, data)

	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, uploadFileMode, info.Mode().Perm(),
		"uploaded file should have the documented default mode")
}

func TestUploadFileOverwritesExisting(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()
	dest := filepath.Join(dir, "existing.txt")
	require.NoError(t, os.WriteFile(dest, []byte("original"), 0o600))

	sendRequest(t, conn, uploadRequest(21, dest, []byte("replacement")))
	result := readUploadResult(t, conn, 21)
	assert.Nil(t, result.errStatus)
	require.NotNil(t, result.resp)

	data, err := os.ReadFile(dest) // #nosec G304 -- t.TempDir path
	require.NoError(t, err)
	assert.Equal(t, "replacement", string(data))
}

func TestUploadFileMissingParent(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	// Parent directory is explicitly not created.
	dest := filepath.Join(t.TempDir(), "nowhere", "file.txt")

	sendRequest(t, conn, uploadRequest(22, dest, []byte("x")))
	result := readUploadResult(t, conn, 22)
	require.NotNil(t, result.errStatus)
	assert.Equal(t, int32(codes.NotFound), result.errStatus.Code,
		"ENOENT on parent dir should map to NOT_FOUND")
	assert.Contains(t, result.errStatus.Message, "parent directory",
		"missing parent should produce a descriptive error")

	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "failed upload must not create the file")
}

func TestUploadFileParentIsFile(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()
	// "parent" is a regular file, not a directory.
	parentAsFile := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(parentAsFile, []byte(""), 0o600))
	dest := filepath.Join(parentAsFile, "child.txt")

	sendRequest(t, conn, uploadRequest(23, dest, []byte("x")))
	result := readUploadResult(t, conn, 23)
	require.NotNil(t, result.errStatus)
	assert.Equal(t, int32(codes.FailedPrecondition), result.errStatus.Code,
		"non-directory parent should map to FAILED_PRECONDITION")
	assert.Contains(t, result.errStatus.Message, "not a directory")
}

func TestUploadFileLeavesNoTempOnFailure(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()
	// Make the destination directory read-only so CreateTemp fails.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	sendRequest(t, conn, uploadRequest(24, filepath.Join(dir, "x.txt"), []byte("payload")))
	result := readUploadResult(t, conn, 24)
	require.NotNil(t, result.errStatus)
	assert.NotEmpty(t, result.errStatus.Message)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "failed upload must not leave a temp file behind")
}

func TestDownloadFileSucceeds(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()
	src := filepath.Join(dir, "in.txt")
	want := []byte("download me\n")
	require.NoError(t, os.WriteFile(src, want, 0o600))

	sendRequest(t, conn, downloadRequest(25, src))
	result := readDownloadResult(t, conn, 25)
	assert.Nil(t, result.errStatus)
	require.NotNil(t, result.resp)
	assert.Equal(t, want, result.resp.FileContent)
}

func TestDownloadFileMissing(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	missing := filepath.Join(t.TempDir(), "nope.txt")

	sendRequest(t, conn, downloadRequest(26, missing))
	result := readDownloadResult(t, conn, 26)
	require.NotNil(t, result.errStatus)
	assert.Equal(t, int32(codes.NotFound), result.errStatus.Code,
		"ENOENT should map to NOT_FOUND")
	assert.Contains(t, result.errStatus.Message, "stat",
		"a missing file should surface as a stat error")
}

func TestDownloadFileIsDirectory(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	dir := t.TempDir()

	sendRequest(t, conn, downloadRequest(27, dir))
	result := readDownloadResult(t, conn, 27)
	require.NotNil(t, result.errStatus)
	assert.Equal(t, int32(codes.FailedPrecondition), result.errStatus.Code,
		"downloading a directory should map to FAILED_PRECONDITION")
	assert.Contains(t, result.errStatus.Message, "directory")
}

func TestDownloadFileTooLarge(t *testing.T) {
	conn := dialHandle(t, newTestServer(t))
	src := filepath.Join(t.TempDir(), "big.bin")
	// Sparse file is cheap — Truncate sets the reported size without
	// allocating the bytes on disk. readFileCapped consults
	// os.Stat().Size() so the guard fires before any read.
	f, err := os.Create(src) // #nosec G304 -- t.TempDir path
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxDownloadFileSize+1))
	require.NoError(t, f.Close())

	sendRequest(t, conn, downloadRequest(28, src))
	result := readDownloadResult(t, conn, 28)
	require.NotNil(t, result.errStatus)
	assert.Equal(t, int32(codes.ResourceExhausted), result.errStatus.Code,
		"oversize download should map to RESOURCE_EXHAUSTED")
	assert.Contains(t, result.errStatus.Message, "too large")
}
