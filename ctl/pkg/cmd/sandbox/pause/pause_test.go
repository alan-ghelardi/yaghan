package pause_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/pause"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pauseCaptureFn lets a test case assert against the forwarded
// PauseSandboxRequest and decide what the mocked service replies.
type pauseCaptureFn func(t *testing.T, req *controlplanev1alpha1.PauseSandboxRequest) (*controlplanev1alpha1.PauseSandboxResponse, error)

// getCaptureFn handles the optional GetSandbox pre-fetch when --version
// is omitted.
type getCaptureFn func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error)

func TestPause(t *testing.T) {
	tests := []struct {
		name string
		args string

		// expectGet is invoked as the response to a GetSandbox call.
		// nil means GetSandbox must NOT be called.
		expectGet getCaptureFn

		// expectPause is invoked as the response to PauseSandbox.
		// nil means PauseSandbox must NOT be called.
		expectPause pauseCaptureFn

		wantErr       string
		wantStdoutHas []string
	}{
		{
			name: "happy path with explicit --version skips the lookup",
			args: "my-sandbox --version 7",
			// expectGet is nil — gomock fails on any unexpected GetSandbox.
			expectPause: func(t *testing.T, req *controlplanev1alpha1.PauseSandboxRequest) (*controlplanev1alpha1.PauseSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				assert.Equal(t, int64(7), req.GetVersion())
				return &controlplanev1alpha1.PauseSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				`Pausing sandbox "my-sandbox" (version=7)`,
				"pause requested",
			},
		},
		{
			name: "auto-fetches version when --version is omitted",
			args: "my-sandbox",
			expectGet: func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				return &controlplanev1alpha1.GetSandboxResponse{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{
							Id:      "my-sandbox",
							Version: 9,
						},
					},
				}, nil
			},
			expectPause: func(t *testing.T, req *controlplanev1alpha1.PauseSandboxRequest) (*controlplanev1alpha1.PauseSandboxResponse, error) {
				assert.Equal(t, int64(9), req.GetVersion(),
					"PauseSandbox must use the version returned by GetSandbox")
				return &controlplanev1alpha1.PauseSandboxResponse{}, nil
			},
			wantStdoutHas: []string{
				"Reading current version of sandbox",
				"version=9",
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
			name: "PauseSandbox FailedPrecondition (stale version) surfaces as an error",
			args: "my-sandbox --version 1",
			expectPause: func(_ *testing.T, _ *controlplanev1alpha1.PauseSandboxRequest) (*controlplanev1alpha1.PauseSandboxResponse, error) {
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
			if tc.expectPause != nil {
				sandboxMock.EXPECT().
					PauseSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.PauseSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.PauseSandboxResponse, error) {
						return tc.expectPause(t, req)
					})
			}

			cmd := pause.New(cmdCtx)
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
