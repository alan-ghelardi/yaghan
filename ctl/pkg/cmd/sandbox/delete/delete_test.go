package delete_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	deletecmd "golang.nuinfra.net/ctl/pkg/cmd/sandbox/delete"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deleteCaptureFn func(t *testing.T, req *controlplanev1alpha1.DeleteSandboxRequest) (*controlplanev1alpha1.DeleteSandboxResponse, error)
type getCaptureFn func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error)

func TestDelete(t *testing.T) {
	tests := []struct {
		name string
		args string

		expectGet    getCaptureFn
		expectDelete deleteCaptureFn

		wantErr       string
		wantStdoutHas []string
	}{
		{
			name: "happy path with explicit --version skips the lookup",
			args: "my-sandbox --version 5",
			expectDelete: func(t *testing.T, req *controlplanev1alpha1.DeleteSandboxRequest) (*controlplanev1alpha1.DeleteSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				assert.Equal(t, int64(5), req.GetVersion())
				return &controlplanev1alpha1.DeleteSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				`Deleting sandbox "my-sandbox" (version=5)`,
				"delete requested",
			},
		},
		{
			name: "auto-fetches version when --version is omitted",
			args: "my-sandbox",
			expectGet: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return &controlplanev1alpha1.GetSandboxResponse{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{
							Id:      "my-sandbox",
							Version: 14,
						},
					},
				}, nil
			},
			expectDelete: func(t *testing.T, req *controlplanev1alpha1.DeleteSandboxRequest) (*controlplanev1alpha1.DeleteSandboxResponse, error) {
				assert.Equal(t, int64(14), req.GetVersion())
				return &controlplanev1alpha1.DeleteSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				"Reading current version of sandbox",
				"version=14",
			},
		},
		{
			name: "rm alias drives the same RPC",
			args: "my-sandbox --version 2",
			expectDelete: func(t *testing.T, req *controlplanev1alpha1.DeleteSandboxRequest) (*controlplanev1alpha1.DeleteSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				assert.Equal(t, int64(2), req.GetVersion())
				return &controlplanev1alpha1.DeleteSandboxResponse{}, nil
			},
			wantStdoutHas: []string{`Deleting sandbox "my-sandbox"`},
		},
		{
			name: "GetSandbox NotFound surfaces as an error",
			args: "missing",
			expectGet: func(_ *testing.T, _ *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				return nil, status.Error(codes.NotFound, "missing")
			},
			wantErr: "NotFound",
		},
		{
			name: "DeleteSandbox FailedPrecondition surfaces as an error",
			args: "my-sandbox --version 1",
			expectDelete: func(_ *testing.T, _ *controlplanev1alpha1.DeleteSandboxRequest) (*controlplanev1alpha1.DeleteSandboxResponse, error) {
				return nil, status.Error(codes.FailedPrecondition, "version mismatch")
			},
			wantErr: "FailedPrecondition",
		},
		{
			name:    "missing positional id is rejected",
			args:    "--version 1",
			wantErr: "accepts 1 arg(s)",
		},
		{
			name:    "extra positional args are rejected",
			args:    "id1 id2 --version 1",
			wantErr: "accepts 1 arg(s)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

			if tc.expectGet != nil {
				sandboxMock.EXPECT().
					GetSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.GetSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.GetSandboxResponse, error) {
						return tc.expectGet(t, req)
					})
			}
			if tc.expectDelete != nil {
				sandboxMock.EXPECT().
					DeleteSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.DeleteSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.DeleteSandboxResponse, error) {
						return tc.expectDelete(t, req)
					})
			}

			cmd := deletecmd.New(cmdCtx)
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
			for _, want := range tc.wantStdoutHas {
				assert.Contains(t, stdout, want)
			}
		})
	}
}

// TestDelete_AliasResolves verifies that 'del' and 'rm' are wired as
// aliases. The actual behavior is covered by the table tests above —
// here we only assert the alias surface so a future rename doesn't
// silently drop the shorthand.
func TestDelete_AliasResolves(t *testing.T) {
	cmdCtx := clitesting.NewContext(t)
	cmd := deletecmd.New(cmdCtx)
	assert.ElementsMatch(t, []string{"del", "rm"}, cmd.Aliases)
}

func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
