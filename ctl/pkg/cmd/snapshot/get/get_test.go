package get_test

import (
	"context"
	"strings"
	"testing"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	cpmocks "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1/mocks"
	clitesting "github.com/alan-ghelardi/yaghan/ctl/pkg/cli/testing"
	"github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/get"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureFn lets each test case assert on the forwarded GetSnapshot
// request and decide what the mocked service replies with. nil means
// the test doesn't expect the RPC to be invoked (validation failure
// before the call).
type captureFn func(t *testing.T, req *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error)

func fixture() *controlplanev1alpha1.Snapshot {
	return &controlplanev1alpha1.Snapshot{
		Metadata: &controlplanev1alpha1.SnapshotMeta{
			Id:          "snap-1",
			Namespace:   "team-alpha",
			Description: "nightly checkpoint",
		},
		Sandbox: &controlplanev1alpha1.SandboxRef{Id: "sb-1"},
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name string
		args string

		capture captureFn

		wantErr        string
		wantStdoutHas  []string
		wantStdoutPref string
	}{
		{
			name: "default output format is yaml",
			args: "snap-1",
			capture: func(t *testing.T, req *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error) {
				assert.Equal(t, "snap-1", req.GetSnapshotId())
				return &controlplanev1alpha1.GetSnapshotResponse{Snapshot: fixture()}, nil
			},
			wantStdoutHas: []string{
				"metadata:",
				"id: snap-1",
				"namespace: team-alpha",
				"description: nightly checkpoint",
				"sandbox:",
			},
		},
		{
			name: "describe alias works",
			args: "snap-1 -o yaml",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error) {
				return &controlplanev1alpha1.GetSnapshotResponse{Snapshot: fixture()}, nil
			},
			wantStdoutHas: []string{"id: snap-1"},
		},
		{
			name: "-o json renders json",
			args: "snap-1 -o json",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error) {
				return &controlplanev1alpha1.GetSnapshotResponse{Snapshot: fixture()}, nil
			},
			wantStdoutPref: "{",
			wantStdoutHas: []string{
				`"snap-1"`,
				`"team-alpha"`,
				`"nightly checkpoint"`,
			},
		},
		{
			name:    "rejects unsupported -o text",
			args:    "snap-1 -o text",
			wantErr: "invalid value",
		},
		{
			name:    "missing positional id is rejected",
			args:    "",
			wantErr: "accepts 1 arg(s)",
		},
		{
			name: "propagates server NotFound",
			args: "missing",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error) {
				return nil, status.Error(codes.NotFound, "snapshot not found")
			},
			wantErr: "NotFound",
		},
		{
			name: "nil snapshot in response surfaces as not-found error",
			args: "missing",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.GetSnapshotRequest) (*controlplanev1alpha1.GetSnapshotResponse, error) {
				return &controlplanev1alpha1.GetSnapshotResponse{}, nil
			},
			wantErr: `snapshot "missing" not found`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			snapshotMock := cmdCtx.ClientSet.SnapshotService.(*cpmocks.MockSnapshotServiceClient)

			if tc.capture != nil {
				snapshotMock.EXPECT().
					GetSnapshot(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.GetSnapshotRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.GetSnapshotResponse, error) {
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

func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
