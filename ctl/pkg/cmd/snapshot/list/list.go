// Package list implements `yag snapshot list`. It calls
// SnapshotService.ListSnapshots and renders the response either as a
// formatted table (default) or as JSON / YAML for piping.
package list

import (
	"fmt"
	"strings"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli/print"
	"github.com/spf13/cobra"
)

const (
	flagNamespace         = "namespace"
	flagSandboxID         = "sandbox-id"
	flagContinuationToken = "continuation-token"
	flagNoPagination      = "no-pagination"
	flagPageSize          = "page-size"
	flagSortOrder         = "sort-order"
)

// Allowed values for the --sort-order flag. Empty is also allowed
// (passes through as the proto's UNSPECIFIED enum value).
var allowedSorts = []string{"newest-first", "oldest-first"}

func New(ctx *cli.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List snapshots",
		Long: `List snapshots filtered by namespace or by sandbox id.

Exactly one of --namespace or --sandbox-id must be provided; the server
enforces the same constraint.

The default view is a formatted table with interactive pagination. JSON
and YAML output marshal the full ListSnapshotsResponse — including the
continuation_token — so callers can drive their own pagination.`,
		Example: `  # Default table view, scoped to a namespace.
  yag snapshot list --namespace team-alpha

  # All snapshots taken from a specific sandbox.
  yag snapshot list --sandbox-id sb-42

  # JSON for piping into jq, walking pages by hand.
  yag snapshot list -N team-alpha -o json
  yag snapshot list -N team-alpha -o json -c <token-from-previous-response>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(ctx, cmd)
		},
	}

	cli.AddOutputFormatFlag(cmd,
		cli.OutputFormatTable,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)

	cmd.Flags().StringP(flagNamespace, "N", "",
		"Filter snapshots by namespace. Mutually exclusive with --sandbox-id.")
	cmd.Flags().StringP(flagSandboxID, "s", "",
		"Filter snapshots by the sandbox they were taken from. Mutually exclusive with --namespace.")
	cmd.Flags().StringP(flagContinuationToken, "c", "",
		"Token for paginating JSON or YAML output. Has no effect on other formats.")
	cmd.Flags().BoolP(flagNoPagination, "n", false,
		"Disable interactive pagination in table output. Has no effect on other formats.")
	cmd.Flags().Int32(flagPageSize, 30, "Number of snapshots per page")
	cmd.Flags().String(flagSortOrder, "",
		fmt.Sprintf("Sort order for the returned snapshots. One of: (%s).", strings.Join(allowedSorts, ", ")))

	return cmd
}

func run(ctx *cli.Context, cmd *cobra.Command) error {
	format, err := cli.GetOutputFormat(cmd,
		cli.OutputFormatTable, cli.OutputFormatJSON, cli.OutputFormatYAML)
	if err != nil {
		return err
	}

	namespace, _ := cmd.Flags().GetString(flagNamespace)
	sandboxID, _ := cmd.Flags().GetString(flagSandboxID)
	if namespace == "" && sandboxID == "" {
		return fmt.Errorf("one of --%s or --%s is required", flagNamespace, flagSandboxID)
	}
	if namespace != "" && sandboxID != "" {
		return fmt.Errorf("--%s and --%s are mutually exclusive", flagNamespace, flagSandboxID)
	}

	sortFlag, _ := cmd.Flags().GetString(flagSortOrder)
	sortOrder, err := parseSortOrder(sortFlag)
	if err != nil {
		return err
	}

	pageSize, _ := cmd.Flags().GetInt32(flagPageSize)
	noPagination, _ := cmd.Flags().GetBool(flagNoPagination)
	contToken, _ := cmd.Flags().GetString(flagContinuationToken)

	listOnce := func(token string) (*controlplanev1alpha1.ListSnapshotsResponse, error) {
		resp, err := ctx.ClientSet.SnapshotService.ListSnapshots(cmd.Context(),
			&controlplanev1alpha1.ListSnapshotsRequest{
				Namespace:         namespace,
				SandboxId:         sandboxID,
				ContinuationToken: token,
				PageSize:          pageSize,
				SortOrder:         sortOrder,
			})
		if err != nil {
			return nil, fmt.Errorf("list snapshots: %w", err)
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
	listOnce func(string) (*controlplanev1alpha1.ListSnapshotsResponse, error),
	noPagination bool,
) error {
	var nextToken string
	first := true

	getItems := func() ([]*controlplanev1alpha1.Snapshot, bool, error) {
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
		return resp.GetSnapshots(), nextToken != "", nil
	}

	table := print.SnapshotsTable(getItems)
	table.DisablePagination = noPagination
	return table.Render(ctx)
}

func parseSortOrder(s string) (controlplanev1alpha1.ListSnapshotsRequest_Order, error) {
	switch s {
	case "":
		return controlplanev1alpha1.ListSnapshotsRequest_ORDER_UNSPECIFIED, nil
	case "newest-first":
		return controlplanev1alpha1.ListSnapshotsRequest_ORDER_NEWEST_FIRST, nil
	case "oldest-first":
		return controlplanev1alpha1.ListSnapshotsRequest_ORDER_OLDEST_FIRST, nil
	default:
		return 0, fmt.Errorf("invalid value %q for --%s. Allowed values: %s",
			s, flagSortOrder, strings.Join(allowedSorts, ", "))
	}
}
