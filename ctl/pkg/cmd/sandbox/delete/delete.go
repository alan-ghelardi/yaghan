// Package delete implements `yag sandbox delete`. It tears a
// sandbox down via SandboxService.DeleteSandbox.
package delete

import (
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a sandbox",
		Long: `Delete a sandbox.

The sandbox transitions through DELETING into DELETED; the underlying
microVM is torn down on the worker node and the record is then
removed from the api-server. The api-server uses optimistic
concurrency control on the sandbox version. Pass --version to skip
the lookup; otherwise the CLI fetches the current sandbox to read
the version before issuing the delete request.`,
		Example: `  # Auto-resolve the version, then delete.
  yag sandbox delete my-sandbox

  # Skip the lookup with an explicit version.
  yag sandbox delete my-sandbox --version 5`,
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
		"Deleting sandbox %q (version=%d)...\n", id, version)

	if _, err := ctx.ClientSet.SandboxService.DeleteSandbox(cmd.Context(),
		&controlplanev1alpha1.DeleteSandboxRequest{
			SandboxId: id,
			Version:   version,
		}); err != nil {
		return fmt.Errorf("delete sandbox: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Sandbox %q delete requested. Run 'yag sandbox get %s' to follow its phase.\n",
		id, id)
	return nil
}
