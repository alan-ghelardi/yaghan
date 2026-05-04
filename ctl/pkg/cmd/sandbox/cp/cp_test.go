package cp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	dpmocks "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1/mocks"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/cp"
	"golang.nuinfra.net/ctl/pkg/machinery"
	machinerytesting "golang.nuinfra.net/ctl/pkg/machinery/testing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fixture bundles a fresh test context with the daemon mock, plus a
// closure that runs `sandbox cp <src> <dst>` against them. Cuts a
// dozen lines of boilerplate from each test case.
type fixture struct {
	ctx  *machinery.Context
	mock *dpmocks.MockDaemonServiceClient
}

func newFixture(t *testing.T) (*fixture, func(src, dst string) error) {
	t.Helper()
	cmdCtx := machinerytesting.NewContext(t)
	mock := cmdCtx.ClientSet.DaemonService.(*dpmocks.MockDaemonServiceClient)

	exec := func(src, dst string) error {
		c := cp.New(cmdCtx)
		c.SilenceErrors = true
		c.SilenceUsage = true
		c.SetOut(cmdCtx.IOStreams.Stdout)
		c.SetErr(cmdCtx.IOStreams.Stderr)
		c.SetArgs([]string{src, dst})
		return c.Execute()
	}
	return &fixture{ctx: cmdCtx, mock: mock}, exec
}

// --- argument parsing (covered through the cobra surface) --------------

