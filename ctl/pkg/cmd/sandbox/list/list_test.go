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
	"golang.nuinfra.net/ctl/pkg/cli"
	climocks "golang.nuinfra.net/ctl/pkg/cli/mocks"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/list"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sandbox is a tiny fixture builder. The fields not set here are not
// asserted on by these tests — DurationFormatter is nil-safe via
// protobuf getter conventions, so the table renderer doesn't panic on
// a zero CreatedAt.
func sandbox(id string) *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{
			Id:        id,
			Namespace: "team-alpha",
		},
		Resources: &cpv1.Resources{
			VcpuCount: 2,
			MemoryMib: 1024,
		},
		Node:   &cpv1.NodeRef{Id: "node-7"},
		Status: &cpv1.SandboxStatus{Phase: cpv1.SandboxStatus_PHASE_RUNNING},
	}
}

// captureFn lets each test inspect the forwarded ListSandboxes
// request and decide what the mocked api-server replies with. nil
// means the test does not expect the RPC to fire (validation should
// fail first).
type captureFn func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error)

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
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, "team-alpha", req.GetNamespace())
				assert.Equal(t, "", req.GetNodeId())
				assert.Equal(t, cpv1.SandboxStatus_PHASE_UNSPECIFIED, req.GetStatusPhase())
				assert.Equal(t, "", req.GetContinuationToken())
				assert.Equal(t, int32(30), req.GetPageSize())
				assert.Equal(t, cpv1.ListSandboxesRequest_ORDER_UNSPECIFIED, req.GetSortOrder())
				return &cpv1.ListSandboxesResponse{
					Sandboxes: []*cpv1.Sandbox{sandbox("sb-1")},
				}, nil
			},
			wantStdoutHas: []string{
				"Sandbox ID", "Namespace", "Node ID", "Phase", "vCPU", "Memory (MiB)",
				"sb-1", "team-alpha", "node-7", "Running",
			},
		},
		{
			name:      "--node-id alone is sufficient",
			args:      "--node-id node-7",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, "", req.GetNamespace())
				assert.Equal(t, "node-7", req.GetNodeId())
				return &cpv1.ListSandboxesResponse{
					Sandboxes: []*cpv1.Sandbox{sandbox("sb-by-node")},
				}, nil
			},
			wantStdoutHas: []string{"sb-by-node"},
		},
		{
			name:      "both --namespace and --node-id may be combined",
			args:      "--namespace team-alpha --node-id node-7",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, "team-alpha", req.GetNamespace())
				assert.Equal(t, "node-7", req.GetNodeId())
				return &cpv1.ListSandboxesResponse{
					Sandboxes: []*cpv1.Sandbox{sandbox("sb-both")},
				}, nil
			},
			wantStdoutHas: []string{"sb-both"},
		},
		{
			name:      "empty result renders nothing",
			args:      "-N team-alpha",
			usesTable: true,
			capture: func(_ *testing.T, _ *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				return &cpv1.ListSandboxesResponse{}, nil
			},
		},
		{
			name: "-o yaml renders camelCase keys and the full response",
			args: "-N team-alpha -o yaml",
			capture: func(_ *testing.T, _ *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				return &cpv1.ListSandboxesResponse{
					Sandboxes:         []*cpv1.Sandbox{sandbox("sb-1")},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutHas: []string{
				"sandboxes:",
				"id: sb-1",
				"phase: PHASE_RUNNING",
				"continuationToken: next",
			},
		},
		{
			name: "-o json renders snake_case keys",
			args: "-N team-alpha -o json",
			capture: func(_ *testing.T, _ *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				return &cpv1.ListSandboxesResponse{
					Sandboxes:         []*cpv1.Sandbox{sandbox("sb-1")},
					ContinuationToken: "next",
				}, nil
			},
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"sandboxes"`,
				`"sb-1"`,
				`"continuation_token"`,
				`"next"`,
			},
		},
		{
			name: "-c seeds the request token in json mode",
			args: "-N team-alpha -o json -c some-token",
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, "some-token", req.GetContinuationToken())
				return &cpv1.ListSandboxesResponse{}, nil
			},
		},
		{
			name:      "-c is ignored in table mode",
			args:      "-N team-alpha -c some-token",
			usesTable: true,
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, "", req.GetContinuationToken(),
					"table mode drives its own pagination; the flag must not seed the first request")
				return &cpv1.ListSandboxesResponse{
					Sandboxes: []*cpv1.Sandbox{sandbox("sb-1")},
				}, nil
			},
			wantStdoutHas: []string{"sb-1"},
		},
		{
			name: "--phase running maps to PHASE_RUNNING",
			args: "-N team-alpha -o json --phase running",
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, req.GetStatusPhase())
				return &cpv1.ListSandboxesResponse{}, nil
			},
		},
		{
			name: "--sort-order newest-first maps to ORDER_NEWEST_FIRST",
			args: "-N team-alpha -o json --sort-order newest-first",
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, cpv1.ListSandboxesRequest_ORDER_NEWEST_FIRST, req.GetSortOrder())
				return &cpv1.ListSandboxesResponse{}, nil
			},
		},
		{
			name: "--page-size is forwarded",
			args: "-N team-alpha -o json --page-size 5",
			capture: func(t *testing.T, req *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				assert.Equal(t, int32(5), req.GetPageSize())
				return &cpv1.ListSandboxesResponse{}, nil
			},
		},
		{
			name:      "--no-pagination skips the load-more prompt",
			args:      "-N team-alpha --no-pagination",
			usesTable: true,
			// hasNext=true would normally trigger KeyInput; with
			// --no-pagination set we expect no such call. The fact
			// that this test passes without registering a KeyInput
			// expectation is itself the assertion (gomock would
			// fail on an unexpected call).
			capture: func(_ *testing.T, _ *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				return &cpv1.ListSandboxesResponse{
					Sandboxes:         []*cpv1.Sandbox{sandbox("sb-1")},
					ContinuationToken: "more",
				}, nil
			},
			wantStdoutHas: []string{"sb-1"},
		},
		{
			name:    "requires one of --namespace or --node-id",
			args:    "",
			wantErr: "one of --namespace or --node-id is required",
		},
		{
			name:    "rejects bogus --phase value",
			args:    "-N team-alpha --phase wedged",
			wantErr: `invalid value "wedged" for --phase`,
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
			capture: func(_ *testing.T, _ *cpv1.ListSandboxesRequest) (*cpv1.ListSandboxesResponse, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
			wantErr: "list sandboxes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

			if tc.usesTable {
				wirePrompterForTable(t, cmdCtx)
			}

			if tc.capture != nil {
				sandboxMock.EXPECT().
					ListSandboxes(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *cpv1.ListSandboxesRequest, _ ...grpc.CallOption) (*cpv1.ListSandboxesResponse, error) {
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
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)
	prompter := cmdCtx.Prompter.(*climocks.MockPrompter)

	wirePrompterForTable(t, cmdCtx)

	gomock.InOrder(
		prompter.EXPECT().
			KeyInput(gomock.Any(), []rune{'l'}).
			Return('l', nil),
	)
	prompter.EXPECT().ClearBelow(gomock.Any()).Times(1)

	calls := 0
	sandboxMock.EXPECT().
		ListSandboxes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *cpv1.ListSandboxesRequest, _ ...grpc.CallOption) (*cpv1.ListSandboxesResponse, error) {
			calls++
			switch calls {
			case 1:
				assert.Equal(t, "", req.GetContinuationToken(), "first page must use the empty token")
				return &cpv1.ListSandboxesResponse{
					Sandboxes:         []*cpv1.Sandbox{sandbox("sb-1")},
					ContinuationToken: "page2",
				}, nil
			case 2:
				assert.Equal(t, "page2", req.GetContinuationToken(),
					"second page must reuse the token from the previous response")
				return &cpv1.ListSandboxesResponse{
					Sandboxes: []*cpv1.Sandbox{sandbox("sb-2")},
				}, nil
			default:
				t.Fatalf("unexpected ListSandboxes call #%d", calls)
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
	assert.Contains(t, stdout, "sb-1")
	assert.Contains(t, stdout, "sb-2")
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
