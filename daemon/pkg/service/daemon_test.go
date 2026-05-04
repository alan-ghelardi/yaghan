package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	transportmocks "golang.nuinfra.net/agent/transport/mocks"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpv1mocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	commonsconfig "golang.nuinfra.net/commons/pkg/config"
	commonsserver "golang.nuinfra.net/commons/pkg/server"
	servertesting "golang.nuinfra.net/commons/pkg/server/testing"
	"golang.nuinfra.net/daemon/pkg/config"
	fcmocks "golang.nuinfra.net/daemon/pkg/firecracker/mocks"
	netmocks "golang.nuinfra.net/daemon/pkg/network/mocks"
	"golang.nuinfra.net/daemon/pkg/service"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
)

// --- harness -------------------------------------------------------------

type harness struct {
	t        *testing.T
	ctrl     *gomock.Controller
	provider *fcmocks.MockProvider
	client   dataplanev1alpha1.DaemonServiceClient
}

// blockingClusterStream is a no-op fake that satisfies the
// ClusterService_EstablishSessionClient interface. Its sole purpose is
// to keep the controller (started inside daemon.Setup) parked on the
// initial Recv so it doesn't interfere with the gRPC tests we actually
// care about.
type blockingClusterStream struct {
	ctx context.Context
}

func (b *blockingClusterStream) Send(*cpv1.EstablishSessionRequest) error { return nil }
func (b *blockingClusterStream) Recv() (*cpv1.EstablishSessionResponse, error) {
	<-b.ctx.Done()
	return nil, b.ctx.Err()
}
func (b *blockingClusterStream) Header() (metadata.MD, error) { return nil, nil }
func (b *blockingClusterStream) Trailer() metadata.MD         { return nil }
func (b *blockingClusterStream) CloseSend() error             { return nil }
func (b *blockingClusterStream) Context() context.Context     { return b.ctx }
func (b *blockingClusterStream) SendMsg(any) error            { return nil }
func (b *blockingClusterStream) RecvMsg(any) error            { return nil }

// Compile-time assertion that the fake satisfies the bidi interface
// before we hand it to the mocked client.
var _ grpc.BidiStreamingClient[cpv1.EstablishSessionRequest, cpv1.EstablishSessionResponse] = (*blockingClusterStream)(nil)

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctrl := gomock.NewController(t)

	provider := fcmocks.NewMockProvider(ctrl)
	driver := netmocks.NewMockDriver(ctrl)
	clusterClient := cpv1mocks.NewMockClusterServiceClient(ctrl)

	// Bind the gRPC port up front so the test can dial it; the same
	// listener is handed to commonsserver.StartServer via WithListener.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	bundle := &config.Bundle{
		Base: commonsconfig.Base{
			GRPCPort: uint(port),
			KeepAlive: &commonsconfig.KeepAlive{
				MinTime:  commonsconfig.DefaultKeepAliveMinTime,
				Interval: commonsconfig.DefaultKeepAliveInterval,
				Timeout:  commonsconfig.DefaultKeepAliveTimeout,
			},
			MaxConcurrentStreams: math.MaxUint32,
		},
		AssetsDir:   "/unused-in-tests",
		Firecracker: &config.Firecracker{},
		MicroVM:     &config.MicroVM{},
		APIServer:   &config.APIServer{Address: "127.0.0.1:0"},
		Controller: &config.Controller{
			SessionIDFile:        filepath.Join(t.TempDir(), "session.id"),
			MaxRetries:           1,
			ReconnectMaxInterval: time.Second,
		},
	}

	// Park the controller's connect loop. EstablishSession may be
	// invoked at most once per controller; we hand back a stream whose
	// Recv blocks until ctx is done so the controller never sees an
	// acknowledgement and stays in the connect handshake.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	clusterClient.EXPECT().
		EstablishSession(gomock.Any()).
		DoAndReturn(func(streamCtx context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[cpv1.EstablishSessionRequest, cpv1.EstablishSessionResponse], error) {
			return &blockingClusterStream{ctx: streamCtx}, nil
		}).
		AnyTimes()

	ctx = commonsserver.WithListener(ctx, listener)
	servertesting.StartServer(ctx, t, service.New(provider, driver, clusterClient, bundle))

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return &harness{
		t:        t,
		ctrl:     ctrl,
		provider: provider,
		client:   dataplanev1alpha1.NewDaemonServiceClient(conn),
	}
}

