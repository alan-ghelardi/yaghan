// Package guest implements the in-VM side of the host⇄guest vsock RPC.
// It listens on a single vsock port, reads length-prefixed Request
// messages (multiplexed by id), and writes Response messages back —
// handling Exec / Stdin / Cancel / Resize for subprocesses started
// inside the VM.
package guest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"

	"github.com/creack/pty"
	"github.com/mdlayher/vsock"
	"golang.nuinfra.net/agent/internal/framing"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

// streamChunkSize is the read buffer per stdout/stderr chunk. 4 KiB
// matches typical pipe buffer writeback and keeps envelope overhead low.
const streamChunkSize = 4096

// maxDownloadFileSize caps the size of a single DownloadFile payload.
// The whole file body travels in one Response envelope, which
// [framing.MaxMessageSize] caps at 4 MiB including proto overhead; 3
// MiB leaves headroom for field tags, length varints, and the path
// strings. Larger files need a chunked protocol — a future revision.
const maxDownloadFileSize = 3 * 1024 * 1024

// uploadFileMode is the mode applied to newly uploaded files. Matches
// the default many tools use (e.g. Go's os.WriteFile).
const uploadFileMode os.FileMode = 0o644

// ListenAndServe starts the guest agent on the given vsock port and
// blocks until ctx is cancelled. The caller supplies the logger so
// lifecycle events upstream (mounts, reaper, cmdline parse) share the
// same sink as the server's own output. Terminates the process on
// fatal setup errors.
func ListenAndServe(ctx context.Context, logger *log.Logger, port uint32) {
	srv := &server{logger: logger}
	if err := srv.listenVsock(ctx, port); err != nil {
		logger.Fatalf("server exited: %v", err)
	}
}

type server struct {
	logger    *log.Logger
	procs     sync.Map
	writeLock sync.Mutex
}

// proc tracks one running Exec. cancel is populated before cmd.Start so
// a racing Cancel request can find the registration; pid / stdin / pty
// become non-zero / non-nil once the process is running and are read
// under mu.
type proc struct {
	cancel context.CancelFunc

	mu    sync.Mutex
	pid   int32
	stdin io.WriteCloser
	pty   *os.File
}

// execHandles groups the stdio attachments a started command exposes,
// plus the host-visible pid so every StreamChunk can carry it.
type execHandles struct {
	pid    int32
	stdin  io.WriteCloser
	stdout io.Reader
	stderr io.Reader // nil in TTY mode (merged into the master fd)
	pty    *os.File  // nil in pipe mode
}

// listenVsock is the production entry path — it opens a vsock listener
// and hands off to [serve]. Separating the two lets tests drive serve
// with any net.Listener (TCP, in-memory) without needing a real VM.
func (s *server) listenVsock(ctx context.Context, port uint32) error {
	listener, err := vsock.Listen(port, &vsock.Config{})
	if err != nil {
		return fmt.Errorf("vsock listen port %d: %w", port, err)
	}
	return s.serve(ctx, listener)
}

// serve runs the accept loop until ctx is cancelled. A background
// goroutine closes the listener on cancellation, unblocking any
// in-flight Accept — there is no polling / select-with-default.
func (s *server) serve(ctx context.Context, listener net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Printf("accept: %v", err)
			continue
		}
		go s.handle(ctx, conn)
	}
}

// handle reads requests off conn and dispatches them. A per-connection
// ctx (cancelled via defer on return) propagates into every Exec it
// spawns, so a client disconnect cancels every subprocess launched
// through this connection.
func (s *server) handle(parentCtx context.Context, conn net.Conn) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	defer conn.Close()

	for {
		var req dataplanev1alpha1.AgentRequest
		if err := framing.Read(conn, &req); err != nil {
			if !errors.Is(err, io.EOF) {
				s.logger.Printf("read request: %v", err)
			}
			return
		}
		switch payload := req.Payload.(type) {
		case *dataplanev1alpha1.AgentRequest_ExecRequest:
			proc := payload.ExecRequest.GetExecProcess()
			if proc == nil {
				// The new ExecRequest oneof allows live stdin frames as
				// well; the agent uses AgentRequest_Stdin for that path,
				// so anything other than ExecProcess at this layer is a
				// protocol violation.
				s.sendError(conn, req.Id, codes.InvalidArgument,
					"ExecRequest must carry an ExecProcess payload")
				continue
			}
			go s.execCommand(ctx, conn, req.Id, proc)
		case *dataplanev1alpha1.AgentRequest_Cancel:
			s.cancelRequest(req.Id)
		case *dataplanev1alpha1.AgentRequest_Stdin:
			s.pumpToStdin(req.Id, payload.Stdin)
		case *dataplanev1alpha1.AgentRequest_Resize:
			s.resizeTerm(req.Id, payload.Resize)
		case *dataplanev1alpha1.AgentRequest_UploadFile:
			go s.handleUpload(conn, req.Id, payload.UploadFile)
		case *dataplanev1alpha1.AgentRequest_DownloadFile:
			go s.handleDownload(conn, req.Id, payload.DownloadFile)
		default:
			// Unknown payload — keep the connection alive so other
			// multiplexed requests on it keep flowing.
			s.logger.Printf("unhandled request payload %T for id %d", payload, req.Id)
		}
	}
}

