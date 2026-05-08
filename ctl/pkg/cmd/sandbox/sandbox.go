package sandbox

import (
	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/cp"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/create"
	deletecmd "golang.nuinfra.net/ctl/pkg/cmd/sandbox/delete"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/exec"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/get"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/pause"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/resume"
	"golang.nuinfra.net/ctl/pkg/cli"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sandbox",
		Aliases: []string{"sbx"},
		Short:   "Manage sandboxes",
		Long:    `FIX ME!`,
	}

	cmd.AddCommand(create.New(ctx))
	cmd.AddCommand(get.New(ctx))
	cmd.AddCommand(pause.New(ctx))
	cmd.AddCommand(resume.New(ctx))
	cmd.AddCommand(deletecmd.New(ctx))
	cmd.AddCommand(exec.New(ctx))
	cmd.AddCommand(cp.New(ctx))

	return cmd
}
