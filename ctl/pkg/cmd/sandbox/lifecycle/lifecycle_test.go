package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/lifecycle"
	machinerytesting "golang.nuinfra.net/ctl/pkg/machinery/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// commandWithVersionFlag returns a fresh cobra.Command with the
// version flag installed and an arbitrary RunE so cobra sees it as a
// runnable command. The tests only invoke ResolveVersion against it,
// not Execute.
func commandWithVersionFlag(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "x"}
	lifecycle.AddVersionFlag(cmd)
	return cmd
}

func TestResolveVersion_UsesFlagWhenSupplied(t *testing.T) {
	cmdCtx := machinerytesting.NewContext(t)
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)
	// No GetSandbox call expected — gomock's controller fails on any
	// unexpected invocation, so the absence of EXPECT is the assertion.

	cmd := commandWithVersionFlag(t)
	require.NoError(t, cmd.Flags().Set(lifecycle.FlagVersion, "7"))

	got, err := lifecycle.ResolveVersion(cmdCtx, cmd, "any")
	require.NoError(t, err)
	assert.Equal(t, int64(7), got)

	// Sanity: the mock would have errored if GetSandbox had been
	// called; this is just to keep the linter happy about the
	// otherwise-unused mock variable.
	_ = sandboxMock
}

func TestResolveVersion_AutoFetchesWhenFlagOmitted(t *testing.T) {
	cmdCtx := machinerytesting.NewContext(t)
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

	sandboxMock.EXPECT().
		GetSandbox(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.GetSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.GetSandboxResponse, error) {
			assert.Equal(t, "sb-1", req.GetSandboxId())
			return &controlplanev1alpha1.GetSandboxResponse{
				Sandbox: &controlplanev1alpha1.Sandbox{
					Metadata: &controlplanev1alpha1.SandboxMeta{
						Id:      "sb-1",
						Version: 12,
					},
				},
			}, nil
		})

	cmd := commandWithVersionFlag(t)
	got, err := lifecycle.ResolveVersion(cmdCtx, cmd, "sb-1")
	require.NoError(t, err)
	assert.Equal(t, int64(12), got)

	stdout := machinerytesting.Read(t, cmdCtx.IOStreams.Stdout)
	assert.Contains(t, stdout, "Reading current version of sandbox")
}

func TestResolveVersion_AutoFetchPropagatesNotFound(t *testing.T) {
	cmdCtx := machinerytesting.NewContext(t)
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

	sandboxMock.EXPECT().
		GetSandbox(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "missing"))

	cmd := commandWithVersionFlag(t)
	_, err := lifecycle.ResolveVersion(cmdCtx, cmd, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
}

func TestResolveVersion_AutoFetchHandlesNilSandbox(t *testing.T) {
	cmdCtx := machinerytesting.NewContext(t)
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

	sandboxMock.EXPECT().
		GetSandbox(gomock.Any(), gomock.Any()).
		Return(&controlplanev1alpha1.GetSandboxResponse{}, nil)

	cmd := commandWithVersionFlag(t)
	_, err := lifecycle.ResolveVersion(cmdCtx, cmd, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `sandbox "missing" not found`)
}

// Compile-time assertion that ResolveVersion's error type composes
// cleanly with errors.Is. The wrapper uses %w, so unwrapping back to
// the underlying gRPC status code remains possible from caller code.
func TestResolveVersion_ErrorIsUnwrappable(t *testing.T) {
	cmdCtx := machinerytesting.NewContext(t)
	sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

	rpcErr := status.Error(codes.Unavailable, "down")
	sandboxMock.EXPECT().
		GetSandbox(gomock.Any(), gomock.Any()).
		Return(nil, rpcErr)

	cmd := commandWithVersionFlag(t)
	_, err := lifecycle.ResolveVersion(cmdCtx, cmd, "x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, rpcErr),
		"wrapped error should still match the underlying gRPC error via errors.Is")
}
