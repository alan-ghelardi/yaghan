package exec_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	dpmocks "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cli"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/exec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// --- streaming fake ------------------------------------------------------

// fakeExecStream is a hand-rolled bidi-stream stand-in for tests. The
// generated grpc.BidiStreamingClient is a generic interface gomock
// can't auto-mock for arbitrary type parameters, so we satisfy it
// here with deterministic queues the test fully controls.
type fakeExecStream struct {
	ctx context.Context

	mu       sync.Mutex
	sent     []*dataplanev1alpha1.ExecRequest
	closeErr error
	closed   atomic.Bool

	// recvCh delivers responses (or terminal errors / io.EOF) in
	// order. Tests buffer the whole script up front and then close the
	// channel to signal "no more frames" — at which point Recv returns
	// io.EOF, mirroring real gRPC behaviour.
	recvCh chan recvFrame
}

type recvFrame struct {
	resp *dataplanev1alpha1.ExecResponse
	err  error
}

func newFakeExecStream(ctx context.Context, bufferedFrames int) *fakeExecStream {
	return &fakeExecStream{
		ctx:    ctx,
		recvCh: make(chan recvFrame, bufferedFrames),
	}
}

func (f *fakeExecStream) Send(req *dataplanev1alpha1.ExecRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, req)
	return nil
}

func (f *fakeExecStream) Recv() (*dataplanev1alpha1.ExecResponse, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case frame, ok := <-f.recvCh:
		if !ok {
			return nil, io.EOF
		}
		return frame.resp, frame.err
	}
}

func (f *fakeExecStream) CloseSend() error {
	f.closed.Store(true)
	return f.closeErr
}

func (f *fakeExecStream) Context() context.Context     { return f.ctx }
func (f *fakeExecStream) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeExecStream) Trailer() metadata.MD         { return nil }
func (f *fakeExecStream) SendMsg(any) error            { return nil }
func (f *fakeExecStream) RecvMsg(any) error            { return nil }

// sentFrames returns a defensive copy of the sent slice for assertions.
func (f *fakeExecStream) sentFrames() []*dataplanev1alpha1.ExecRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*dataplanev1alpha1.ExecRequest, len(f.sent))
	copy(out, f.sent)
	return out
}

// emit pushes a non-error response.
func (f *fakeExecStream) emit(resp *dataplanev1alpha1.ExecResponse) {
	f.recvCh <- recvFrame{resp: resp}
}

// emitErr pushes a terminal error (Recv returns this then io.EOF).
func (f *fakeExecStream) emitErr(err error) {
	f.recvCh <- recvFrame{err: err}
}

func (f *fakeExecStream) closeRecv() { close(f.recvCh) }

// Compile-time assertion: fakeExecStream satisfies the bidi interface
// the daemon client returns, with the right type parameters.
var _ grpc.BidiStreamingClient[dataplanev1alpha1.ExecRequest, dataplanev1alpha1.ExecResponse] = (*fakeExecStream)(nil)

// --- helpers -------------------------------------------------------------

// streamChunk builds a StreamChunk ExecResponse.
func streamChunk(streamType dataplanev1alpha1.StreamChunk_StreamType, data string) *dataplanev1alpha1.ExecResponse {
	return &dataplanev1alpha1.ExecResponse{
		Payload: &dataplanev1alpha1.ExecResponse_StreamChunk{
			StreamChunk: &dataplanev1alpha1.StreamChunk{
				Stream: streamType,
				Data:   []byte(data),
			},
		},
	}
}

// processResult builds a ProcessResult ExecResponse.
func processResult(code int32) *dataplanev1alpha1.ExecResponse {
	return &dataplanev1alpha1.ExecResponse{
		Payload: &dataplanev1alpha1.ExecResponse_ProcessResult{
			ProcessResult: &dataplanev1alpha1.ProcessResult{ExitCode: code},
		},
	}
}

// runCmd builds the exec command, injects a stream the test controls,
// runs Execute and returns the resulting error.
func runCmd(t *testing.T, cmdCtx *cli.Context, fake *fakeExecStream, args string) error {
	t.Helper()
	daemonMock := cmdCtx.ClientSet.DaemonService.(*dpmocks.MockDaemonServiceClient)
	if fake != nil {
		daemonMock.EXPECT().
			Exec(gomock.Any()).
			Return(fake, nil).
			Times(1)
	}

	cmd := exec.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(cmdCtx.IOStreams.Stdout)
	cmd.SetErr(cmdCtx.IOStreams.Stderr)
	cmd.SetArgs(splitArgs(args))
	return cmd.Execute()
}

func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}

// --- happy paths ---------------------------------------------------------

