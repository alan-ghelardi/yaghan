package create_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	clitesting "golang.nuinfra.net/ctl/pkg/cli/testing"
	"golang.nuinfra.net/ctl/pkg/cmd/sandbox/create"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureFn inspects the forwarded CreateSandboxRequest and returns the
// response/error the mocked SandboxService should reply with. nil means
// the test does not expect CreateSandbox to be invoked at all (e.g.
// validation errors that fail before the RPC).
type captureFn func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error)

func TestCreate(t *testing.T) {
	tests := []struct {
		name string
		args string

		// capture, when set, asserts on the forwarded request and replies
		// to it. Leave nil for cases that fail before reaching the RPC.
		capture captureFn

		wantErr    string // substring match on err.Error(); empty = success
		wantStdout string // substring match on captured stdout
	}{
		{
			name: "uses defaults when no flags or id are supplied",
			args: "",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				meta := req.GetSandbox().GetMetadata()
				resources := req.GetSandbox().GetResources()
				assert.Equal(t, "default", meta.GetNamespace())
				assert.Equal(t, uint32(1), resources.GetVcpuCount())
				assert.Equal(t, uint64(128), resources.GetMemoryMib())
				_, err := uuid.Parse(meta.GetId())
				assert.NoError(t, err, "id must be a generated UUID, got %q", meta.GetId())
				return echo(req), nil
			},
			wantStdout: "created.",
		},
		{
			name: "honours explicit id positional",
			args: "my-sandbox",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, "my-sandbox", req.GetSandbox().GetMetadata().GetId())
				return echo(req), nil
			},
			wantStdout: `"my-sandbox"`,
		},
		{
			name: "honours --namespace and --vcpu",
			args: "--namespace tenant-a --vcpu 4",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, "tenant-a", req.GetSandbox().GetMetadata().GetNamespace())
				assert.Equal(t, uint32(4), req.GetSandbox().GetResources().GetVcpuCount())
				return echo(req), nil
			},
			wantStdout: `namespace "tenant-a"`,
		},
		{
			name: "parses memory in MiB",
			args: "--memory 512MiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, uint64(512), req.GetSandbox().GetResources().GetMemoryMib())
				return echo(req), nil
			},
			wantStdout: "memory=512MiB",
		},
		{
			name: "parses memory in GiB and converts to MiB",
			args: "--memory 2GiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, uint64(2048), req.GetSandbox().GetResources().GetMemoryMib())
				return echo(req), nil
			},
			wantStdout: "memory=2048MiB",
		},
		{
			name: "honours shorthand -v / -m",
			args: "-v 4 -m 1GiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				resources := req.GetSandbox().GetResources()
				assert.Equal(t, uint32(4), resources.GetVcpuCount())
				assert.Equal(t, uint64(1024), resources.GetMemoryMib())
				return echo(req), nil
			},
			wantStdout: "vcpu=4",
		},
		{
			name:    "rejects memory with unknown unit",
			args:    "--memory 1KB",
			wantErr: `invalid memory "1KB"`,
		},
		{
			name: "parses --disk in MiB",
			args: "--disk 8192MiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, uint64(8192), req.GetSandbox().GetResources().GetDiskMib())
				return echo(req), nil
			},
			wantStdout: "disk=8192MiB",
		},
		{
			name: "parses --disk in GiB and converts to MiB",
			args: "--disk 4GiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, uint64(4096), req.GetSandbox().GetResources().GetDiskMib())
				return echo(req), nil
			},
			wantStdout: "disk=4096MiB",
		},
		{
			name: "omitted --disk leaves disk_mib at zero so the daemon picks its default",
			args: "",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.EqualValues(t, 0, req.GetSandbox().GetResources().GetDiskMib())
				return echo(req), nil
			},
			wantStdout: "disk=default",
		},
		{
			name:    "rejects --disk with unknown unit",
			args:    "--disk 1KB",
			wantErr: `invalid disk "1KB"`,
		},
		{
			name:    "rejects memory missing the unit",
			args:    "--memory 256",
			wantErr: `invalid memory "256"`,
		},
		{
			name:    "rejects more than one positional arg",
			args:    "id1 id2",
			wantErr: "accepts at most",
		},
		{
			name: "propagates server errors verbatim",
			args: "",
			capture: func(_ *testing.T, _ *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				return nil, status.Error(codes.AlreadyExists, "sandbox exists")
			},
			wantErr: "AlreadyExists",
		},
		{
			name: "--source snapshot:<id> populates Metadata.Source.SnapshotId and omits Resources",
			args: "--source snapshot:snap-1",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				src := req.GetSandbox().GetMetadata().GetSource()
				require.NotNil(t, src, "Metadata.Source must be set when --source is supplied")
				assert.Equal(t, "snap-1", src.GetSnapshotId())
				assert.Equal(t, "", src.GetImageId(),
					"snapshot source must not also set image_id")
				// Snapshot-sourced sandboxes inherit Resources from the
				// snapshot record; the CLI must NOT attach a Resources
				// proto to the request (default values would otherwise
				// be rejected by the api-server).
				assert.Nil(t, req.GetSandbox().GetResources(),
					"Resources must be nil when --source is a snapshot")
				return echo(req), nil
			},
			wantStdout: "resources inherited from snapshot",
		},
		{
			name:    "rejects --source snapshot together with explicit --vcpu",
			args:    "--source snapshot:snap-1 --vcpu 2",
			wantErr: "--vcpu/--memory/--disk cannot be set when --source is a snapshot",
		},
		{
			name:    "rejects --source snapshot together with explicit --memory",
			args:    "--source snapshot:snap-1 --memory 2GiB",
			wantErr: "--vcpu/--memory/--disk cannot be set when --source is a snapshot",
		},
		{
			name:    "rejects --source snapshot together with shorthand -v",
			args:    "-s snapshot:snap-1 -v 4",
			wantErr: "--vcpu/--memory/--disk cannot be set when --source is a snapshot",
		},
		{
			name: "--source image:<id> populates Metadata.Source.ImageId and keeps Resources",
			args: "--source image:img-1 --vcpu 2 --memory 1GiB",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				src := req.GetSandbox().GetMetadata().GetSource()
				require.NotNil(t, src)
				assert.Equal(t, "img-1", src.GetImageId())
				assert.Equal(t, "", src.GetSnapshotId())
				// Image-sourced sandboxes still attach Resources — only
				// the snapshot path inherits from the record.
				require.NotNil(t, req.GetSandbox().GetResources())
				assert.Equal(t, uint32(2), req.GetSandbox().GetResources().GetVcpuCount())
				assert.Equal(t, uint64(1024), req.GetSandbox().GetResources().GetMemoryMib())
				return echo(req), nil
			},
			wantStdout: "source=image:img-1",
		},
		{
			name: "honours shorthand -s and omits Resources for snapshot",
			args: "-s snapshot:abc",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Equal(t, "abc", req.GetSandbox().GetMetadata().GetSource().GetSnapshotId())
				assert.Nil(t, req.GetSandbox().GetResources())
				return echo(req), nil
			},
			wantStdout: "source=snapshot:abc",
		},
		{
			name: "omitted --source leaves Metadata.Source nil",
			args: "",
			capture: func(t *testing.T, req *controlplanev1alpha1.CreateSandboxRequest) (*controlplanev1alpha1.CreateSandboxResponse, error) {
				assert.Nil(t, req.GetSandbox().GetMetadata().GetSource(),
					"Metadata.Source must be nil when --source is not supplied")
				return echo(req), nil
			},
			// stdout's "Creating sandbox..." line has no source suffix when
			// the flag was omitted; the case above already asserts on
			// "created." for the success message.
		},
		{
			name:    "rejects --source value with no colon",
			args:    "--source abc",
			wantErr: `invalid --source "abc": expected format <type>:<id>`,
		},
		{
			name:    "rejects --source with empty id after the colon",
			args:    "--source snapshot:",
			wantErr: "both type and id are required",
		},
		{
			name:    "rejects --source with empty type before the colon",
			args:    "--source :abc",
			wantErr: "both type and id are required",
		},
		{
			name:    "rejects --source with unknown type",
			args:    "--source bogus:abc",
			wantErr: `unknown type "bogus"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdCtx := clitesting.NewContext(t)
			sandboxMock := cmdCtx.ClientSet.SandboxService.(*cpmocks.MockSandboxServiceClient)

			if tc.capture != nil {
				sandboxMock.EXPECT().
					CreateSandbox(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *controlplanev1alpha1.CreateSandboxRequest, _ ...grpc.CallOption) (*controlplanev1alpha1.CreateSandboxResponse, error) {
						return tc.capture(t, req)
					})
			}

			cmd := create.New(cmdCtx)
			// Silence cobra's own usage/error printing — the test asserts
			// against the error returned by Execute and the stdout we
			// wrote ourselves.
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
			if tc.wantStdout != "" {
				assert.Contains(t,
					clitesting.Read(t, cmdCtx.IOStreams.Stdout),
					tc.wantStdout)
			}
		})
	}
}

// echo replies with the same Sandbox the client sent, simulating an
// api-server that accepts the request as-is. Good enough to exercise
// the success-path output.
func echo(req *controlplanev1alpha1.CreateSandboxRequest) *controlplanev1alpha1.CreateSandboxResponse {
	return &controlplanev1alpha1.CreateSandboxResponse{Sandbox: req.GetSandbox()}
}

// splitArgs is a thin guard around clitesting.SplitArgs: the
// upstream helper returns []string{""} for an empty string, which cobra
// treats as one positional argument; we want zero.
func splitArgs(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	return clitesting.SplitArgs(args)
}