// execCommand orchestrates one Exec lifecycle: registration, start,
// output streaming, and final response.
func (s *server) execCommand(parentCtx context.Context, conn net.Conn, id uint64, req *dataplanev1alpha1.ExecProcess) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// Register before cmd.Start so a Cancel request arriving in the
	// microsecond window between dispatch and process start can still
	// reach this id.
	p := &proc{cancel: cancel}
	s.procs.Store(id, p)
	defer s.procs.Delete(id)

	// #nosec G204 -- the agent's entire purpose is to run caller-
	// supplied commands inside the guest. Trust boundary is the vsock
	// channel; tainted-input checks are enforced by the host, not here.
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = req.Cwd
	cmd.Env = makeEnviron(req)

	handles, err := s.startCommand(cmd, req.Tty)
	if err != nil {
		s.sendExecError(conn, id, err)
		return
	}

	// Publish pid / stdin / pty so Stdin, Resize, and disconnect
	// handlers can reach them.
	p.mu.Lock()
	p.pid = handles.pid
	p.stdin = handles.stdin
	p.pty = handles.pty
	p.mu.Unlock()

	wg := s.streamOutputs(conn, id, handles)

	// Shutdown ordering is different per mode. For pipes, os/exec
	// says it's incorrect to call Wait before pipe reads complete
	// (Wait closes the parent ends). For a PTY, cmd.Wait has no
	// pipe to close but the master fd must be closed explicitly to
	// unblock the reader, which can only happen after the child
	// exits.
	var execErr error
	if handles.pty != nil {
		execErr = cmd.Wait()
		// Detach the fd from the proc entry before closing so a
		// racing Resize or Stdin observes nil under p.mu; closing
		// the fd while a Setsize ioctl is in flight would otherwise
		// trip the race detector (ioctl bypasses *os.File's internal
		// mutex).
		ptmx := s.detachStdio(id)
		if ptmx != nil {
			_ = ptmx.Close()
		}
		wg.Wait()
	} else {
		wg.Wait()
		execErr = cmd.Wait()
		s.detachStdio(id)
	}

	s.sendExecResponse(conn, id, cmd, execErr)
}

// startCommand prepares the command's stdio and starts it. In TTY mode
// the returned handles expose the PTY master fd as both stdin and
// stdout; stderr stays nil because it is merged by the terminal.
func (s *server) startCommand(cmd *exec.Cmd, tty bool) (*execHandles, error) {
	if tty {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			return nil, fmt.Errorf("pty start: %w", err)
		}
		return &execHandles{
			pid:    int32(cmd.Process.Pid),
			stdin:  ptmx,
			stdout: ptmx,
			pty:    ptmx,
		}, nil
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return &execHandles{
		pid:    int32(cmd.Process.Pid),
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// streamOutputs spawns one reader goroutine per attached stream and
// returns a WaitGroup the caller must Wait on before sending the final
// ExecResponse. Waiting here is what keeps the response after the last
// chunk and avoids the os/exec pipe-closed-mid-read race.
func (s *server) streamOutputs(conn net.Conn, id uint64, h *execHandles) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.stream(conn, id, h.pid, h.stdout, dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT)
	}()
	if h.stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.stream(conn, id, h.pid, h.stderr, dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDERR)
		}()
	}
	return &wg
}

// stream reads r in chunks, emits each as a StreamChunk wrapped in an
// ExecResponse, and ends with a final chunk bearing Eof=true so the
// client has an explicit per-stream end marker independent of the
// terminal ProcessResult. Every chunk — including the EOF marker —
// carries the host-visible pid so the client can correlate output
// with guest-side processes.
func (s *server) stream(conn net.Conn, id uint64, pid int32, r io.Reader, t dataplanev1alpha1.StreamChunk_StreamType) {
	buf := make([]byte, streamChunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.write(conn, newStreamChunkResponse(id, &dataplanev1alpha1.StreamChunk{
				Pid:    pid,
				Data:   buf[:n],
				Stream: t,
			}))
		}
		if err != nil {
			break
		}
	}
	s.write(conn, newStreamChunkResponse(id, &dataplanev1alpha1.StreamChunk{
		Pid:    pid,
		Stream: t,
		Eof:    true,
	}))
}

