// Package exec implements `yag sandbox exec`. It opens a
// bidirectional stream against the daemon's Exec RPC and shuttles
// stdin/stdout/stderr between the local terminal and a guest process,
// forwarding the guest's exit code via cli.ExitCodeError.
package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"

	dataplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/data_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// execStreamSender is the minimal Send surface the resize helpers need.
// Both *grpc.BidiStreamingClient[ExecRequest, ExecResponse] and the
// hand-rolled fakeExecStream in tests satisfy it implicitly.
type execStreamSender interface {
	Send(*dataplanev1alpha1.ExecRequest) error
}

const (
	flagEnv         = "env"
	flagInteractive = "interactive"
	flagTTY         = "tty"
	flagCWD         = "cwd"

	// stdinChunkSize is the granularity at which we cut local stdin
	// into StdinChunk frames. 32 KiB is large enough to amortise the
	// per-frame overhead and small enough that an early ProcessResult
	// from the agent doesn't have to wait on a giant in-flight chunk.
	stdinChunkSize = 32 * 1024
)

// envKeyPattern matches a POSIX-shaped environment variable name.
// Values may be empty or contain '=' (we split on the FIRST '=' only).
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <id> -- <command> [args...]",
		Short: "Run a command inside a sandbox",
		Long: `Run a command inside a running sandbox.

The exec command opens a bidirectional stream against the daemon and
streams stdout/stderr back to your terminal. The CLI exits with the
same status code as the guest command, so it composes with shell
pipelines.

Arguments after the literal '--' separator are passed verbatim to the
command — that lets you forward flags ('-l', '--all', etc.) to the
guest without yag trying to parse them.`,
		Example: `  # Print the guest hostname.
  yag sandbox exec my-sandbox -- hostname

  # Open an interactive shell with a TTY allocated.
  yag sandbox exec my-sandbox -it -- /bin/bash

  # Run a build with extra env and a custom working directory.
  yag sandbox exec my-sandbox -w /workspace -e GOOS=linux -e GOARCH=amd64 -- go build ./...

  # Pipe data on stdin without a TTY.
  cat input.txt | yag sandbox exec my-sandbox -i -- sh -c 'cat > /tmp/out'`,
		Args: validateArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	cmd.Flags().StringSliceP(flagEnv, "e", nil,
		"Environment variable in KEY=VALUE form. Repeatable. Key must "+
			"match [A-Za-z_][A-Za-z0-9_]*; value may be empty or contain '='.")
	cmd.Flags().BoolP(flagInteractive, "i", false,
		"Forward this terminal's stdin to the command. Implied by --tty.")
	cmd.Flags().BoolP(flagTTY, "t", false,
		"Allocate a pseudo-terminal in the sandbox. Implies --interactive. "+
			"Note: window-size negotiation is not yet supported, so the guest TTY "+
			"uses its default geometry; stderr is merged into stdout (POSIX TTY behaviour).")
	cmd.Flags().StringP(flagCWD, "w", "",
		"Working directory for the command inside the sandbox (default: agent's home).")

	return cmd
}

