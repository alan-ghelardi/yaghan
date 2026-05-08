package resume_test

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
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/resume"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type resumeCaptureFn func(t *testing.T, req *controlplanev1alpha1.ResumeSandboxRequest) (*controlplanev1alpha1.ResumeSandboxResponse, error)
type getCaptureFn func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error)

func TestResume(t *testing.T) {
	tests := []struct {
		name string
		args string

		expectGet    getCaptureFn
		expectResume resumeCaptureFn

		wantErr       string
		wantStdoutHas []string
	}{
		{
			name: "happy path with explicit --version skips the lookup",
			args: "my-sandbox --version 4",
			expectResume: func(t *testing.T, req *controlplanev1alpha1.ResumeSandboxRequest) (*controlplanev1alpha1.ResumeSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				assert.Equal(t, int64(4), req.GetVersion())
				return &controlplanev1alpha1.ResumeSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				`Resuming sandbox "my-sandbox" (version=4)`,
				"resume requested",
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
							Version: 11,
						},
					},
				}, nil
			},
			expectResume: func(t *testing.T, req *controlplanev1alpha1.ResumeSandboxRequest) (*controlplanev1alpha1.ResumeSandboxResponse, error) {
				assert.Equal(t, int64(11), req.GetVersion())
				return &controlplanev1alpha1.ResumeSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				"Reading current version of sandbox",
				"version=11",
			},
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
			name: "ResumeSandbox FailedPrecondition surfaces as an error",
			args: "my-sandbox --version 1",
			expectResume: func(_ *testing.T, _ *controlplanev1alpha1.ResumeSandboxRequest) (*controlplanev1alpha1.ResumeSandboxResponse, error) {
				return nil, status.Error(codes.FailedPrecondition, "not paused")
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
			if tc.expectResume != nil {
				sandboxMock.EXPECT().
					ResumeSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.ResumeSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.ResumeSandboxResponse, error) {
						return tc.expectResume(t, req)
					})
			}

			cmd := resume.New(cmdCtx)
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

func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