// expectMicroVMConversation wires the full chain
//
//	provider.GetMicroVM(id) → vm.VSock() → transport.OpenConversation()
//
// returning the conversation mock plus its recv channel so a test can
// inject AgentResponses into the channel and assert against
// conv.Send. Close is allowed any number of times so a test that
// returns early is not a gomock failure.
func (h *harness) expectMicroVMConversation(sandboxID string) (*transportmocks.MockConversation, chan *dataplanev1alpha1.AgentResponse) {
	h.t.Helper()
	vm := fcmocks.NewMockMicroVM(h.ctrl)
	tr := transportmocks.NewMockTransport(h.ctrl)
	conv := transportmocks.NewMockConversation(h.ctrl)
	recvCh := make(chan *dataplanev1alpha1.AgentResponse, 16)

	h.provider.EXPECT().GetMicroVM(sandboxID).Return(vm)
	vm.EXPECT().VSock().Return(tr, nil)
	tr.EXPECT().OpenConversation().Return(conv)
	conv.EXPECT().Recv().Return((<-chan *dataplanev1alpha1.AgentResponse)(recvCh)).AnyTimes()
	conv.EXPECT().Close().Return(nil).AnyTimes()
	return conv, recvCh
}

// assertCode pulls the gRPC status off err and asserts the code.
func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok, "error is not a gRPC status: %v", err)
	assert.Equal(t, want, st.Code(), "message: %s", st.Message())
}

// --- UploadFile ----------------------------------------------------------

func TestUploadFile_RejectsEmptySandboxID(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		Source: []byte("payload"),
		Dest:   "/tmp/x",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestUploadFile_RejectsEmptySource(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-1",
		Dest:      "/tmp/x",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestUploadFile_RejectsEmptyDest(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-1",
		Source:    []byte("payload"),
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestUploadFile_NotFoundWhenVMMissing(t *testing.T) {
	h := newHarness(t)
	h.provider.EXPECT().GetMicroVM("sb-missing").Return(nil)

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-missing",
		Source:    []byte("payload"),
		Dest:      "/tmp/x",
	})
	assertCode(t, err, codes.NotFound)
}

func TestUploadFile_InternalWhenVSockFails(t *testing.T) {
	h := newHarness(t)
	vm := fcmocks.NewMockMicroVM(h.ctrl)
	h.provider.EXPECT().GetMicroVM("sb-vsock").Return(vm)
	vm.EXPECT().VSock().Return(nil, errors.New("vsock dial failed"))

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-vsock",
		Source:    []byte("payload"),
		Dest:      "/tmp/x",
	})
	assertCode(t, err, codes.Internal)
}

func TestUploadFile_HappyPath(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-ok")

	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.AgentRequest) error {
			upload := req.GetUploadFile()
			require.NotNil(t, upload, "agent must receive an UploadFileRequest")
			assert.Equal(t, "sb-ok", upload.GetSandboxId())
			assert.Equal(t, "/tmp/payload", upload.GetDest())
			assert.Equal(t, []byte("hello"), upload.GetSource())
			recvCh <- &dataplanev1alpha1.AgentResponse{
				Payload: &dataplanev1alpha1.AgentResponse_UploadFile{
					UploadFile: &dataplanev1alpha1.UploadFileResponse{},
				},
			}
			return nil
		})

	resp, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-ok",
		Source:    []byte("hello"),
		Dest:      "/tmp/payload",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestUploadFile_PropagatesAgentError(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-err")

	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *dataplanev1alpha1.AgentRequest) error {
			recvCh <- &dataplanev1alpha1.AgentResponse{
				Payload: &dataplanev1alpha1.AgentResponse_Error{
					Error: &status.Status{
						Code:    int32(codes.PermissionDenied),
						Message: "guest refused write",
					},
				},
			}
			return nil
		})

	_, err := h.client.UploadFile(t.Context(), &dataplanev1alpha1.UploadFileRequest{
		SandboxId: "sb-err",
		Source:    []byte("hello"),
		Dest:      "/tmp/x",
	})
	assertCode(t, err, codes.PermissionDenied)
}

// --- DownloadFile --------------------------------------------------------