// validateArgs enforces the docker-shaped layout
// `<id> [flags] -- <command> [args...]`. Cobra surfaces ArgsLenAtDash
// as the index where `--` appeared in the original token stream; we
// require it to be exactly 1 (one positional before the dash) and at
// least one positional after.
func validateArgs(cmd *cobra.Command, args []string) error {
	dash := cmd.ArgsLenAtDash()
	if dash != 1 {
		return errors.New(
			"usage: yag sandbox exec <id> [flags] -- <command> [args...]\n" +
				"the sandbox id must be the only argument before `--`")
	}
	if len(args) < 2 {
		return errors.New("a command to run must be supplied after `--`")
	}
	return nil
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := args[0]
	cmdAndArgs := args[1:]

	envEntries, err := cmd.Flags().GetStringSlice(flagEnv)
	if err != nil {
		return err
	}
	envMap, err := parseEnv(envEntries)
	if err != nil {
		return err
	}
	interactive, err := cmd.Flags().GetBool(flagInteractive)
	if err != nil {
		return err
	}
	tty, err := cmd.Flags().GetBool(flagTTY)
	if err != nil {
		return err
	}
	cwd, err := cmd.Flags().GetString(flagCWD)
	if err != nil {
		return err
	}
	// --tty implies --interactive. Without stdin forwarding a PTY is
	// effectively useless.
	if tty {
		interactive = true
	}

	stream, err := ctx.ClientSet.DaemonService.Exec(cmd.Context())
	if err != nil {
		return fmt.Errorf("open exec stream: %w", err)
	}

	// First frame must carry ExecProcess (enforced by the daemon).
	if err := stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: id,
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{
				Command: cmdAndArgs[0],
				Args:    cmdAndArgs[1:],
				Env:     envMap,
				Cwd:     cwd,
				Tty:     tty,
			},
		},
	}); err != nil {
		return fmt.Errorf("send ExecProcess: %w", err)
	}

	// Put the local terminal into raw mode when -t is set AND we
	// actually have a tty on stdin. In tests stdin is a strings.Reader,
	// not a *os.File, so this branch is naturally skipped.
	//
	// While we're here, also send the initial size and install a
	// SIGWINCH watcher so the guest TTY tracks the local terminal as
	// the user resizes their window.
	if tty {
		if fd, isTerm := terminalFD(ctx.IOStreams.Stdin); isTerm {
			if restore, err := makeRaw(fd); err == nil {
				defer restore()
			}
			if cols, rows, err := term.GetSize(fd); err == nil {
				_ = sendResize(stream, id, uint32(cols), uint32(rows))
			}
			watchCtx, cancelWatch := context.WithCancel(cmd.Context())
			defer cancelWatch()
			watchTerminalSize(watchCtx, fd, stream, id)
		}
	}

	// Stdin pump. Spawn only when interactive — otherwise close the
	// send half immediately so commands like `cat` see EOF instead of
	// blocking forever.
	var stdinErrCh chan error
	if interactive {
		stdinErrCh = make(chan error, 1)
		go func() {
			stdinErrCh <- pumpStdin(ctx.IOStreams.Stdin, stream, id)
		}()
	} else {
		_ = stream.CloseSend()
	}

	// Receiver loop. Synchronous; runs in the foreground goroutine so
	// it can drain trailing chunks after ProcessResult before the
	// stream EOFs.
	var exitCode int32
	var sawResult bool
	for {
		resp, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return recvErr
		}
		switch p := resp.GetPayload().(type) {
		case *dataplanev1alpha1.ExecResponse_StreamChunk:
			dst := ctx.IOStreams.Stdout
			if p.StreamChunk.GetStream() == dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDERR {
				dst = ctx.IOStreams.Stderr
			}
			if data := p.StreamChunk.GetData(); len(data) > 0 {
				if _, werr := dst.Write(data); werr != nil {
					return fmt.Errorf("write %s: %w",
						p.StreamChunk.GetStream(), werr)
				}
			}
		case *dataplanev1alpha1.ExecResponse_ProcessResult:
			exitCode = p.ProcessResult.GetExitCode()
			sawResult = true
		}
	}

	// Drain the stdin pump if it was running. A pump error is logged
	// to stderr but doesn't override the guest's exit code: the user
	// cares more about whether their command succeeded than whether
	// stdin forwarding hit a transient hiccup as the agent was
	// shutting down.
	if stdinErrCh != nil {
		if pumpErr := <-stdinErrCh; pumpErr != nil && !errors.Is(pumpErr, io.EOF) {
			fmt.Fprintln(ctx.IOStreams.Stderr,
				"warning: stdin forwarding ended:", pumpErr)
		}
	}

	if !sawResult {
		return errors.New("agent did not return a process result")
	}
	if exitCode != 0 {
		return &cli.ExitCodeError{Code: int(exitCode)}
	}
	return nil
}

