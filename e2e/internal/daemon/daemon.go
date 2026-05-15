// Package daemon starts and stops the yaghan daemon binary in the
// e2e suite. Mirror of internal/apiserver but with the MinIO
// credentials the daemon's snapshot S3 store needs.
package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alan-ghelardi/yaghan/e2e/internal/infra"
	"github.com/alan-ghelardi/yaghan/e2e/internal/process"
)

// Options configure the daemon binary.
type Options struct {
	// BinPath is the absolute path to the compiled daemon binary
	// (e2e/bin/daemon in the default suite layout).
	BinPath string

	// ConfigPath is the absolute path to the rendered YAML config.
	ConfigPath string

	// LogDir is the directory the binary's stdout/stderr is appended
	// to as "daemon.log". Must already exist.
	LogDir string
}

// Start launches the daemon child process.
func Start(ctx context.Context, opts Options) (*process.Process, error) {
	if opts.BinPath == "" || opts.ConfigPath == "" || opts.LogDir == "" {
		return nil, fmt.Errorf("daemon: BinPath, ConfigPath, and LogDir are required")
	}
	return process.Start(ctx, process.Spec{
		Name:    "daemon",
		BinPath: opts.BinPath,
		Args:    []string{"-config", opts.ConfigPath},
		Env: []string{
			"AWS_ACCESS_KEY_ID=" + infra.MinIOCreds.AccessKeyID,
			"AWS_SECRET_ACCESS_KEY=" + infra.MinIOCreds.SecretAccessKey,
			"AWS_REGION=" + infra.MinIOCreds.Region,
		},
		LogFile: filepath.Join(opts.LogDir, "daemon.log"),
	})
}

// GracefulStopTimeout is the SIGTERM-to-SIGKILL window the suite uses
// when tearing down the daemon.
const GracefulStopTimeout = 10 * time.Second
