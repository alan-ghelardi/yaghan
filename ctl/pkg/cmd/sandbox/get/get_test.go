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
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/get"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureFn lets each test case assert on the forwarded GetSandbox
// request and decide what the mocked service replies with. nil means
// the test doesn't expect the RPC to be invoked (validation failure
// before the call).
type captureFn func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error)

// fixture is the canonical sandbox the mocked api-server returns.
// Tests assert that fields from this object surface in the rendered
// output.
func fixture() *controlplanev1alpha1.Sandbox {
	return &controlplanev1alpha1.Sandbox{
		Metadata: &controlplanev1alpha1.SandboxMeta{
			Id:        "my-sandbox",
			Namespace: "default",
		},
		Resources: &controlplanev1alpha1.Resources{
			VcpuCount: 2,
			MemoryMib: 1024,
		},
		Status: &controlplanev1alpha1.SandboxStatus{
			Phase: controlplanev1alpha1.SandboxStatus_PHASE_RUNNING,
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
			args: "my-sandbox",
			capture: func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				return &controlplanev1alpha1.GetSandboxResponse{Sandbox: fixture()}, nil
			},
			// protoyaml emits camelCase keys (it does not honour the
			// protojson UseProtoNames option); protojson on the
			// cli.Marshal path is configured with
			// UseProtoNames=true and emits snake_case — that's the
			// distinguishing marker between the two formats.
			wantStdoutHas: []string{
				"metadata:",
				"id: my-sandbox",
				"resources:",
				"vcpuCount: 2",
			},
		},
		{
			name: "explicit -o yaml renders yaml",
			args: "my-sandbox -o yaml",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return &controlplanev1alpha1.GetSandboxResponse{Sandbox: fixture()}, nil
			},
			wantStdoutHas: []string{"id: my-sandbox", "vcpuCount: 2"},
		},
		{
			name: "-o json renders json with indent",
			args: "my-sandbox -o json",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return &controlplanev1alpha1.GetSandboxResponse{Sandbox: fixture()}, nil
			},
			// protojson intentionally randomises whitespace between
			// tokens (`"k": "v"` vs `"k":  "v"`) to discourage callers
			// from parsing the indented form. So we only assert on the
			// stable bits: a `{` opener, the quoted key + value
			// substrings, and the snake_case field name (UseProtoNames)
			// which separates JSON output from protoyaml's camelCase.
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"my-sandbox"`,
				`"vcpu_count"`,
				`"PHASE_RUNNING"`,
			},
		},
		{
			name:    "rejects unsupported -o text",
			args:    "my-sandbox -o text",
			wantErr: "invalid value",
		},
		{
			name:    "rejects bogus -o value",
			args:    "my-sandbox -o xml",
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
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return nil, status.Error(codes.NotFound, "sandbox not found")
			},
			wantErr: "NotFound",
		},
		{
			name: "nil sandbox in response surfaces as not-found error",
			args: "missing",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return &controlplanev1alpha1.GetSandboxResponse{}, nil
			},
			wantErr: `sandbox "missing" not found`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

			if tc.capture != nil {
				sandboxMock.EXPECT().
					GetSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.GetSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.GetSandboxResponse, error) {
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