func TestExec_HappyPath_NoFlags(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 4)
	fake.emit(streamChunk(dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT, "hello\n"))
	fake.emit(processResult(0))
	fake.closeRecv()

	require.NoError(t, runCmd(t, cmdCtx, fake, "sb-1 -- echo hello"))

	assert.Equal(t, "hello\n",
		clitesting.Read(t, cmdCtx.IOStreams.Stdout),
		"stdout chunks must reach IOStreams.Stdout")
	assert.True(t, fake.closed.Load(),
		"non-interactive run must CloseSend immediately so the agent stdin drains")

	sent := fake.sentFrames()
	require.Len(t, sent, 1, "non-interactive run must send only the ExecProcess frame")
	first := sent[0].GetExecProcess()
	require.NotNil(t, first)
	assert.Equal(t, "sb-1", sent[0].GetSandboxId())
	assert.Equal(t, "echo", first.GetCommand())
	assert.Equal(t, []string{"hello"}, first.GetArgs())
	assert.Empty(t, first.GetEnv())
	assert.Empty(t, first.GetCwd())
	assert.False(t, first.GetTty())
}

func TestExec_StderrIsDemuxed(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 4)
	fake.emit(streamChunk(dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT, "out"))
	fake.emit(streamChunk(dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDERR, "err"))
	fake.emit(processResult(0))
	fake.closeRecv()

	require.NoError(t, runCmd(t, cmdCtx, fake, "sb-1 -- whatever"))

	assert.Equal(t, "out", clitesting.Read(t, cmdCtx.IOStreams.Stdout))
	assert.Equal(t, "err", clitesting.Read(t, cmdCtx.IOStreams.Stderr))
}

func TestExec_NonZeroExitSurfacesAsExitCodeError(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 2)
	fake.emit(processResult(7))
	fake.closeRecv()

	err := runCmd(t, cmdCtx, fake, "sb-1 -- false")

	require.Error(t, err)
	var ec *cli.ExitCodeError
	require.ErrorAs(t, err, &ec, "must surface as *cli.ExitCodeError")
	assert.Equal(t, 7, ec.Code)
}

func TestExec_ZeroExitDoesNotProduceExitCodeError(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 2)
	fake.emit(processResult(0))
	fake.closeRecv()

	err := runCmd(t, cmdCtx, fake, "sb-1 -- true")

	require.NoError(t, err)
}

// --- env -----------------------------------------------------------------

func TestExec_EnvIsForwarded(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 2)
	fake.emit(processResult(0))
	fake.closeRecv()

	require.NoError(t, runCmd(t, cmdCtx, fake,
		"sb-1 -e FOO=bar -e BAZ=qux=more -- env"))

	first := fake.sentFrames()[0].GetExecProcess()
	assert.Equal(t, map[string]string{
		"FOO": "bar",
		"BAZ": "qux=more", // values are split on the FIRST '=' only
	}, first.GetEnv())
}

