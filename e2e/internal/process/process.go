// Package process wraps a long-running child binary the e2e suite
// supervises: starts it with a controlled environment, tees its
// stdout/stderr to a log file, and stops it cleanly with SIGTERM /
// SIGKILL escalation.
package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Spec describes a single supervised binary.
type Spec struct {
	// Name is the short label that appears in error messages
	// ("api-server", "daemon"). Also used as a fallback log filename
	// if LogFile is empty.
	Name string

	// BinPath is the absolute path to the binary to execute.
	BinPath string

	// Args are the binary's argv (without argv[0]).
	Args []string

	// Env are the extra environment variables to pass on top of the
	// current process's env. Values follow os/exec.Cmd.Env shape
	// ("KEY=value"). The parent process's env is forwarded so the
	// binary inherits PATH and friends.
	Env []string

	// LogFile is the path the supervised process's combined
	// stdout/stderr is appended to. The file is opened with O_CREATE
	// so the directory must already exist.
	LogFile string
}

// Process is a supervised child started from a Spec.
type Process struct {
	spec Spec
	cmd  *exec.Cmd
	log  *os.File
	done chan error
}

// Start launches the binary described by spec and returns a Process
// handle. The process inherits the parent's environment plus
// spec.Env. Its combined stdout/stderr is appended to spec.LogFile.
//
// Start does not wait for the binary to become ready — readiness is a
// service-level concern handled by infra.WaitFor* probes in the
// caller.
func Start(_ context.Context, spec Spec) (*Process, error) {
	if spec.LogFile == "" {
		return nil, fmt.Errorf("process %s: LogFile is required", spec.Name)
	}
	logFile, err := os.OpenFile(spec.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", spec.LogFile, err)
	}

	// G204: spec.BinPath and spec.Args are not user input — they are
	// hardcoded by the suite's process supervisors (e2e/bin/<name>
	// and `-config <path>`). Test-code only.
	cmd := exec.Command(spec.BinPath, spec.Args...) //nolint:gosec
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Setpgid so Stop can signal the whole group — covers any
	// children the binary itself spawns.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", spec.Name, err)
	}

	p := &Process{
		spec: spec,
		cmd:  cmd,
		log:  logFile,
		done: make(chan error, 1),
	}
	go func() {
		p.done <- cmd.Wait()
	}()
	return p, nil
}

// Stop sends SIGTERM to the process group, waits up to graceful for
// the process to exit, then escalates to SIGKILL. Returns the wait
// error from the process (may be nil for a clean SIGTERM exit, or
// non-nil for a signal-terminated exit — callers typically ignore the
// returned error during teardown).
func (p *Process) Stop(graceful time.Duration) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
	if err != nil {
		// Process already gone; fall back to PID-only signaling.
		pgid = p.cmd.Process.Pid
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	select {
	case err := <-p.done:
		return p.cleanup(err)
	case <-time.After(graceful):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		err := <-p.done
		return p.cleanup(err)
	}
}

// Done returns a channel that receives the process's wait error
// (nil for a clean exit) once the binary terminates. Useful to detect
// an unexpected early exit while the suite is still running.
func (p *Process) Done() <-chan error { return p.done }

// LogPath returns the path that p's stdout/stderr is being written to.
func (p *Process) LogPath() string { return p.spec.LogFile }

func (p *Process) cleanup(err error) error {
	_ = p.log.Close()
	// A SIGTERM/SIGKILL exit surfaces as *exec.ExitError; treat that
	// as the expected teardown path.
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil
	}
	return err
}
