// Package get implements `yag sandbox get`. It calls
// SandboxService.GetSandbox and renders the response in the
// user-selected output format using the helpers in
// ctl/pkg/cli.
package get

import (
	"fmt"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/spf13/cobra"
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Aliases: []string{"describe"},
		Short:   "Show a sandbox",
		Long: `Fetch a sandbox by its id and render it in the requested format.

The api-server returns the full Sandbox object (metadata, resources,
node assignment, intent, status). Use --output-format to switch
between yaml (default) and json. A human-friendly text view will be
added in a future release.`,
		Example: `  # Show the sandbox in YAML (default).
  yag sandbox get my-sandbox

  # Show it as JSON for piping into jq.
  yag sandbox get my-sandbox -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	cli.AddOutputFormatFlag(cmd,
		cli.OutputFormatYAML,
		cli.OutputFormatYAML, cli.OutputFormatJSON)

	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command, args []string) error {
	id := args[0]

	format, err := cli.GetOutputFormat(cmd,
		cli.OutputFormatYAML, cli.OutputFormatJSON)
	if err != nil {
		return err
	}

	resp, err := ctx.ClientSet.SandboxService.GetSandbox(cmd.Context(),
		&controlplanev1alpha1.GetSandboxRequest{SandboxId: id})
	if err != nil {
		return fmt.Errorf("get sandbox: %w", err)
	}
	if resp.GetSandbox() == nil {
		return fmt.Errorf("sandbox %q not found", id)
	}

	out, err := cli.Marshal(resp.GetSandbox(), format)
	if err != nil {
		return fmt.Errorf("marshal sandbox: %w", err)
	}
	if _, err := ctx.IOStreams.Stdout.Write(out); err != nil {
		return err
	}
	return nil
}
