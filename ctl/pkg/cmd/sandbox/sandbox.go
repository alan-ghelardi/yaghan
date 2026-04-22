package sandbox

import (
	"github.com/spf13/cobra"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/create"
	deletecmd "golang.nuinfra.net/ctl/pkg/cmd/sandbox/delete"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/get"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/pause"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/resume"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

func New(ctx *machinery.Context) *cobra.Command {
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

	return cmd
}
