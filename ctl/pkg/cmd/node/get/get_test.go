package get_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cmd/node/get"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureFn lets each test case assert on the forwarded GetNode
// request and decide what the mocked service replies with. nil means
// the test doesn't expect the RPC to be invoked (validation failure
// before the call).
type captureFn func(t *testing.T, req *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error)

// fixture is the canonical node the mocked api-server returns. Tests
// assert that fields from this object surface in the rendered output.
func fixture() *controlplanev1alpha1.Node {
	return &controlplanev1alpha1.Node{
		Metadata: &controlplanev1alpha1.NodeMeta{
			Id: "node-1",
		},
		Resources: &controlplanev1alpha1.NodeResources{
			CpuCapacityMillicores: 8000,
			MemoryCapacityBytes:   16 * 1024 * 1024,
		},
		Status: &controlplanev1alpha1.NodeStatus{
			Phase: controlplanev1alpha1.NodeStatus_PHASE_HEALTHY,
		},
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name string
		args string

		capture captureFn

		wantErr        string   // substring match on err.Error()
		wantStdoutHas  []string // every entry must be a substring of stdout
		wantStdoutPref string   // optional: stdout must start with this
	}{
		{
			name: "default output format is yaml",
			args: "node-1",
			capture: func(t *testing.T, req *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error) {
				assert.Equal(t, "node-1", req.GetNodeId())
				return &controlplanev1alpha1.GetNodeResponse{Node: fixture()}, nil
			},
			// protoyaml emits camelCase keys (it does not honour the
			// protojson UseProtoNames option); protojson on the
			// cli.Marshal path is configured with
			// UseProtoNames=true and emits snake_case — that's the
			// distinguishing marker between the two formats.
			wantStdoutHas: []string{
				"metadata:",
				"id: node-1",
				"resources:",
				"cpuCapacityMillicores: 8000",
				"phase: PHASE_HEALTHY",
			},
		},
		{
			name: "explicit -o yaml renders yaml",
			args: "node-1 -o yaml",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error) {
				return &controlplanev1alpha1.GetNodeResponse{Node: fixture()}, nil
			},
			wantStdoutHas: []string{"id: node-1", "cpuCapacityMillicores: 8000"},
		},
		{
			name: "-o json renders json with indent",
			args: "node-1 -o json",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error) {
				return &controlplanev1alpha1.GetNodeResponse{Node: fixture()}, nil
			},
			// protojson intentionally randomises whitespace between
			// tokens (`"k": "v"` vs `"k":  "v"`) to discourage callers
			// from parsing the indented form. So we only assert on the
			// stable bits: a `{` opener, the quoted key + value
			// substrings, and the snake_case field name (UseProtoNames)
			// which separates JSON output from protoyaml's camelCase.
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"node-1"`,
				`"cpu_capacity_millicores"`,
				`"PHASE_HEALTHY"`,
			},
		},
		{
			name:    "rejects unsupported -o text",
			args:    "node-1 -o text",
			wantErr: "invalid value",
		},
		{
			name:    "rejects bogus -o value",
			args:    "node-1 -o xml",
			wantErr: "invalid value",
		},
		{
			name:    "missing positional id is rejected",
			args:    "",
			wantErr: "accepts 1 arg(s)",
		},
		{
			name:    "rejects extra positional args",
			args:    "id1 id2",
			wantErr: "accepts 1 arg(s)",
		},
		{
			name: "propagates server NotFound",
			args: "missing",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error) {
				return nil, status.Error(codes.NotFound, "node not found")
			},
			wantErr: "NotFound",
		},
		{
			name: "nil node in response surfaces as not-found error",
			args: "missing",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetNodeRequest) (*controlplanev1alpha1.GetNodeResponse, error) {
				return &controlplanev1alpha1.GetNodeResponse{}, nil
			},
			wantErr: `node "missing" not found`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			clusterMock := cmdCtx.ClientSet.ClusterService.(*cpmocks.MockClusterServiceClient)

			if tc.capture != nil {
				clusterMock.EXPECT().
					GetNode(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.GetNodeRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.GetNodeResponse, error) {
						return tc.capture(t, req)
					})
			}

			cmd := get.New(cmdCtx)
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

// splitArgs guards against clitesting.SplitArgs("") returning
// []string{""}, which cobra would treat as one positional arg.
func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