func TestExec_EnvValidationRejectsBadEntries(t *testing.T) {
	tests := []struct {
		name        string
		entry       string
		errContains string
	}{
		{"missing equals", "KEY", "must be KEY=VALUE"},
		{"empty key", "=val", "must not be empty"},
		{"digit-leading key", "1KEY=v", `must match`},
		{"illegal char in key", "K-Y=v", `must match`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)

			// No EXPECT() — Exec must NOT be invoked when validation fails
			// upstream. gomock fails the test if it is.
			cmd := exec.New(cmdCtx)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(cmdCtx.IOStreams.Stdout)
			cmd.SetErr(cmdCtx.IOStreams.Stderr)
			cmd.SetArgs([]string{"sb-1", "-e", tc.entry, "--", "echo"})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

// --- cwd / args ----------------------------------------------------------

func TestExec_CWDAndArgsForwarded(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	fake := newFakeExecStream(t.Context(), 2)
	fake.emit(processResult(0))
	fake.closeRecv()

	require.NoError(t, runCmd(t, cmdCtx, fake,
		"sb-1 --cwd /workspace -- go build ./..."))

	proc := fake.sentFrames()[0].GetExecProcess()
	assert.Equal(t, "/workspace", proc.GetCwd())
	assert.Equal(t, "go", proc.GetCommand())
	assert.Equal(t, []string{"build", "./..."}, proc.GetArgs())
}

// --- stdin forwarding ----------------------------------------------------

// withStdin replaces the IOStreams.Stdin reader on cmdCtx.
func withStdin(cmdCtx *cli.Context, content string) {
	cmdCtx.IOStreams.Stdin = strings.NewReader(content)
}

func TestExec_InteractiveForwardsStdin(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	withStdin(cmdCtx, "hi")

	fake := newFakeExecStream(t.Context(), 4)
	// Block the receiver until the stdin pump has had a chance to push
	// its frames; otherwise the test is racy: the receiver could close
	// before any Stdin frames arrive.
	stdinSeen := make(chan struct{})
	go func() {
		// Poll the sent slice for the terminal Stdin{Eof} frame.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, f := range fake.sentFrames() {
				if s := f.GetStdin(); s != nil && s.GetEof() {
					close(stdinSeen)
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		// Timed out — close anyway so the test doesn't hang forever.
		close(stdinSeen)
	}()

	go func() {
		<-stdinSeen
		fake.emit(processResult(0))
		fake.closeRecv()
	}()

	require.NoError(t, runCmd(t, cmdCtx, fake, "sb-1 -i -- cat"))

	stdinFrames := []*dataplanev1alpha1.StdinChunk{}
	for _, f := range fake.sentFrames() {
		if s := f.GetStdin(); s != nil {
			stdinFrames = append(stdinFrames, s)
		}
	}
	require.GreaterOrEqual(t, len(stdinFrames), 1,
		"interactive run must forward at least one Stdin frame")
	// Concatenate the data frames; the terminal frame carries Eof:true.
	var data strings.Builder
	var sawEOF bool
	for _, s := range stdinFrames {
		data.Write(s.GetData())
		if s.GetEof() {
			sawEOF = true
		}
	}
	assert.Equal(t, "hi", data.String())
	assert.True(t, sawEOF, "the last Stdin frame must carry Eof:true")
	assert.True(t, fake.closed.Load(),
		"the stdin pump must CloseSend after forwarding EOF")
}

func TestExec_TTYImpliesInteractive(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	withStdin(cmdCtx, "x")

	fake := newFakeExecStream(t.Context(), 4)

	// Same gating dance as the previous test: wait for the Stdin EOF
	// frame before unblocking the receiver.
	stdinSeen := make(chan struct{})
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, f := range fake.sentFrames() {
				if s := f.GetStdin(); s != nil && s.GetEof() {
					close(stdinSeen)
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(stdinSeen)
	}()
	go func() {
		<-stdinSeen
		fake.emit(processResult(0))
		fake.closeRecv()
	}()

	// -t alone, no -i.
	require.NoError(t, runCmd(t, cmdCtx, fake, "sb-1 -t -- /bin/sh"))

	first := fake.sentFrames()[0].GetExecProcess()
	require.NotNil(t, first)
	assert.True(t, first.GetTty(), "ExecProcess.Tty must be set when -t is supplied")

	// Verify stdin forwarding actually fired despite -i not being set
	// — the implication should have promoted us into the pump path.
	hasStdin := false
	for _, f := range fake.sentFrames() {
		if f.GetStdin() != nil {
			hasStdin = true
			break
		}
	}
	assert.True(t, hasStdin, "-t must imply -i and produce Stdin frames")
}

// --- argument validation -------------------------------------------------

func TestExec_RequiresDoubleDashSeparator(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)

	cmd := exec.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"sb-1", "ls"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be the only argument before `--`")
}

func TestExec_RequiresCommandAfterDash(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)

	cmd := exec.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"sb-1", "--"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command to run must be supplied")
}

func TestExec_RequiresSandboxIdBeforeDash(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)

	cmd := exec.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	// Two positionals before `--` — the first is supposed to be the
	// only one (the sandbox id).
	cmd.SetArgs([]string{"sb-1", "extra", "--", "ls"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be the only argument before `--`")
}

// --- daemon errors -------------------------------------------------------

func TestExec_DaemonStreamErrorPropagates(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)

	fake := newFakeExecStream(t.Context(), 2)
	fake.emitErr(status.Error(codes.NotFound, "no such sandbox"))
	fake.closeRecv()

	err := runCmd(t, cmdCtx, fake, "sb-missing -- ls")

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a gRPC status: %v", err)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestExec_AgentClosesWithoutResult(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)

	fake := newFakeExecStream(t.Context(), 2)
	fake.emit(streamChunk(dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT, "partial"))
	fake.closeRecv() // EOF without a ProcessResult

	err := runCmd(t, cmdCtx, fake, "sb-1 -- ls")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not return a process result")
}

// --- resize --------------------------------------------------------------

func TestSendResize_ForwardsFrame(t *testing.T) {
	fake := newFakeExecStream(t.Context(), 1)

	require.NoError(t, exec.SendResizeForTest(fake, "sb-1", 120, 40))

	sent := fake.sentFrames()
	require.Len(t, sent, 1)
	assert.Equal(t, "sb-1", sent[0].GetSandboxId())
	resize := sent[0].GetResize()
	require.NotNil(t, resize, "frame must carry a Resize payload")
	assert.Equal(t, uint32(120), resize.GetCols())
	assert.Equal(t, uint32(40), resize.GetRows())
}

// Note: TestExec_TTYSendsInitialResizeWhenTerminal is intentionally
// omitted. The initial-resize and SIGWINCH-watcher paths live behind
// the terminalFD guard, which requires stdin to be a real *os.File on
// a TTY. Test stdin is a strings.Reader, so those branches are
// unreachable here — same reason raw-mode toggling is untested. The
// underlying frame translation is covered by TestSendResize_ForwardsFrame
// above, and the daemon-side forwarding by TestExec_ForwardsResizeFrames.

// --- Exec dial-time error -----------------------------------------------

func TestExec_DialErrorPropagates(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	daemonMock := cmdCtx.ClientSet.DaemonService.(*dpmocks.MockDaemonServiceClient)

	dialErr := errors.New("daemon unavailable")
	daemonMock.EXPECT().
		Exec(gomock.Any()).
		Return(nil, dialErr).
		Times(1)

	cmd := exec.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"sb-1", "--", "ls"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, dialErr)
}
