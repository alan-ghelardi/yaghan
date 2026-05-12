// Package delete implements `sindri snapshot delete`. It calls
// SnapshotService.DeleteSnapshot, which removes the DB row
// synchronously. Snapshots are immutable, so the operation has no
// version handshake and is idempotent server-side.
package delete

import (
	"fmt"

	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/cli"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a snapshot",
		Long: `Delete a snapshot.

The api-server removes the snapshot row synchronously and the call
returns OK regardless of whether the row was present — repeating a
delete is safe.

Cleanup of the snapshot artifact in durable storage is a separate
concern owned by the daemon-side store; this command does not block on
artifact teardown.`,
		Example: `  sindri snapshot delete snap-123
  sindri snapshot rm snap-123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}
	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := args[0]

	if _, err := ctx.ClientSet.SnapshotService.DeleteSnapshot(cmd.Context(),
		&controlplanev1alpha1.DeleteSnapshotRequest{SnapshotId: id}); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}

	fmt.Fprintf(ctx.IOStreams.Stdout, "Snapshot %q deleted.\n", id)
	return nil
}
