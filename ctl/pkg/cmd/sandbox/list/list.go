// Package list implements `sindri sandbox list`. It calls
// SandboxService.ListSandboxes and renders the response either as a
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
	flagNamespace         = "namespace"
	flagNodeID            = "node-id"
	flagPhase             = "phase"
	flagContinuationToken = "continuation-token"
	flagNoPagination      = "no-pagination"
	flagPageSize          = "page-size"
	flagSortOrder         = "sort-order"
)

// Allowed values for the --phase and --sort-order flags. Empty is
// also allowed (passes through as the proto's UNSPECIFIED enum value).
var (
	allowedPhases = []string{
		"pending", "running", "pausing", "paused", "resuming",
		"snapshotting", "deleting", "deleted", "failed",
	}
	allowedSorts = []string{"newest-first", "oldest-first"}
)

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List sandboxes",
		Long: `List sandboxes filtered by namespace and/or by node id.

At least one of --namespace or --node-id must be provided; the server
enforces the same constraint to prevent unbounded queries. Both may be
combined to scope a listing to a single namespace on a single node.

The default view is a formatted table with interactive pagination.
JSON and YAML output marshal the full ListSandboxesResponse — including
the continuation_token — so callers can drive their own pagination.`,
		Example: `  # Default table view, scoped to a namespace.
  sindri sandbox list --namespace team-alpha

  # All sandboxes scheduled on a node.
  sindri sandbox list --node-id node-7

  # Combine both filters and only show running sandboxes.
  sindri sandbox list -N team-alpha --node-id node-7 --phase running

  # JSON for piping into jq, walking pages by hand.
  sindri sandbox list -N team-alpha -o json
  sindri sandbox list -N team-alpha -o json -c <token-from-previous-response>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(ctx, cmd)
		},
	}

	cli.AddOutputFormatFlag(cmd,
		cli.OutputFormatTable,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)

	cmd.Flags().StringP(flagNamespace, "N", "",
		"Filter sandboxes by namespace. At least one of --namespace or --node-id is required.")
	cmd.Flags().String(flagNodeID, "",
		"Filter sandboxes by the id of the node they are scheduled on. At least one of --namespace or --node-id is required.")
	cmd.Flags().StringP(flagPhase, "p", "",
		fmt.Sprintf("Filter sandboxes by status phase. One of: (%s).", strings.Join(allowedPhases, ", ")))
	cmd.Flags().StringP(flagContinuationToken, "c", "",
		"Token for paginating JSON or YAML output. Has no effect on other formats.")
	cmd.Flags().BoolP(flagNoPagination, "n", false,
		"Disable interactive pagination in table output. Has no effect on other formats.")
	cmd.Flags().Int32(flagPageSize, 30, "Number of sandboxes per page")
	cmd.Flags().String(flagSortOrder, "",
		fmt.Sprintf("Sort order for the returned sandboxes. One of: (%s).", strings.Join(allowedSorts, ", ")))

	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command) error {
	format, err := cli.GetOutputFormat(cmd,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)
	if err != nil {
		return err
	}

	namespace, _ := cmd.Flags().GetString(flagNamespace)
	nodeID, _ := cmd.Flags().GetString(flagNodeID)
	if namespace == "" && nodeID == "" {
		return fmt.Errorf("one of --%s or --%s is required", flagNamespace, flagNodeID)
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

	listOnce := func(token string) (*controlplanev1alpha1.ListSandboxesResponse, error) {
		resp, err := ctx.ClientSet.SandboxService.ListSandboxes(cmd.Context(),
			&controlplanev1alpha1.ListSandboxesRequest{
				Namespace:         namespace,
				NodeId:            nodeID,
				StatusPhase:       statusPhase,
				ContinuationToken: token,
				PageSize:          pageSize,
				SortOrder:         sortOrder,
			})
		if err != nil {
			return nil, fmt.Errorf("list sandboxes: %w", err)
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

// renderTable drives the interactive table renderer. The first call to
// GetItems uses an empty continuation token; each subsequent call
// (triggered when the user presses 'l' to load more) reuses the token
// returned by the previous response.
func renderTable(
	ctx *cli.Context,
	listOnce func(string) (*controlplanev1alpha1.ListSandboxesResponse, error),
	noPagination bool,
) error {
	var nextToken string
	first := true

	getItems := func() ([]*controlplanev1alpha1.Sandbox, bool, error) {
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
		return resp.GetSandboxes(), nextToken != "", nil
	}

	table := print.SandboxesTable(getItems)
	table.DisablePagination = noPagination
	return table.Render(ctx)
}

func parsePhase(s string) (controlplanev1alpha1.SandboxStatus_Phase, error) {
	switch s {
	case "":
		return controlplanev1alpha1.SandboxStatus_PHASE_UNSPECIFIED, nil
	case "pending":
		return controlplanev1alpha1.SandboxStatus_PHASE_PENDING, nil
	case "running":
		return controlplanev1alpha1.SandboxStatus_PHASE_RUNNING, nil
	case "pausing":
		return controlplanev1alpha1.SandboxStatus_PHASE_PAUSING, nil
	case "paused":
		return controlplanev1alpha1.SandboxStatus_PHASE_PAUSED, nil
	case "resuming":
		return controlplanev1alpha1.SandboxStatus_PHASE_RESUMING, nil
	case "snapshotting":
		return controlplanev1alpha1.SandboxStatus_PHASE_SNAPSHOTTING, nil
	case "deleting":
		return controlplanev1alpha1.SandboxStatus_PHASE_DELETING, nil
	case "deleted":
		return controlplanev1alpha1.SandboxStatus_PHASE_DELETED, nil
	case "failed":
		return controlplanev1alpha1.SandboxStatus_PHASE_FAILED, nil
	default:
		return 0, fmt.Errorf("invalid value %q for --%s. Allowed values: %s",
			s, flagPhase, strings.Join(allowedPhases, ", "))
	}
}

func parseSortOrder(s string) (controlplanev1alpha1.ListSandboxesRequest_Order, error) {
	switch s {
	case "":
		return controlplanev1alpha1.ListSandboxesRequest_ORDER_UNSPECIFIED, nil
	case "newest-first":
		return controlplanev1alpha1.ListSandboxesRequest_ORDER_NEWEST_FIRST, nil
	case "oldest-first":
		return controlplanev1alpha1.ListSandboxesRequest_ORDER_OLDEST_FIRST, nil
	default:
		return 0, fmt.Errorf("invalid value %q for --%s. Allowed values: %s",
			s, flagSortOrder, strings.Join(allowedSorts, ", "))
	}
}