func TestDownloadFile_RejectsEmptySandboxID(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.DownloadFile(t.Context(), &dataplanev1alpha1.DownloadFileRequest{
		Source: "/tmp/x",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestDownloadFile_RejectsEmptySource(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.DownloadFile(t.Context(), &dataplanev1alpha1.DownloadFileRequest{
		SandboxId: "sb-1",
	})
	assertCode(t, err, codes.InvalidArgument)
}

func TestDownloadFile_NotFoundWhenVMMissing(t *testing.T) {
	h := newHarness(t)
	h.provider.EXPECT().GetMicroVM("sb-missing").Return(nil)

	_, err := h.client.DownloadFile(t.Context(), &dataplanev1alpha1.DownloadFileRequest{
		SandboxId: "sb-missing",
		Source:    "/tmp/x",
	})
	assertCode(t, err, codes.NotFound)
}

func TestDownloadFile_HappyPath(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-dl")

	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.AgentRequest) error {
			download := req.GetDownloadFile()
			require.NotNil(t, download)
			assert.Equal(t, "/tmp/file.txt", download.GetSource())
			recvCh <- &dataplanev1alpha1.AgentResponse{
				Payload: &dataplanev1alpha1.AgentResponse_DownloadFile{
					DownloadFile: &dataplanev1alpha1.DownloadFileResponse{
						FileContent: []byte("file contents"),
					},
				},
			}
			return nil
		})

	resp, err := h.client.DownloadFile(t.Context(), &dataplanev1alpha1.DownloadFileRequest{
		SandboxId: "sb-dl",
		Source:    "/tmp/file.txt",
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("file contents"), resp.GetFileContent())
}

// --- Exec ---------------------------------------------------------------

func TestExec_RejectsEmptySandboxID(t *testing.T) {
	h := newHarness(t)

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/true"},
		},
	}))
	_, err = stream.Recv()
	assertCode(t, err, codes.InvalidArgument)
}

func TestExec_RejectsFirstFrameWithoutExecProcess(t *testing.T) {
	h := newHarness(t)

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-1",
		Payload: &dataplanev1alpha1.ExecRequest_Stdin{
			Stdin: &dataplanev1alpha1.StdinChunk{Data: []byte("oops")},
		},
	}))
	_, err = stream.Recv()
	assertCode(t, err, codes.InvalidArgument)
}

func TestExec_NotFoundWhenVMMissing(t *testing.T) {
	h := newHarness(t)
	h.provider.EXPECT().GetMicroVM("sb-missing").Return(nil)

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-missing",
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/true"},
		},
	}))
	_, err = stream.Recv()
	assertCode(t, err, codes.NotFound)
}

func TestExec_StreamsChunksAndProcessResult(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-exec")

	// AnyTimes because the daemon's stdin goroutine also calls Send
	// with a terminal Stdin{Eof: true} when the client closes its half
	// of the stream — that's covered by TestExec_ForwardsStdinFrames;
	// here we just want to drive the response side.
	var sendOnce sync.Once
	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.AgentRequest) error {
			sendOnce.Do(func() {
				er := req.GetExecRequest()
				require.NotNil(t, er, "first forwarded frame must be an ExecRequest")
				require.NotNil(t, er.GetExecProcess())
				assert.Equal(t, "/bin/echo", er.GetExecProcess().GetCommand())
				recvCh <- streamChunk("hello\n", dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT)
				recvCh <- streamChunk("warn\n", dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDERR)
				recvCh <- processResult(0)
			})
			return nil
		}).
		AnyTimes()

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-exec",
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{
				Command: "/bin/echo",
				Args:    []string{"hello"},
			},
		},
	}))
	require.NoError(t, stream.CloseSend())

	got := drainExecResponses(t, stream)
	require.Len(t, got, 3)
	assert.Equal(t, "hello\n", string(got[0].GetStreamChunk().GetData()))
	assert.Equal(t, dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT, got[0].GetStreamChunk().GetStream())
	assert.Equal(t, "warn\n", string(got[1].GetStreamChunk().GetData()))
	require.NotNil(t, got[2].GetProcessResult())
	assert.Equal(t, int32(0), got[2].GetProcessResult().GetExitCode())
}

func TestExec_ForwardsStdinFrames(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-stdin")

	var (
		mu       sync.Mutex
		captured []*dataplanev1alpha1.AgentRequest
	)
	eofSent := make(chan struct{})
	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.AgentRequest) error {
			mu.Lock()
			captured = append(captured, req)
			mu.Unlock()
			if s := req.GetStdin(); s != nil && s.GetEof() {
				close(eofSent)
			}
			return nil
		}).
		AnyTimes()

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-stdin",
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/cat"},
		},
	}))
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-stdin",
		Payload: &dataplanev1alpha1.ExecRequest_Stdin{
			Stdin: &dataplanev1alpha1.StdinChunk{Data: []byte("hi")},
		},
	}))
	require.NoError(t, stream.CloseSend())

	// Wait for the daemon's stdin goroutine to forward the terminal
	// EOF before we shut the response side down — otherwise the
	// response goroutine could win the errCh race and exit before the
	// stdin goroutine has finished its work.
	select {
	case <-eofSent:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not forward Stdin{Eof: true} after CloseSend")
	}

	recvCh <- processResult(0)
	_ = drainExecResponses(t, stream)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(captured), 3,
		"expected at least: ExecRequest + Stdin{Data} + Stdin{Eof}")
	require.NotNil(t, captured[0].GetExecRequest(), "first forwarded frame must be ExecRequest")
	stdinFrames := []*dataplanev1alpha1.StdinChunk{}
	for _, c := range captured[1:] {
		if s := c.GetStdin(); s != nil {
			stdinFrames = append(stdinFrames, s)
		}
	}
	require.Len(t, stdinFrames, 2, "expected one data Stdin and one EOF Stdin")
	assert.Equal(t, []byte("hi"), stdinFrames[0].GetData())
	assert.False(t, stdinFrames[0].GetEof())
	assert.Empty(t, stdinFrames[1].GetData())
	assert.True(t, stdinFrames[1].GetEof(),
		"client CloseSend must produce a terminal Stdin{Eof: true}")
}

