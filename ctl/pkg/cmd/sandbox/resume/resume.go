// Package resume implements `yag sandbox resume`. It transitions a
// paused sandbox back into the RUNNING phase via
// SandboxService.ResumeSandbox.
package resume

import (
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a paused sandbox",
		Long: `Resume a sandbox.

The sandbox transitions through RESUMING into RESUMED. The api-server
uses optimistic concurrency control on the sandbox version. Pass
--version to skip the lookup; otherwise the CLI fetches the current
sandbox to read the version before issuing the resume request.`,
		Example: `  # Auto-resolve the version, then resume.
  yag sandbox resume my-sandbox

  # Skip the lookup with an explicit version.
  yag sandbox resume my-sandbox --version 4`,
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
		"Resuming sandbox %q (version=%d)...\n", id, version)

	if _, err := ctx.ClientSet.SandboxService.ResumeSandbox(cmd.Context(),
		&controlplanev1alpha1.ResumeSandboxRequest{
			SandboxId: id,
			Version:   version,
		}); err != nil {
		return fmt.Errorf("resume sandbox: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Sandbox %q resume requested. Run 'yag sandbox get %s' to follow its phase.\n",
		id, id)
	return nil
}
