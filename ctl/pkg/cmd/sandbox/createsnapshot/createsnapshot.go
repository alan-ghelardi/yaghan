// Package createsnapshot implements `sindri sandbox create-snapshot`.
// It records the user's intent to snapshot a sandbox via
// SandboxService.CreateSnapshot; the data-plane daemon performs the
// firecracker snapshot, persists the artifacts to durable storage,
// and stamps Sandbox.LastSnapshot once the reconciler converges.
package createsnapshot

import (
	"fmt"

	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/cli"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/lifecycle"
)

const flagDescription = "description"

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-snapshot <id>",
		Short: "Trigger a snapshot for a running sandbox",
		Long: `Trigger a sandbox snapshot.

The api-server records the intent; the data-plane daemon performs the
firecracker CreateSnapshot, persists the artifacts to durable storage,
and stamps Sandbox.LastSnapshot. The api-server uses optimistic
concurrency control on the sandbox version. Pass --version to skip the
lookup; otherwise the CLI fetches the current sandbox to read the
version before issuing the snapshot request.`,
		Example: `  # Auto-resolve the version, then trigger.
  sindri sandbox create-snapshot my-sandbox

  # Attach a description for operators.
  sindri sandbox create-snapshot my-sandbox --description "pre-deploy"

  # Skip the lookup with an explicit version.
  sindri sandbox create-snapshot my-sandbox --version 3`,
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

	if _, err := ctx.ClientSet.SandboxService.CreateSnapshot(cmd.Context(),
		&controlplanev1alpha1.CreateSnapshotRequest{
			SandboxId:   id,
			Version:     version,
			Description: description,
		}); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout,
		"Snapshot requested for sandbox %q. Run 'sindri sandbox get %s' to follow its progress.\n",
		id, id)
	return nil
}
