package sandbox

import (
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/cp"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/create"
	deletecmd "github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/delete"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/exec"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/get"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/list"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/pause"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/resume"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/snapshot"
	"github.com/spf13/cobra"
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
		list.New(ctx),
		pause.New(ctx),
		resume.New(ctx),
		snapshot.New(ctx),
		deletecmd.New(ctx),
		exec.New(ctx),
		cp.New(ctx),
	)

	return cmd
}
