// Package exec implements `sindri sandbox exec`. It opens a
// bidirectional stream against the daemon's Exec RPC and shuttles
// stdin/stdout/stderr between the local terminal and a guest process,
// forwarding the guest's exit code via machinery.ExitCodeError.
package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/machinery"
	"golang.org/x/term"
)

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

func New(ctx *machinery.Context) *cobra.Command {
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
guest without sindri trying to parse them.`,
		Example: `  # Print the guest hostname.
  sindri sandbox exec my-sandbox -- hostname

  # Open an interactive shell with a TTY allocated.
  sindri sandbox exec my-sandbox -it -- /bin/bash

  # Run a build with extra env and a custom working directory.
  sindri sandbox exec my-sandbox -w /workspace -e GOOS=linux -e GOARCH=amd64 -- go build ./...

  # Pipe data on stdin without a TTY.
  cat input.txt | sindri sandbox exec my-sandbox -i -- sh -c 'cat > /tmp/out'`,
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
			"usage: sindri sandbox exec <id> [flags] -- <command> [args...]\n" +
				"the sandbox id must be the only argument before `--`")
	}
	if len(args) < 2 {
		return errors.New("a command to run must be supplied after `--`")
	}
	return nil
}

func run(ctx *machinery.Context, cmd *cobra.Command, args []string) error {
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
	if tty {
		if restore, ok := maybeRawMode(ctx.IOStreams.Stdin); ok {
			defer restore()
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
		return &machinery.ExitCodeError{Code: int(exitCode)}
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

// maybeRawMode flips the local terminal into raw mode if stdin is a
// TTY (a *os.File whose fd is a terminal). The returned restore
// closure undoes the change. A second return value of false means
// stdin is not a terminal — the caller should leave the terminal
// alone (this is the test path).
//
// Wrapped with sync.Once defensively so a deferred-then-explicit
// restore can't double-call into the term package.
func maybeRawMode(stdin io.Reader) (restore func(), ok bool) {
	f, isFile := stdin.(*os.File)
	if !isFile {
		return nil, false
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return nil, false
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(func() { _ = term.Restore(fd, state) })
	}, true
}