// TestExec_ForwardsResizeFrames verifies that a ResizePTY frame sent
// after the initial ExecProcess is repackaged as AgentRequest.Resize
// and forwarded over the conversation. The agent's PTY-resize ioctl is
// out of scope for the daemon-layer test — we only assert the
// daemon's frame translation.
func TestExec_ForwardsResizeFrames(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-resize")

	var (
		mu       sync.Mutex
		captured []*dataplanev1alpha1.AgentRequest
	)
	resizeSeen := make(chan struct{})
	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dataplanev1alpha1.AgentRequest) error {
			mu.Lock()
			captured = append(captured, req)
			mu.Unlock()
			if req.GetResize() != nil {
				select {
				case <-resizeSeen:
				default:
					close(resizeSeen)
				}
			}
			return nil
		}).
		AnyTimes()

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-resize",
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/sh", Tty: true},
		},
	}))
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-resize",
		Payload: &dataplanev1alpha1.ExecRequest_Resize{
			Resize: &dataplanev1alpha1.ResizePTY{Cols: 120, Rows: 40},
		},
	}))

	// Synchronise on the daemon having actually seen the Resize.
	select {
	case <-resizeSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not forward Resize within 2s")
	}

	// Tear the stream down cleanly so the test fixture exits.
	recvCh <- processResult(0)
	require.NoError(t, stream.CloseSend())
	_ = drainExecResponses(t, stream)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured[0].GetExecRequest(),
		"first forwarded frame must be ExecRequest")
	resizeFrames := []*dataplanev1alpha1.ResizePTY{}
	for _, c := range captured[1:] {
		if r := c.GetResize(); r != nil {
			resizeFrames = append(resizeFrames, r)
		}
	}
	require.Len(t, resizeFrames, 1, "expected exactly one forwarded Resize frame")
	assert.Equal(t, uint32(120), resizeFrames[0].GetCols())
	assert.Equal(t, uint32(40), resizeFrames[0].GetRows())
}

func TestExec_PropagatesAgentError(t *testing.T) {
	h := newHarness(t)
	conv, recvCh := h.expectMicroVMConversation("sb-bad")

	conv.EXPECT().
		Send(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *dataplanev1alpha1.AgentRequest) error {
			recvCh <- &dataplanev1alpha1.AgentResponse{
				Payload: &dataplanev1alpha1.AgentResponse_Error{
					Error: &status.Status{
						Code:    int32(codes.NotFound),
						Message: "/bin/nope: no such file",
					},
				},
			}
			return nil
		})

	stream, err := h.client.Exec(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&dataplanev1alpha1.ExecRequest{
		SandboxId: "sb-bad",
		Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
			ExecProcess: &dataplanev1alpha1.ExecProcess{Command: "/bin/nope"},
		},
	}))

	for {
		_, recvErr := stream.Recv()
		if recvErr == nil {
			continue
		}
		assertCode(t, recvErr, codes.NotFound)
		return
	}
}

// --- helpers -------------------------------------------------------------

func streamChunk(data string, stream dataplanev1alpha1.StreamChunk_StreamType) *dataplanev1alpha1.AgentResponse {
	return &dataplanev1alpha1.AgentResponse{
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_StreamChunk{
					StreamChunk: &dataplanev1alpha1.StreamChunk{
						Stream: stream,
						Data:   []byte(data),
					},
				},
			},
		},
	}
}

func processResult(exitCode int32) *dataplanev1alpha1.AgentResponse {
	return &dataplanev1alpha1.AgentResponse{
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_ProcessResult{
					ProcessResult: &dataplanev1alpha1.ProcessResult{ExitCode: exitCode},
				},
			},
		},
	}
}

// drainExecResponses reads ExecResponses from stream until io.EOF.
func drainExecResponses(t *testing.T, stream grpc.BidiStreamingClient[dataplanev1alpha1.ExecRequest, dataplanev1alpha1.ExecResponse]) []*dataplanev1alpha1.ExecResponse {
	t.Helper()
	var out []*dataplanev1alpha1.ExecResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out = append(out, resp)
	}
}
