package list_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cmd/node/list"
	"golang.nuinfra.net/ctl/pkg/machinery"
	machinerymocks "golang.nuinfra.net/ctl/pkg/machinery/mocks"
	machinerytesting "golang.nuinfra.net/ctl/pkg/machinery/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// node is a tiny fixture builder used across cases. The fields not
// set here (CreatedAt / LastModifiedAt / Resources) are not asserted
// on by these tests — DurationFormatter is nil-safe via protobuf
// getter conventions, so the table renderer doesn't panic on them.
func node(id string, phase cpv1.NodeStatus_Phase) *cpv1.Node {
	return &cpv1.Node{
		Metadata: &cpv1.NodeMeta{Id: id},
		Status:   &cpv1.NodeStatus{Phase: phase},
	}
}

// captureFn lets each test inspect the forwarded ListNodes request
// and decide what the mocked api-server replies with. nil means the
// test does not expect the RPC to fire (validation should fail
// first).
type captureFn func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error)

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
			name:      "default output is table with headers",
			args:      "",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, "", req.GetContinuationToken())
				assert.Equal(t, int32(30), req.GetPageSize())
				assert.Equal(t, cpv1.NodeStatus_PHASE_UNSPECIFIED, req.GetStatusPhase())
				assert.Equal(t, cpv1.ListNodesRequest_ORDER_UNSPECIFIED, req.GetSortOrder())
				return &cpv1.ListNodesResponse{
					Nodes: []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
				}, nil
			},
			wantStdoutHas: []string{"Node ID", "Status", "node-1", "Healthy"},
		},
		{
			name:      "empty result renders nothing",
			args:      "",
			usesTable: true,
			capture: func(_ *testing.T, _ *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				return &cpv1.ListNodesResponse{}, nil
			},
		},
		{
			name: "-o yaml renders camelCase keys and the full response",
			args: "-o yaml",
			capture: func(_ *testing.T, _ *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				return &cpv1.ListNodesResponse{
					Nodes:             []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutHas: []string{
				"nodes:",
				"id: node-1",
				"phase: PHASE_HEALTHY",
				"continuationToken: next",
			},
		},
		{
			name: "-o json renders snake_case keys",
			args: "-o json",
			capture: func(_ *testing.T, _ *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				return &cpv1.ListNodesResponse{
					Nodes:             []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"nodes"`,
				`"node-1"`,
				`"continuation_token"`,
				`"next"`,
			},
		},
		{
			name: "-c seeds the request token in json mode",
			args: "-o json -c some-token",
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, "some-token", req.GetContinuationToken())
				return &cpv1.ListNodesResponse{}, nil
			},
		},
		{
			name:      "-c is ignored in table mode",
			args:      "-c some-token",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, "", req.GetContinuationToken(),
					"table mode drives its own pagination; the flag must not seed the first request")
				return &cpv1.ListNodesResponse{
					Nodes: []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
				}, nil
			},
			wantStdoutHas: []string{"node-1"},
		},
		{
			name: "--phase healthy maps to PHASE_HEALTHY",
			args: "-o json --phase healthy",
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, cpv1.NodeStatus_PHASE_HEALTHY, req.GetStatusPhase())
				return &cpv1.ListNodesResponse{}, nil
			},
		},
		{
			name: "--sort-order newest-first maps to ORDER_NEWEST_FIRST",
			args: "-o json --sort-order newest-first",
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, cpv1.ListNodesRequest_ORDER_NEWEST_FIRST, req.GetSortOrder())
				return &cpv1.ListNodesResponse{}, nil
			},
		},
		{
			name: "--page-size is forwarded",
			args: "-o json --page-size 5",
			capture: func(t *testing.T, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				assert.Equal(t, int32(5), req.GetPageSize())
				return &cpv1.ListNodesResponse{}, nil
			},
		},
		{
			name:      "--no-pagination skips the load-more prompt",
			args:      "--no-pagination",
			usesTable: true,
			// hasNext=true would normally trigger KeyInput; with
			// --no-pagination set we expect no such call. The fact
			// that this test passes without registering a KeyInput
			// expectation is itself the assertion (gomock would
			// fail on an unexpected call).
			capture: func(_ *testing.T, _ *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				return &cpv1.ListNodesResponse{
					Nodes:             []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
					ContinuationToken: "more",
				}, nil
			},
			wantStdoutHas: []string{"node-1"},
		},
		{
			name:    "rejects unsupported -o text",
			args:    "-o text",
			wantErr: "invalid value",
		},
		{
			name:    "rejects bogus --phase value",
			args:    "--phase wedged",
			wantErr: `invalid value "wedged" for --phase`,
		},
		{
			name:    "rejects bogus --sort-order value",
			args:    "--sort-order sideways",
			wantErr: `invalid value "sideways" for --sort-order`,
		},
		{
			name:    "rejects positional args",
			args:    "extra",
			wantErr: "unknown command",
		},
		{
			name: "propagates server error",
			args: "-o json",
			capture: func(_ *testing.T, _ *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
			wantErr: "list nodes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := machinerytesting.NewContext(t)
			clusterMock := cmdCtx.ClientSet.ClusterService.(*cpmocks.MockClusterServiceClient)

			if tc.usesTable {
				wirePrompterForTable(t, cmdCtx)
			}

			if tc.capture != nil {
				clusterMock.EXPECT().
					ListNodes(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *cpv1.ListNodesRequest, _ ...grpc.CallOption) (*cpv1.ListNodesResponse, error) {
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

			stdout := machinerytesting.Read(t, cmdCtx.IOStreams.Stdout)
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
	cmdCtx := machinerytesting.NewContext(t)
	clusterMock := cmdCtx.ClientSet.ClusterService.(*cpmocks.MockClusterServiceClient)
	prompter := cmdCtx.Prompter.(*machinerymocks.MockPrompter)

	wirePrompterForTable(t, cmdCtx)

	// Press 'l' once after the first page; ClearBelow then runs
	// before the recursive Render call fetches page 2.
	gomock.InOrder(
		prompter.EXPECT().
			KeyInput(gomock.Any(), []rune{'l'}).
			Return('l', nil),
	)
	prompter.EXPECT().ClearBelow(gomock.Any()).Times(1)

	calls := 0
	clusterMock.EXPECT().
		ListNodes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *cpv1.ListNodesRequest, _ ...grpc.CallOption) (*cpv1.ListNodesResponse, error) {
			calls++
			switch calls {
			case 1:
				assert.Equal(t, "", req.GetContinuationToken(), "first page must use the empty token")
				return &cpv1.ListNodesResponse{
					Nodes:             []*cpv1.Node{node("node-1", cpv1.NodeStatus_PHASE_HEALTHY)},
					ContinuationToken: "page2",
				}, nil
			case 2:
				assert.Equal(t, "page2", req.GetContinuationToken(),
					"second page must reuse the token from the previous response")
				return &cpv1.ListNodesResponse{
					Nodes: []*cpv1.Node{node("node-2", cpv1.NodeStatus_PHASE_HEALTHY)},
				}, nil
			default:
				t.Fatalf("unexpected ListNodes call #%d", calls)
				return nil, nil
			}
		}).Times(2)

	cmd := list.New(cmdCtx)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(cmdCtx.IOStreams.Stdout)
	cmd.SetErr(cmdCtx.IOStreams.Stderr)
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())

	stdout := machinerytesting.Read(t, cmdCtx.IOStreams.Stdout)
	assert.Contains(t, stdout, "node-1")
	assert.Contains(t, stdout, "node-2")
}

// wirePrompterForTable installs the baseline expectations every
// table render needs: Cursor() returning a mock cursor whose
// Position() resolves to a zero position. Tests that drive
// pagination layer their own KeyInput / ClearBelow expectations on
// top.
func wirePrompterForTable(t *testing.T, ctx *machinery.Context) {
	t.Helper()
	ctrl := gomock.NewController(t)
	cursor := machinerymocks.NewMockCursor(ctrl)
	cursor.EXPECT().Position().Return(machinery.Position{}, nil).AnyTimes()

	prompter := ctx.Prompter.(*machinerymocks.MockPrompter)
	prompter.EXPECT().Cursor().Return(cursor).AnyTimes()
}

// splitArgs guards against machinerytesting.SplitArgs("") returning
// []string{""}, which cobra would treat as one positional arg.
func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return machinerytesting.SplitArgs(args)
}
