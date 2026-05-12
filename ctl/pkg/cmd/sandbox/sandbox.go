package sandbox

import (
	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cli"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/cp"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/create"
	deletecmd "golang.nuinfra.net/ctl/pkg/cmd/sandbox/delete"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/exec"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/get"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/pause"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/resume"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/snapshot"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sandbox",
		Aliases: []string{"sbx", "sandboxes"},
		Short:   "Manage sandboxes",
		Long:    `FIX ME!`,
	}

	cmd.AddCommand(
		create.New(ctx),
		get.New(ctx),
		pause.New(ctx),
		resume.New(ctx),
		snapshot.New(ctx),
		deletecmd.New(ctx),
		exec.New(ctx),
		cp.New(ctx),
	)

	return cmd
}
