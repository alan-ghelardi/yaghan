package list_test

import (
	"context"
	"strings"
	"testing"

	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	cpmocks "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1/mocks"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	climocks "github.com/alan-ghelardi/yaghan/ctl/pkg/cli/mocks"
	clitesting "github.com/alan-ghelardi/yaghan/ctl/pkg/cli/testing"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/list"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// snapshot is a tiny fixture builder. The fields not set here are not
// asserted on by these tests — the renderer's DurationFormatter is
// nil-safe via protobuf getter conventions, so the table renderer
// doesn't panic on a zero CreatedAt.
func snapshot(id string) *cpv1.Snapshot {
	return &cpv1.Snapshot{
		Metadata: &cpv1.SnapshotMeta{
			Id:          id,
			Namespace:   "team-alpha",
			Description: "nightly",
		},
		Sandbox: &cpv1.SandboxRef{Id: "sb-1"},
	}
}

// captureFn lets each test inspect the forwarded ListSnapshots request
// and decide what the mocked api-server replies with. nil means the
// test does not expect the RPC to fire (validation should fail first).
type captureFn func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error)

func TestList(t *testing.T) {
	tests := []struct {
		name string
		args string

		capture captureFn

		// usesTable controls whether the test wires up baseline
		// Prompter.Cursor() expectations. Non-table cases (json/yaml,
		// validation failures) leave the prompter untouched.
		usesTable bool

		wantErr        string
		wantStdoutHas  []string
		wantStdoutPref string
	}{
		{
			name:      "default output is table with headers when filtered by namespace",
			args:      "--namespace team-alpha",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				assert.Equal(t, "team-alpha", req.GetNamespace())
				assert.Equal(t, "", req.GetSandboxId())
				assert.Equal(t, "", req.GetContinuationToken())
				assert.Equal(t, int32(30), req.GetPageSize())
				assert.Equal(t, cpv1.ListSnapshotsRequest_ORDER_UNSPECIFIED, req.GetSortOrder())
				return &cpv1.ListSnapshotsResponse{
					Snapshots: []*cpv1.Snapshot{snapshot("snap-1")},
				}, nil
			},
			wantStdoutHas: []string{"Snapshot ID", "Namespace", "Sandbox ID", "snap-1", "team-alpha", "sb-1"},
		},
		{
			name:      "--sandbox-id forwards the filter",
			args:      "--sandbox-id sb-42",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				assert.Equal(t, "", req.GetNamespace())
				assert.Equal(t, "sb-42", req.GetSandboxId())
				return &cpv1.ListSnapshotsResponse{
					Snapshots: []*cpv1.Snapshot{snapshot("snap-bs-1")},
				}, nil
			},
			wantStdoutHas: []string{"snap-bs-1"},
		},
		{
			name: "-o yaml renders the full response",
			args: "-N team-alpha -o yaml",
			capture: func(_ *testing.T, _ *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				return &cpv1.ListSnapshotsResponse{
					Snapshots:         []*cpv1.Snapshot{snapshot("snap-1")},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutHas: []string{
				"snapshots:",
				"id: snap-1",
				"continuationToken: next",
			},
		},
		{
			name: "-o json renders snake_case keys",
			args: "-N team-alpha -o json",
			capture: func(_ *testing.T, _ *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				return &cpv1.ListSnapshotsResponse{
					Snapshots:         []*cpv1.Snapshot{snapshot("snap-1")},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"snapshots"`,
				`"snap-1"`,
				`"continuation_token"`,
				`"next"`,
			},
		},
		{
			name: "-c seeds the request token in json mode",
			args: "-N team-alpha -o json -c some-token",
			capture: func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				assert.Equal(t, "some-token", req.GetContinuationToken())
				return &cpv1.ListSnapshotsResponse{}, nil
			},
		},
		{
			name: "--sort-order newest-first maps to ORDER_NEWEST_FIRST",
			args: "-N team-alpha -o json --sort-order newest-first",
			capture: func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				assert.Equal(t, cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST, req.GetSortOrder())
				return &cpv1.ListSnapshotsResponse{}, nil
			},
		},
		{
			name: "--page-size is forwarded",
			args: "-N team-alpha -o json --page-size 5",
			capture: func(t *testing.T, req *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				assert.Equal(t, int32(5), req.GetPageSize())
				return &cpv1.ListSnapshotsResponse{}, nil
			},
		},
		{
			name:    "requires one of --namespace or --sandbox-id",
			args:    "",
			wantErr: "one of --namespace or --sandbox-id is required",
		},
		{
			name:    "rejects both --namespace and --sandbox-id",
			args:    "--namespace team-alpha --sandbox-id sb-1",
			wantErr: "--namespace and --sandbox-id are mutually exclusive",
		},
		{
			name:    "rejects bogus --sort-order value",
			args:    "-N team-alpha --sort-order sideways",
			wantErr: `invalid value "sideways" for --sort-order`,
		},
		{
			name:    "rejects unsupported -o text",
			args:    "-N team-alpha -o text",
			wantErr: "invalid value",
		},
		{
			name:    "rejects positional args",
			args:    "-N team-alpha extra",
			wantErr: "unknown command",
		},
		{
			name: "propagates server error",
			args: "-N team-alpha -o json",
			capture: func(_ *testing.T, _ *cpv1.ListSnapshotsRequest) (*cpv1.ListSnapshotsResponse, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
			wantErr: "list snapshots",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			snapshotMock := cmdCtx.ClientSet.SnapshotService.(*cpmocks.MockSnapshotServiceClient)

			if tc.usesTable {
				wirePrompterForTable(t, cmdCtx)
			}

			if tc.capture != nil {
				snapshotMock.EXPECT().
					ListSnapshots(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *cpv1.ListSnapshotsRequest, _ ...grpc.CallOption) (*cpv1.ListSnapshotsResponse, error) {
						return tc.capture(t, req)
					})
			}

			cmd := list.New(cmdCtx)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(cmdCtx.IOStreams.Stdout)
			cmd.SetErr(cmdCtx.IOStreams.Stderr)
			cmd.SetArgs(splitArgs(tc.args))

			err := cmd.Execute()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)

			stdout := clitesting.Read(t, cmdCtx.IOStreams.Stdout)
			if tc.wantStdoutPref != "" {
				assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), tc.wantStdoutPref),
					"stdout %q must start with %q", stdout, tc.wantStdoutPref)
			}
			for _, want := range tc.wantStdoutHas {
				assert.Contains(t, stdout, want)
			}
		})
	}
}

// TestList_Pagination exercises the interactive table loop: page 1
// returns a continuation token, the user presses 'l', and page 2
// finishes the listing.
func TestList_Pagination(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	snapshotMock := cmdCtx.ClientSet.SnapshotService.(*cpmocks.MockSnapshotServiceClient)
	prompter := cmdCtx.Prompter.(*climocks.MockPrompter)

	wirePrompterForTable(t, cmdCtx)

	gomock.InOrder(
		prompter.EXPECT().
			KeyInput(gomock.Any(), []rune{'l'}).
			Return('l', nil),
	)
	prompter.EXPECT().ClearBelow(gomock.Any()).Times(1)

	calls := 0
	snapshotMock.EXPECT().
		ListSnapshots(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *cpv1.ListSnapshotsRequest, _ ...grpc.CallOption) (*cpv1.ListSnapshotsResponse, error) {
			calls++
			switch calls {
			case 1:
				assert.Equal(t, "", req.GetContinuationToken(), "first page must use the empty token")
				return &cpv1.ListSnapshotsResponse{
					Snapshots:         []*cpv1.Snapshot{snapshot("snap-1")},
					ContinuationToken: "page2",
				}, nil
			case 2:
				assert.Equal(t, "page2", req.GetContinuationToken(),
					"second page must reuse the token from the previous response")
				return &cpv1.ListSnapshotsResponse{
					Snapshots: []*cpv1.Snapshot{snapshot("snap-2")},
				}, nil
			default:
				t.Fatalf("unexpected ListSnapshots call #%d", calls)
				return nil, nil
			}
		}).Times(2)

	cmd := list.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(cmdCtx.IOStreams.Stdout)
	cmd.SetErr(cmdCtx.IOStreams.Stderr)
	cmd.SetArgs([]string{"--namespace", "team-alpha"})

	require.NoError(t, cmd.Execute())

	stdout := clitesting.Read(t, cmdCtx.IOStreams.Stdout)
	assert.Contains(t, stdout, "snap-1")
	assert.Contains(t, stdout, "snap-2")
}

// wirePrompterForTable installs the baseline expectations every table
// render needs: Cursor() returning a mock cursor whose Position()
// resolves to a zero position. Tests that drive pagination layer their
// own KeyInput / ClearBelow expectations on top.
func wirePrompterForTable(t *testing.T, ctx *cli.Context) {
	t.Helper()
	ctrl := gomock.NewController(t)
	cursor := climocks.NewMockCursor(ctrl)
	cursor.EXPECT().Position().Return(cli.Position{}, nil).AnyTimes()

	prompter := ctx.Prompter.(*climocks.MockPrompter)
	prompter.EXPECT().Cursor().Return(cursor).AnyTimes()
}

// splitArgs guards against clitesting.SplitArgs("") returning
// []string{""}, which cobra would treat as one positional arg.
func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