func TestParseReferenceErrors(t *testing.T) {
	tests := []struct {
		name        string
		src, dst    string
		errContains string
	}{
		{
			name:        "empty source",
			src:         "",
			dst:         "sb:/foo",
			errContains: "argument cannot be empty",
		},
		{
			name:        "empty target",
			src:         "sb:/foo",
			dst:         "",
			errContains: "argument cannot be empty",
		},
		{
			name:        "empty sandbox id in source",
			src:         ":/foo",
			dst:         "./local",
			errContains: "empty sandbox id",
		},
		{
			name:        "empty path in target",
			src:         "./local",
			dst:         "sb:",
			errContains: "empty path",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, run := newFixture(t)
			err := run(tc.src, tc.dst)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

// --- direction validation ----------------------------------------------

func TestRejectsBothLocal(t *testing.T) {
	_, run := newFixture(t)
	err := run("./a", "./b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither argument references a sandbox")
}

func TestRejectsCrossSandbox(t *testing.T) {
	_, run := newFixture(t)
	err := run("sb-1:/foo", "sb-2:/bar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-sandbox copy is not supported")
}

// --- ./prefix path-vs-id disambiguation --------------------------------

// TestLocalPrefixWinsOverColon makes sure a literal local file whose
// name contains a ':' is parsed as local (driven through the
// leading-'/' rule, since absolute paths are always unambiguous). The
// upload happy path then exercises the file end-to-end.
func TestLocalPrefixWinsOverColon(t *testing.T) {
	srcDir := t.TempDir()
	// t.TempDir is always absolute on every supported OS, so the
	// leading "/" forces local interpretation regardless of the ":"
	// in the basename.
	srcPath := filepath.Join(srcDir, "weird:name.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("ok"), 0o600))

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		UploadFile(gomock.Any(), gomock.Any()).
		Return(&dataplanev1alpha1.UploadFileResponse{}, nil)

	require.NoError(t, run(srcPath, "sb-1:/srv/x.txt"))
}

// --- upload --------------------------------------------------------------

func TestUploadHappyPath(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "payload.bin")
	want := []byte("hello sandbox")
	require.NoError(t, os.WriteFile(srcPath, want, 0o600))

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		UploadFile(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.UploadFileRequest, _ ...grpc.CallOption) (*dataplanev1alpha1.UploadFileResponse, error) {
			assert.Equal(t, "sb-1", req.GetSandboxId())
			assert.Equal(t, "/srv/payload.bin", req.GetDest())
			assert.Equal(t, want, req.GetSource())
			return &dataplanev1alpha1.UploadFileResponse{}, nil
		})

	require.NoError(t, run(srcPath, "sb-1:/srv/payload.bin"))

	stdout := machinerytesting.Read(t, fix.ctx.IOStreams.Stdout)
	assert.Contains(t, stdout, "copied")
	assert.Contains(t, stdout, "sb-1:/srv/payload.bin")
}

func TestUploadAppendsBasenameForTrailingSlashDest(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "report.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("data"), 0o600))

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		UploadFile(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.UploadFileRequest, _ ...grpc.CallOption) (*dataplanev1alpha1.UploadFileResponse, error) {
			// Dest had a trailing slash; CLI must have appended basename.
			assert.Equal(t, "/srv/reports/report.txt", req.GetDest())
			return &dataplanev1alpha1.UploadFileResponse{}, nil
		})

	require.NoError(t, run(srcPath, "sb-1:/srv/reports/"))
}

func TestUploadRejectsMissingLocalSource(t *testing.T) {
	_, run := newFixture(t)
	// No EXPECT — UploadFile must NOT be called when the local read
	// fails. gomock's controller fails the test if it is.
	err := run("./does-not-exist-anywhere.txt", "sb-1:/srv/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read local source")
}

func TestUploadPropagatesServerError(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "x.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("data"), 0o600))

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		UploadFile(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.PermissionDenied, "guest refused"))

	err := run(srcPath, "sb-1:/srv/x.txt")
	require.Error(t, err)
	st, ok := status.FromError(unwrap(err))
	require.True(t, ok, "wrapped error should surface as gRPC status: %v", err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// --- download ------------------------------------------------------------

func TestDownloadHappyPath(t *testing.T) {
	dstDir := t.TempDir()
	target := filepath.Join(dstDir, "fetched.bin")

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		DownloadFile(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.DownloadFileRequest, _ ...grpc.CallOption) (*dataplanev1alpha1.DownloadFileResponse, error) {
			assert.Equal(t, "sb-1", req.GetSandboxId())
			assert.Equal(t, "/var/log/app.log", req.GetSource())
			return &dataplanev1alpha1.DownloadFileResponse{
				FileContent: []byte("from sandbox"),
			}, nil
		})

	require.NoError(t, run("sb-1:/var/log/app.log", target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, []byte("from sandbox"), got)

	stdout := machinerytesting.Read(t, fix.ctx.IOStreams.Stdout)
	assert.Contains(t, stdout, "copied sb-1:/var/log/app.log")
	assert.Contains(t, stdout, target)
}

func TestDownloadAppendsBasenameForTrailingSlashTarget(t *testing.T) {
	dstDir := t.TempDir()
	// Trailing slash → directory mode.
	dstWithSlash := dstDir + string(filepath.Separator)

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		DownloadFile(gomock.Any(), gomock.Any()).
		Return(&dataplanev1alpha1.DownloadFileResponse{
			FileContent: []byte("payload"),
		}, nil)

	require.NoError(t, run("sb-1:/srv/file.bin", dstWithSlash))

	got, err := os.ReadFile(filepath.Join(dstDir, "file.bin"))
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
	_ = fix
}

func TestDownloadAppendsBasenameForExistingDirectoryTarget(t *testing.T) {
	dstDir := t.TempDir()
	// No trailing slash, but the target IS an existing directory —
	// POSIX cp behaviour: append basename anyway.

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		DownloadFile(gomock.Any(), gomock.Any()).
		Return(&dataplanev1alpha1.DownloadFileResponse{
			FileContent: []byte("payload"),
		}, nil)

	require.NoError(t, run("sb-1:/srv/file.bin", dstDir))

	got, err := os.ReadFile(filepath.Join(dstDir, "file.bin"))
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
	_ = fix
}

func TestDownloadPropagatesServerError(t *testing.T) {
	dstDir := t.TempDir()
	target := filepath.Join(dstDir, "out.bin")

	fix, run := newFixture(t)
	fix.mock.EXPECT().
		DownloadFile(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "no such file"))

	err := run("sb-1:/nope", target)
	require.Error(t, err)
	st, ok := status.FromError(unwrap(err))
	require.True(t, ok, "wrapped error should surface as gRPC status: %v", err)
	assert.Equal(t, codes.NotFound, st.Code())

	// The local target must not have been written.
	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr),
		"download failure must not create a local file")
	_ = fix
}

// unwrap walks the error chain to find the deepest non-nil error so we
// can assert against the gRPC status that the run helper wraps with
// `download file: %w` / `upload file: %w`.
func unwrap(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}
