// Package yag drives the yag CLI as a child process in e2e tests.
// It exists so scenarios assert on proto objects rather than parsed
// strings: RunYAML invokes yag with `-o yaml`, then unmarshals the
// stdout into the caller's proto message via the same protoyaml
// library yag itself uses to marshal (ctl/pkg/cli/utils.go).
package yag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"buf.build/go/protoyaml"
	"google.golang.org/protobuf/proto"
)

// BinEnvVar names the environment variable Run reads to resolve the
// yag binary. The Ginkgo BeforeSuite sets this to e2e/bin/yag.
const BinEnvVar = "E2E_YAG_BIN"

// Result captures everything the suite might want to assert on after
// running yag.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Run executes yag with the given args. The returned Result is
// populated even on non-zero exit so callers can include stderr in
// failure messages. A non-nil error is returned only when the binary
// could not be started (missing path, permission denied, …) — a
// non-zero exit status is signalled via Result.ExitCode.
func Run(ctx context.Context, args ...string) (*Result, error) {
	binPath := os.Getenv(BinEnvVar)
	if binPath == "" {
		return nil, fmt.Errorf("yag: %s is not set; run via the e2e suite (BeforeSuite sets it) "+
			"or export it manually to e2e/bin/yag", BinEnvVar)
	}

	// G702: binPath and args are not user input — binPath is
	// resolved from BinEnvVar set by the e2e suite to a fixed
	// e2e/bin/yag path, and args come from spec authors writing
	// scenarios in this same module. Test-code only.
	cmd := exec.CommandContext(ctx, binPath, args...) //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := &Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}

	var exit *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exit):
		res.ExitCode = exit.ExitCode()
	default:
		return res, fmt.Errorf("yag: exec %s: %w", binPath, err)
	}
	return res, nil
}

// RunYAML invokes yag with the caller's args plus `-o yaml`, asserts
// a zero exit code, and unmarshals stdout into target.
//
// Returns an error if yag failed to start, exited non-zero (stderr is
// included in the error message), or produced YAML the protoyaml
// unmarshaler rejected.
func RunYAML(ctx context.Context, target proto.Message, args ...string) error {
	full := append([]string{}, args...)
	full = append(full, "-o", "yaml")

	res, err := Run(ctx, full...)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("yag %v: exit %d\nstderr:\n%s", args, res.ExitCode, res.Stderr)
	}
	if err := protoyaml.Unmarshal(res.Stdout, target); err != nil {
		return fmt.Errorf("yag %v: unmarshal yaml: %w\nstdout:\n%s", args, err, res.Stdout)
	}
	return nil
}
