// Package list implements `sindri node list`. It calls
// ClusterService.ListNodes and renders the response either as a
// formatted table (default) or as JSON / YAML for piping.
package list

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/ctl/pkg/cli"
	"golang.nuinfra.net/ctl/pkg/cli/print"
)

const (
	flagContinuationToken = "continuation-token"
	flagPhase             = "phase"
	flagNoPagination      = "no-pagination"
	flagPageSize          = "page-size"
	flagSortOrder         = "sort-order"
)

// Allowed values for the --phase and --sort-order flags. Empty is
// also allowed (passes through as the proto's UNSPECIFIED enum value).
var (
	allowedPhases = []string{"healthy", "unhealthy", "lost", "deleted", "unknown"}
	allowedSorts  = []string{"newest-first", "oldest-first"}
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List nodes",
		Long: `List cluster nodes in the requested format.

The default view is a formatted table with interactive pagination.
JSON and YAML output marshal the full ListNodesResponse — including
the continuation_token — so callers can drive their own pagination.`,
		Example: `  # Default table view (interactive pagination on 'l').
  sindri node list

  # Filter by status phase.
  sindri node list --phase healthy

  # JSON for piping into jq, walking pages by hand.
  sindri node list -o json
  sindri node list -o json -c <token-from-previous-response>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(ctx, cmd)
		},
	}

	cli.AddOutputFormatFlag(cmd,
		cli.OutputFormatTable,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)

	cmd.Flags().StringP(flagContinuationToken, "c", "",
		"Token for paginating JSON or YAML output. Has no effect on other formats.")
	cmd.Flags().StringP(flagPhase, "p", "",
		fmt.Sprintf("Filter nodes by status phase. One of: (%s).", strings.Join(allowedPhases, ", ")))
	cmd.Flags().BoolP(flagNoPagination, "n", false,
		"Disable interactive pagination in table output. Has no effect on other formats.")
	cmd.Flags().Int32(flagPageSize, 30, "Number of nodes per page")
	cmd.Flags().String(flagSortOrder, "",
		fmt.Sprintf("Sort order for the returned nodes. One of: (%s).", strings.Join(allowedSorts, ", ")))

	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command) error {
	format, err := cli.GetOutputFormat(cmd,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)
	if err != nil {
		return err
	}

	phaseFlag, _ := cmd.Flags().GetString(flagPhase)
	statusPhase, err := parsePhase(phaseFlag)
	if err != nil {
		return err
	}

	sortFlag, _ := cmd.Flags().GetString(flagSortOrder)
	sortOrder, err := parseSortOrder(sortFlag)
	if err != nil {
		return err
	}

	pageSize, _ := cmd.Flags().GetInt32(flagPageSize)
	noPagination, _ := cmd.Flags().GetBool(flagNoPagination)
	contToken, _ := cmd.Flags().GetString(flagContinuationToken)

	listOnce := func(token string) (*controlplanev1alpha1.ListNodesResponse, error) {
		resp, err := ctx.ClientSet.ClusterService.ListNodes(cmd.Context(),
			&controlplanev1alpha1.ListNodesRequest{
				StatusPhase:       statusPhase,
				ContinuationToken: token,
				PageSize:          pageSize,
				SortOrder:         sortOrder,
			})
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		return resp, nil
	}

	switch format {
	case cli.OutputFormatTable:
		return renderTable(ctx, listOnce, noPagination)
	case cli.OutputFormatJSON, cli.OutputFormatYAML:
		resp, err := listOnce(contToken)
		if err != nil {
			return err
		}
		out, err := cli.Marshal(resp, format)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		_, err = ctx.IOStreams.Stdout.Write(out)
		return err
	}

	return fmt.Errorf("unsupported output format %q", format)
}

// renderTable drives the interactive table renderer. The first call
// to GetItems uses an empty continuation token; each subsequent call
// (triggered when the user presses 'l' to load more) reuses the
// token returned by the previous response.
func renderTable(
	ctx *cli.Context,
	listOnce func(string) (*controlplanev1alpha1.ListNodesResponse, error),
	noPagination bool,
) error {
	var nextToken string
	first := true

	getItems := func() ([]*controlplanev1alpha1.Node, bool, error) {
		token := ""
		if !first {
			token = nextToken
		}
		first = false

		resp, err := listOnce(token)
		if err != nil {
			return nil, false, err
		}
		nextToken = resp.GetContinuationToken()
		return resp.GetNodes(), nextToken != "", nil
	}

	table := print.NodesTable(getItems)
	table.DisablePagination = noPagination
	return table.Render(ctx)
}

func parsePhase(s string) (controlplanev1alpha1.NodeStatus_Phase, error) {
	switch s {
	case "":
		return controlplanev1alpha1.NodeStatus_PHASE_UNSPECIFIED, nil
	case "healthy":
		return controlplanev1alpha1.NodeStatus_PHASE_HEALTHY, nil
	case "unhealthy":
		return controlplanev1alpha1.NodeStatus_PHASE_UNHEALTHY, nil
	case "lost":
		return controlplanev1alpha1.NodeStatus_PHASE_LOST, nil
	case "deleted":
		return controlplanev1alpha1.NodeStatus_PHASE_DELETED, nil
	case "unknown":
		return controlplanev1alpha1.NodeStatus_PHASE_UNKNOWN, nil
	default:
		return 0, fmt.Errorf("invalid value %q for --%s. Allowed values: %s",
			s, flagPhase, strings.Join(allowedPhases, ", "))
	}
}

func parseSortOrder(s string) (controlplanev1alpha1.ListNodesRequest_Order, error) {
	switch s {
	case "":
		return controlplanev1alpha1.ListNodesRequest_ORDER_UNSPECIFIED, nil
	case "newest-first":
		return controlplanev1alpha1.ListNodesRequest_ORDER_NEWEST_FIRST, nil
	case "oldest-first":
		return controlplanev1alpha1.ListNodesRequest_ORDER_OLDEST_FIRST, nil
	default:
		return 0, fmt.Errorf("invalid value %q for --%s. Allowed values: %s",
			s, flagSortOrder, strings.Join(allowedSorts, ", "))
	}
}
