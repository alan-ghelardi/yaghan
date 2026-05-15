// Package snapshot implements `yag sandbox snapshot`.
// It records the user's intent to snapshot a sandbox via
// SandboxService.StartSnapshot; the data-plane daemon performs the
// firecracker snapshot, persists the artifacts to durable storage,
// and stamps Sandbox.LastSnapshot once the reconciler converges.
package snapshot

import (
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

const flagDescription = "description"

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snapshot <id>",
		Aliases: []string{"snap"},
		Short:   "Trigger a snapshot for a running sandbox",
		Long: `Trigger a sandbox snapshot.

The api-server records the intent; the data-plane daemon performs the
firecracker CreateSnapshot, persists the artifacts to durable storage,
and stamps Sandbox.LastSnapshot. The api-server uses optimistic
concurrency control on the sandbox version. Pass --version to skip the
lookup; otherwise the CLI fetches the current sandbox to read the
version before issuing the snapshot request.`,
		Example: `  # Auto-resolve the version, then trigger.
  yag sandbox snapshot my-sandbox

  # Attach a description for operators.
  yag sandbox snapshot my-sandbox --description "pre-deploy"

  # Skip the lookup with an explicit version.
  yag sandbox snapshot my-sandbox --version 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	lifecycle.AddVersionFlag(cmd)
	cmd.Flags().String(flagDescription, "",
		"Human-readable label attached to the snapshot intent (max 256 chars).")
	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := args[0]
	version, err := lifecycle.ResolveVersion(ctx, cmd, id)
	if err != nil {
		return err
	}
	description, err := cmd.Flags().GetString(flagDescription)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Requesting snapshot for sandbox %q (version=%d)...\n", id, version)

	if _, err := ctx.ClientSet.SandboxService.StartSnapshot(cmd.Context(),
		&controlplanev1alpha1.StartSnapshotRequest{
			SandboxId:   id,
			Version:     version,
			Description: description,
		}); err != nil {
		return fmt.Errorf("start snapshot: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Snapshot requested for sandbox %q. Run 'yag sandbox get %s' to follow its progress.\n",
		id, id)
	return nil
}
