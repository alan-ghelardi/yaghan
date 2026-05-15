// Package pause implements `yag sandbox pause`. It transitions a
// running sandbox into the PAUSED phase via SandboxService.PauseSandbox.
package pause

import (
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause a running sandbox",
		Long: `Pause a sandbox.

The sandbox transitions through PAUSING into PAUSED. The api-server
uses optimistic concurrency control on the sandbox version. Pass
--version to skip the lookup; otherwise the CLI fetches the current
sandbox to read the version before issuing the pause request.`,
		Example: `  # Auto-resolve the version, then pause.
  yag sandbox pause my-sandbox

  # Skip the lookup with an explicit version.
  yag sandbox pause my-sandbox --version 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	lifecycle.AddVersionFlag(cmd)
	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := args[0]
	version, err := lifecycle.ResolveVersion(ctx, cmd, id)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Pausing sandbox %q (version=%d)...\n", id, version)

	if _, err := ctx.ClientSet.SandboxService.PauseSandbox(cmd.Context(),
		&controlplanev1alpha1.PauseSandboxRequest{
			SandboxId: id,
			Version:   version,
		}); err != nil {
		return fmt.Errorf("pause sandbox: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Sandbox %q pause requested. Run 'yag sandbox get %s' to follow its phase.\n",
		id, id)
	return nil
}