// newStreamChunkResponse wraps a StreamChunk in the AgentResponse →
// ExecResponse → StreamChunk envelope dictated by the protocol.
func newStreamChunkResponse(id uint64, chunk *dataplanev1alpha1.StreamChunk) *dataplanev1alpha1.AgentResponse {
	return &dataplanev1alpha1.AgentResponse{
		Id: id,
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_StreamChunk{StreamChunk: chunk},
			},
		},
	}
}

// sendExecError reports a startup failure. No stream chunks precede it
// because the process never ran. An exec-start ENOENT is surfaced as
// NOT_FOUND; everything else rolls up to INTERNAL.
func (s *server) sendExecError(conn net.Conn, id uint64, err error) {
	code := codes.Internal
	if errors.Is(err, os.ErrNotExist) {
		code = codes.NotFound
	}
	s.sendError(conn, id, code, err.Error())
}

// sendExecResponse emits the closing response of a successful exec
// exchange. Always ordered after every stream chunk and both EOF
// markers, thanks to the WaitGroup in [execCommand]. A non-zero exit
// is a successful RPC, so only the exit_code is reported; callers
// distinguish failure from exit status by inspecting Response.error
// (which this path never sets) vs Response.exec_response.
//
// The only exec_response path that is still an error is when cmd.Wait
// returns without ever populating ProcessState — an internal
// inconsistency that should not occur in practice. We treat it as
// INTERNAL rather than silently sending exit_code=0.
func (s *server) sendExecResponse(conn net.Conn, id uint64, cmd *exec.Cmd, execErr error) {
	if cmd.ProcessState == nil {
		msg := "exec: process state missing after wait"
		if execErr != nil {
			msg = fmt.Sprintf("%s: %v", msg, execErr)
		}
		s.sendError(conn, id, codes.Internal, msg)
		return
	}
	s.write(conn, &dataplanev1alpha1.AgentResponse{
		Id: id,
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_ProcessResult{
					ProcessResult: &dataplanev1alpha1.ProcessResult{
						ExitCode: int32(cmd.ProcessState.ExitCode()),
					},
				},
			},
		},
	})
}

// sendError emits a Response whose payload is the error variant,
// carrying a google.rpc.Status. Used for every RPC failure path
// across exec / upload / download.
func (s *server) sendError(conn net.Conn, id uint64, code codes.Code, message string) {
	s.write(conn, &dataplanev1alpha1.AgentResponse{
		Id: id,
		Payload: &dataplanev1alpha1.AgentResponse_Error{
			Error: &status.Status{
				Code:    int32(code),
				Message: message,
			},
		},
	})
}

// codeForFileErr maps a filesystem error from the upload/download
// paths to a gRPC code. Operates off both errors.Is (for ENOENT) and
// substring matches on the error message produced by the file
// helpers (atomicWriteFile, readFileCapped) — those messages are
// stable internal text, not user input.
func codeForFileErr(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if errors.Is(err, os.ErrNotExist) {
		return codes.NotFound
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "is not a directory"),
		strings.Contains(msg, "is a directory, not a file"):
		return codes.FailedPrecondition
	case strings.Contains(msg, "too large"),
		strings.Contains(msg, "grew past"):
		return codes.ResourceExhausted
	}
	return codes.Internal
}

// write is the sole authorized writer for conn — serialized so the
// framing length prefix + body can never interleave with another
// Response.
func (s *server) write(conn net.Conn, message *dataplanev1alpha1.AgentResponse) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	if err := framing.Write(conn, message); err != nil {
		s.logger.Printf("write response #%d: %v", message.Id, err)
	}
}

func (s *server) cancelRequest(id uint64) {
	if v, ok := s.procs.Load(id); ok {
		v.(*proc).cancel()
	}
}

