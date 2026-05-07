// Package get implements `sindri node get`. It calls
// ClusterService.GetNode and renders the response in the user-selected
// output format using the helpers in ctl/pkg/machinery.
package get

import (
	"fmt"

	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

func New(ctx *machinery.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Show a node",
		Long: `Fetch a node by its id and render it in the requested format.

The api-server returns the full Node object (metadata, resources,
status). Use --output-format to switch between yaml (default) and
json. A human-friendly text view will be added in a future release.`,
		Example: `  # Show the node in YAML (default).
  sindri node get my-node

  # Show it as JSON for piping into jq.
  sindri node get my-node -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(ctx, cmd, args)
		},
	}

	machinery.AddOutputFormatFlag(cmd,
		machinery.OutputFormatYAML,
		machinery.OutputFormatYAML, machinery.OutputFormatJSON)

	return cmd
}

func run(ctx *machinery.Context, cmd *cobra.Command, args []string) error {
	id := args[0]

	format, err := machinery.GetOutputFormat(cmd,
		machinery.OutputFormatYAML, machinery.OutputFormatJSON)
	if err != nil {
		return err
	}

	resp, err := ctx.ClientSet.ClusterService.GetNode(cmd.Context(),
		&controlplanev1alpha1.GetNodeRequest{NodeId: id})
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	if resp.GetNode() == nil {
		return fmt.Errorf("node %q not found", id)
	}

	out, err := machinery.Marshal(resp.GetNode(), format)
	if err != nil {
		return fmt.Errorf("marshal node: %w", err)
	}
	if _, err := ctx.IOStreams.Stdout.Write(out); err != nil {
		return err
	}
	return nil
}
