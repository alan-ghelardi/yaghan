// Package get implements `yag snapshot get`. It calls
// SnapshotService.GetSnapshot and renders the response in the
// user-selected output format using the helpers in ctl/pkg/cli.
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
		Short:   "Show a snapshot",
		Long: `Fetch a snapshot by its id and render it in the requested format.

The api-server returns the full Snapshot object (metadata, sandbox
reference). Use --output-format to switch between yaml (default) and
json.`,
		Example: `  # Show the snapshot in YAML (default).
  yag snapshot get snap-123

  # Show it as JSON for piping into jq.
  yag snapshot get snap-123 -o json`,
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

	resp, err := ctx.ClientSet.SnapshotService.GetSnapshot(cmd.Context(),
		&controlplanev1alpha1.GetSnapshotRequest{SnapshotId: id})
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	if resp.GetSnapshot() == nil {
		return fmt.Errorf("snapshot %q not found", id)
	}

	out, err := cli.Marshal(resp.GetSnapshot(), format)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if _, err := ctx.IOStreams.Stdout.Write(out); err != nil {
		return err
	}
	return nil
}