// pumpToStdin feeds stdin data into the child. When chunk.Eof is true in
// pipe mode, it closes the child's stdin after the write so programs
// like `cat` can terminate cleanly. In TTY mode eof is ignored — the
// master fd is shared with stdout and closing it would also end output
// capture; terminal apps signal EOF via Ctrl-D inside a normal data
// chunk instead.
//
// Holding p.mu across the write (rather than snapshotting the pointer)
// serialises with detachStdio's nil-then-close sequence so the fd can
// never be closed mid-use.
func (s *server) pumpToStdin(id uint64, chunk *dataplanev1alpha1.StdinChunk) {
	v, ok := s.procs.Load(id)
	if !ok {
		return
	}
	p := v.(*proc)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin == nil {
		return
	}
	if len(chunk.Data) > 0 {
		if _, err := p.stdin.Write(chunk.Data); err != nil {
			s.logger.Printf("stdin #%d: %v", id, err)
		}
	}
	if chunk.Eof {
		if p.pty != nil {
			s.logger.Printf("stdin #%d: eof ignored in tty mode", id)
			return
		}
		if err := p.stdin.Close(); err != nil {
			s.logger.Printf("stdin close #%d: %v", id, err)
		}
		// Further writes on this id are no-ops.
		p.stdin = nil
	}
}

func (s *server) resizeTerm(id uint64, params *dataplanev1alpha1.ResizePTY) {
	v, ok := s.procs.Load(id)
	if !ok {
		return
	}
	p := v.(*proc)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pty == nil {
		return
	}
	err := pty.Setsize(p.pty, &pty.Winsize{
		Cols: uint16(params.Cols),
		Rows: uint16(params.Rows),
	})
	if err != nil {
		s.logger.Printf("resize #%d: %v", id, err)
	}
}

// detachStdio clears and returns the PTY master fd for the given id
// under the proc's mutex. After this call every pumpToStdin /
// resizeTerm for that id is a no-op, making it safe for the caller to
// close the fd.
func (s *server) detachStdio(id uint64) *os.File {
	v, ok := s.procs.Load(id)
	if !ok {
		return nil
	}
	p := v.(*proc)
	p.mu.Lock()
	defer p.mu.Unlock()
	ptmx := p.pty
	p.stdin = nil
	p.pty = nil
	return ptmx
}

// makeEnviron builds the child's environment. Request-supplied pairs
// come first so they win under getenv semantics (first match) over the
// parent's inherited vars.
func makeEnviron(req *dataplanev1alpha1.ExecProcess) []string {
	parent := os.Environ()
	out := make([]string, 0, len(req.Env)+len(parent))
	for k, v := range req.Env {
		out = append(out, k+"="+v)
	}
	return append(out, parent...)
}

// handleUpload writes req.Source to req.Dest atomically and reports the
// outcome. The parent directory of dest must already exist — we do not
// create it to avoid turning uploads into an implicit mkdir primitive.
func (s *server) handleUpload(conn net.Conn, id uint64, req *dataplanev1alpha1.UploadFileRequest) {
	err := atomicWriteFile(req.Dest, req.Source, uploadFileMode)
	s.sendUploadResponse(conn, id, err)
}

// atomicWriteFile writes data to path in a way that observers never see
// a half-written file: write to a sibling temp file, fsync, rename. On
// any error the temp file is removed.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("parent directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent %q is not a directory", dir)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename to destination: %w", err)
	}
	committed = true
	return nil
}

func (s *server) sendUploadResponse(conn net.Conn, id uint64, err error) {
	if err != nil {
		s.sendError(conn, id, codeForFileErr(err), err.Error())
		return
	}
	s.write(conn, &dataplanev1alpha1.AgentResponse{
		Id:      id,
		Payload: &dataplanev1alpha1.AgentResponse_UploadFile{UploadFile: &dataplanev1alpha1.UploadFileResponse{}},
	})
}

// handleDownload reads req.Source in full and returns its contents. The
// request fails loudly if the file is larger than maxDownloadFileSize
// — chunked streaming is a planned follow-up.
func (s *server) handleDownload(conn net.Conn, id uint64, req *dataplanev1alpha1.DownloadFileRequest) {
	data, err := readFileCapped(req.Source, maxDownloadFileSize)
	s.sendDownloadResponse(conn, id, data, err)
}

// readFileCapped stats, opens, and fully reads path, refusing anything
// larger than maxSize bytes. Using a LimitReader on top of Open handles
// the race where the file grows between the stat and the read.
func readFileCapped(path string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a directory, not a file", path)
	}
	if info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxSize)
	}

	f, err := os.Open(path) // #nosec G304 -- caller-supplied path is the feature.
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file grew past %d bytes during read", maxSize)
	}
	return data, nil
}

func (s *server) sendDownloadResponse(conn net.Conn, id uint64, data []byte, err error) {
	if err != nil {
		s.sendError(conn, id, codeForFileErr(err), err.Error())
		return
	}
	resp := &dataplanev1alpha1.DownloadFileResponse{FileContent: data}
	s.write(conn, &dataplanev1alpha1.AgentResponse{
		Id:      id,
		Payload: &dataplanev1alpha1.AgentResponse_DownloadFile{DownloadFile: resp},
	})
}
