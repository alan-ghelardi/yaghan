// Package snapshot groups subcommands that operate on Snapshot
// resources. It exists as a registration point under the root `yag`
// command; the concrete behaviour lives in the per-verb subpackages.
//
// Note: `yag sandbox snapshot` (under ctl/pkg/cmd/sandbox/snapshot)
// is a different command — it triggers a snapshot on a sandbox via
// SandboxService.StartSnapshot. This package contains the resource
// surface (get / list / delete) backed by SnapshotService.
package snapshot

import (
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	deletecmd "github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/delete"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/get"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/list"
	"github.com/spf13/cobra"
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