// parseEnv turns the slice of "KEY=VALUE" strings into a map. It
// splits on the first '=' so "FOO=bar=baz" yields key=FOO, value=bar=baz.
// Keys must satisfy envKeyPattern; values are unconstrained (including
// empty).
func parseEnv(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid --env %q: must be KEY=VALUE", e)
		}
		key, value := e[:idx], e[idx+1:]
		if key == "" {
			return nil, fmt.Errorf("invalid --env %q: key must not be empty", e)
		}
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf(
				"invalid --env %q: key %q must match [A-Za-z_][A-Za-z0-9_]*",
				e, key)
		}
		out[key] = value
	}
	return out, nil
}

// pumpStdin reads chunks from stdin and forwards them as StdinChunk
// frames until EOF or an error, then sends a terminal Stdin{Eof: true}
// and CloseSends the stream so the agent's stdin pipe drains.
func pumpStdin(
	stdin io.Reader,
	stream interface {
		Send(*dataplanev1alpha1.ExecRequest) error
		CloseSend() error
	},
	sandboxID string,
) error {
	defer func() { _ = stream.CloseSend() }()

	buf := make([]byte, stdinChunkSize)
	for {
		n, err := stdin.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&dataplanev1alpha1.ExecRequest{
				SandboxId: sandboxID,
				Payload: &dataplanev1alpha1.ExecRequest_Stdin{
					Stdin: &dataplanev1alpha1.StdinChunk{
						Data: append([]byte(nil), buf[:n]...),
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		}
		if err == nil {
			continue
		}
		// Send the terminal EOF marker regardless of error type — even
		// on a Read error we want the agent to release its stdin pipe.
		_ = stream.Send(&dataplanev1alpha1.ExecRequest{
			SandboxId: sandboxID,
			Payload: &dataplanev1alpha1.ExecRequest_Stdin{
				Stdin: &dataplanev1alpha1.StdinChunk{Eof: true},
			},
		})
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

// terminalFD returns the file descriptor of stdin when it's a real
// terminal (`*os.File` whose fd passes `term.IsTerminal`). Tests pass
// a `strings.Reader`, which never satisfies the type assertion, so
// the second return value is `false` and the caller naturally skips
// every TTY-only branch.
func terminalFD(stdin io.Reader) (fd int, ok bool) {
	f, isFile := stdin.(*os.File)
	if !isFile {
		return 0, false
	}
	fd = int(f.Fd())
	if !term.IsTerminal(fd) {
		return 0, false
	}
	return fd, true
}

// makeRaw puts fd into raw mode and returns a restore closure. Wrapped
// with sync.Once defensively so a deferred-then-explicit restore can't
// double-call into the term package.
func makeRaw(fd int) (restore func(), err error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() { _ = term.Restore(fd, state) })
	}, nil
}

// sendResize forwards a single ResizePTY frame on the stream.
// Extracted so unit tests can drive it directly against the
// fakeExecStream without going through the cobra wiring or worrying
// about real terminals.
func sendResize(stream execStreamSender, sandboxID string, cols, rows uint32) error {
	return stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: sandboxID,
		Payload: &dataplanev1alpha1.ExecRequest_Resize{
			Resize: &dataplanev1alpha1.ResizePTY{Cols: cols, Rows: rows},
		},
	})
}

// watchTerminalSize installs a SIGWINCH handler that re-reads the
// local terminal size and forwards it as a ResizePTY frame on every
// signal until ctx is cancelled. fd must already be a terminal — the
// caller gates this behind terminalFD.
//
// The handler runs in its own goroutine and exits when ctx is
// cancelled (the typical path is the deferred cancel in the exec run
// flow); signal.Stop is called to release the channel registration so
// the runtime stops delivering SIGWINCH after the watcher tears down.
func watchTerminalSize(ctx context.Context, fd int, stream execStreamSender, sandboxID string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				cols, rows, err := term.GetSize(fd)
				if err != nil {
					continue
				}
				_ = sendResize(stream, sandboxID, uint32(cols), uint32(rows))
			}
		}
	}()
}
