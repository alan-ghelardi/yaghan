package snapshot_test

import (
	"context"
	"strings"
	"testing"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	cpmocks "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1/mocks"
	clitesting "github.com/alan-ghelardi/yaghan/ctl/pkg/cli/testing"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/sandbox/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// startSnapshotCaptureFn lets a test case assert against the forwarded
// StartSnapshotRequest and decide what the mocked service replies.
type startSnapshotCaptureFn func(t *testing.T, req *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error)

// getCaptureFn handles the optional GetSandbox pre-fetch when --version
// is omitted.
type getCaptureFn func(t *testing.T, req *controlplanev1alpha1.GetSandboxRequest) (*controlplanev1alpha1.GetSandboxResponse, error)

func TestSnapshot(t *testing.T) {
	tests := []struct {
		name string
		args string

		// expectGet is invoked as the response to a GetSandbox call.
		// nil means GetSandbox must NOT be called.
		expectGet getCaptureFn

		// expectStartSnapshot is invoked as the response to
		// StartSnapshot. nil means StartSnapshot must NOT be called.
		expectStartSnapshot startSnapshotCaptureFn

		wantErr       string
		wantStdoutHas []string
	}{
		{
			name: "happy path with explicit --version skips the lookup, empty description",
			args: "my-sandbox --version 7",
			// expectGet is nil — gomock fails on any unexpected GetSandbox.
			expectStartSnapshot: func(t *testing.T, req *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandboxId())
				assert.Equal(t, int64(7), req.GetVersion())
				assert.Equal(t, "", req.GetDescription(),
					"description must default to empty when --description is omitted")
				return &controlplanev1alpha1.StartSnapshotResponse{}, nil
			},
			wantStdoutHas: []string{
				`Requesting snapshot for sandbox "my-sandbox" (version=7)`,
				"Snapshot requested",
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
			expectStartSnapshot: func(t *testing.T, req *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				assert.Equal(t, int64(9), req.GetVersion(),
					"StartSnapshot must use the version returned by GetSandbox")
				return &controlplanev1alpha1.StartSnapshotResponse{}, nil
			},
			wantStdoutHas: []string{
				"Reading current version of sandbox",
				"version=9",
			},
		},
		{
			name: "description flag is forwarded to the server",
			args: `my-sandbox --version 1 --description pre-deploy`,
			expectStartSnapshot: func(t *testing.T, req *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				assert.Equal(t, "pre-deploy", req.GetDescription())
				return &controlplanev1alpha1.StartSnapshotResponse{}, nil
			},
			wantStdoutHas: []string{"Snapshot requested"},
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
			name: "StartSnapshot Aborted (stale version) surfaces as an error",
			args: "my-sandbox --version 1",
			expectStartSnapshot: func(_ *testing.T, _ *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				return nil, status.Error(codes.Aborted, "version mismatch")
			},
			wantErr: "Aborted",
		},
		{
			name: "StartSnapshot FailedPrecondition (e.g. during in-flight pause) surfaces as an error",
			args: "my-sandbox --version 1",
			expectStartSnapshot: func(_ *testing.T, _ *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				return nil, status.Error(codes.FailedPrecondition, "cannot snapshot during pause")
			},
			wantErr: "FailedPrecondition",
		},
		{
			name: "server InvalidArgument (e.g. description too long) surfaces as an error",
			args: "my-sandbox --version 1 --description " + strings.Repeat("a", 257),
			expectStartSnapshot: func(_ *testing.T, _ *controlplanev1alpha1.StartSnapshotRequest) (*controlplanev1alpha1.StartSnapshotResponse, error) {
				return nil, status.Error(codes.InvalidArgument, "description too long")
			},
			wantErr: "InvalidArgument",
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
			if tc.expectStartSnapshot != nil {
				sandboxMock.EXPECT().
					StartSnapshot(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.StartSnapshotRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.StartSnapshotResponse, error) {
						return tc.expectStartSnapshot(t, req)
					})
			}

			cmd := snapshot.New(cmdCtx)
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
