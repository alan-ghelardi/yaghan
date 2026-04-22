// Package lifecycle hosts helpers shared by the mutation subcommands
// (pause, resume, delete). They all need optimistic-concurrency
// versions, so the version flag declaration and resolution live here
// rather than being duplicated three times.
package lifecycle

import (
	"fmt"

	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

// FlagVersion is the flag name. Callers reference it via this const so
// flag-changed checks and tests stay in sync.
const FlagVersion = "version"

// AddVersionFlag declares --version as an optional int64 flag. When the
// caller omits it, ResolveVersion will fetch the sandbox to discover
// the current version itself.
func AddVersionFlag(cmd *cobra.Command) {
	cmd.Flags().Int64(FlagVersion, 0,
		"Sandbox version observed by the caller (optimistic concurrency). "+
			"If unset, the CLI fetches the sandbox first and uses the version it reports.")
}

// ResolveVersion returns the value of --version when supplied;
// otherwise it calls GetSandbox and returns metadata.version. The
// pre-fetch path emits a short status line so users see why an extra
// roundtrip happened.
func ResolveVersion(ctx *machinery.Context, cmd *cobra.Command, sandboxID string) (int64, error) {
	if cmd.Flags().Changed(FlagVersion) {
		return cmd.Flags().GetInt64(FlagVersion)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Reading current version of sandbox %q...\n", sandboxID)

	resp, err := ctx.ClientSet.SandboxService.GetSandbox(cmd.Context(),
		&controlplanev1alpha1.GetSandboxRequest{SandboxId: sandboxID})
	if err != nil {
		return 0, fmt.Errorf("get sandbox to resolve version: %w", err)
	}
	if resp.GetSandbox() == nil {
		return 0, fmt.Errorf("sandbox %q not found", sandboxID)
	}
	return resp.GetSandbox().GetMetadata().GetVersion(), nil
}
