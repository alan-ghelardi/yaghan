// Package snapshot groups subcommands that operate on Snapshot
// resources. It exists as a registration point under the root `sindri`
// command; the concrete behaviour lives in the per-verb subpackages.
//
// Note: `sindri sandbox snapshot` (under ctl/pkg/cmd/sandbox/snapshot)
// is a different command — it triggers a snapshot on a sandbox via
// SandboxService.StartSnapshot. This package contains the resource
// surface (get / list / delete) backed by SnapshotService.
package snapshot

import (
	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cli"
	deletecmd "golang.nuinfra.net/ctl/pkg/cmd/snapshot/delete"
	"golang.nuinfra.net/ctl/pkg/cmd/snapshot/get"
	"golang.nuinfra.net/ctl/pkg/cmd/snapshot/list"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snapshot",
		Aliases: []string{"snapshots", "snap"},
		Short:   "Manage snapshots",
		Long: `Inspect, list, and delete sandbox snapshots.

Snapshots are created by the daemon as the final stage of the snapshot
reconcile loop; the create RPC is internal-only and not exposed through
this CLI.`,
	}

	cmd.AddCommand(
		get.New(ctx),
		list.New(ctx),
		deletecmd.New(ctx),
	)

	return cmd
}
