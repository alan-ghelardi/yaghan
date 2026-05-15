package delete_test

import (
	"context"
	"strings"
	"testing"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	cpmocks "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1/mocks"
	clitesting "github.com/alan-ghelardi/yaghan/ctl/pkg/cli/testing"
	deletecmd "github.com/alan-ghelardi/yaghan/ctl/pkg/cmd/snapshot/delete"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type captureFn func(t *testing.T, req *controlplanev1alpha1.DeleteSnapshotRequest) (*controlplanev1alpha1.DeleteSnapshotResponse, error)

func TestDelete(t *testing.T) {
	tests := []struct {
		name string
		args string

		capture captureFn

		wantErr       string
		wantStdoutHas []string
	}{
		{
			name: "happy path forwards the id and prints a confirmation",
			args: "snap-1",
			capture: func(t *testing.T, req *controlplanev1alpha1.DeleteSnapshotRequest) (*controlplanev1alpha1.DeleteSnapshotResponse, error) {
				assert.Equal(t, "snap-1", req.GetSnapshotId())
				return &controlplanev1alpha1.DeleteSnapshotResponse{}, nil
			},
			wantStdoutHas: []string{`Snapshot "snap-1" deleted`},
		},
		{
			name: "rm alias works",
			args: "snap-1",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.DeleteSnapshotRequest) (*controlplanev1alpha1.DeleteSnapshotResponse, error) {
				return &controlplanev1alpha1.DeleteSnapshotResponse{}, nil
			},
			wantStdoutHas: []string{"deleted"},
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
			name: "propagates server error",
			args: "snap-1",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.DeleteSnapshotRequest) (*controlplanev1alpha1.DeleteSnapshotResponse, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
			wantErr: "delete snapshot",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			snapshotMock := cmdCtx.ClientSet.SnapshotService.(*cpmocks.MockSnapshotServiceClient)

			if tc.capture != nil {
				snapshotMock.EXPECT().
					DeleteSnapshot(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.DeleteSnapshotRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.DeleteSnapshotResponse, error) {
						return tc.capture(t, req)
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

func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
