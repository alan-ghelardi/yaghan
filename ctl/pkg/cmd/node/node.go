// Package node groups subcommands that operate on cluster nodes. It
// exists as a registration point under the root `sindri` command; the
// concrete behaviour lives in the per-verb subpackages.
package node

import (
	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cli"
	"golang.nuinfra.net/ctl/pkg/cmd/node/get"
	"golang.nuinfra.net/ctl/pkg/cmd/node/list"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "node",
		Aliases: []string{"nodes"},
		Short:   "Manage nodes",
		Long:    `Inspect cluster nodes.`,
	}

	cmd.AddCommand(
		get.New(ctx),
		list.New(ctx),
	)

	return cmd
}
