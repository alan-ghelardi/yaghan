// Package apiserver starts and stops the api-server binary in the
// e2e suite. It's a thin wrapper around internal/process that pins
// the environment variables api-server's AWS SDK chain needs to talk
// to DynamoDB Local (driven by api-server/dev/start.sh's exports).
package apiserver

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alan-ghelardi/yaghan/e2e/internal/infra"
	"github.com/alan-ghelardi/yaghan/e2e/internal/process"
)

// Options configure the api-server binary.
type Options struct {
	// BinPath is the absolute path to the compiled api-server binary
	// (e2e/bin/api-server in the default suite layout).
	BinPath string

	// ConfigPath is the absolute path to the rendered YAML config.
	ConfigPath string

	// LogDir is the directory the binary's stdout/stderr is appended
	// to as "api-server.log". Must already exist.
	LogDir string
}

// Start launches the api-server child process.
func Start(ctx context.Context, opts Options) (*process.Process, error) {
	if opts.BinPath == "" || opts.ConfigPath == "" || opts.LogDir == "" {
		return nil, fmt.Errorf("apiserver: BinPath, ConfigPath, and LogDir are required")
	}
	return process.Start(ctx, process.Spec{
		Name:    "api-server",
		BinPath: opts.BinPath,
		Args:    []string{"-config", opts.ConfigPath},
		Env: []string{
			"AWS_ACCESS_KEY_ID=" + infra.DynamoDBLocalCreds.AccessKeyID,
			"AWS_SECRET_ACCESS_KEY=" + infra.DynamoDBLocalCreds.SecretAccessKey,
			"AWS_REGION=" + infra.DynamoDBLocalCreds.Region,
		},
		LogFile: filepath.Join(opts.LogDir, "api-server.log"),
	})
}

// GracefulStopTimeout is the SIGTERM-to-SIGKILL window the suite uses
// when tearing down the api-server. 10s is plenty for the gRPC
// server's graceful stop.
const GracefulStopTimeout = 10 * time.Second
