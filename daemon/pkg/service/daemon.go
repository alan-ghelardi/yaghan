// Daemon-side gRPC handlers for the data-plane service. Each RPC
// resolves the target sandbox to a MicroVM via firecracker.Provider,
// opens a per-RPC Conversation on its vsock transport, and forwards
// the operation to the in-VM agent.
package service

import (
	"context"
	"errors"
	"io"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.nuinfra.net/agent/transport"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Exec implements [dataplanev1alpha1.DaemonServiceServer]. It expects
// the first stream message to carry an ExecProcess; subsequent client
// frames may only carry stdin chunks. Buf.validate doesn't run on
// streaming RPCs, so the handler enforces the proto's `required`
// rules by hand.
func (d *daemon) Exec(stream dataplanev1alpha1.DaemonService_ExecServer) error {
	ctx := stream.Context()

	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if first.GetSandboxId() == "" {
		return status.Error(codes.InvalidArgument, "sandbox_id is required")
	}
	if first.GetExecProcess() == nil {
		return status.Error(codes.InvalidArgument, "first Exec frame must carry an ExecProcess")
	}

	tr, err := d.transportFor(ctx, first.GetSandboxId())
	if err != nil {
		return err
	}

	conv := tr.OpenConversation()
	defer conv.Close()

	if err := conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
		Payload: &dataplanev1alpha1.AgentRequest_ExecRequest{ExecRequest: first},
	}); err != nil {
		return status.Errorf(codes.Internal, "forward Exec to agent: %v", err)
	}

	// Spawn the stdin forwarder fire-and-forget: its lifetime is bound
	// to the conversation, which the deferred Close above tears down on
	// return. Run the response loop synchronously so the handler does
	// not return — and tear the conversation down — before every
	// pending agent response has been forwarded to the client.
	go func() {
		_ = d.forwardExecStdin(ctx, stream, conv, first.GetSandboxId())
	}()
	return d.forwardExecResponses(ctx, stream, conv)
}

// forwardExecStdin reads further client frames after the initial
// ExecProcess and routes them to the agent. The client half-closing
// the stream is forwarded as a final Stdin{Eof: true} so commands
// blocking on stdin observe a clean EOF.
func (d *daemon) forwardExecStdin(
	ctx context.Context,
	stream dataplanev1alpha1.DaemonService_ExecServer,
	conv transport.Conversation,
	sandboxID string,
) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// Half-close: tell the agent's stdin pipe to drain.
			_ = conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
				Payload: &dataplanev1alpha1.AgentRequest_Stdin{
					Stdin: &dataplanev1alpha1.StdinChunk{Eof: true},
				},
			})
			return nil
		}
		if err != nil {
			return err
		}
		if got := req.GetSandboxId(); got != "" && got != sandboxID {
			return status.Errorf(codes.InvalidArgument,
				"sandbox_id %q does not match initial frame %q", got, sandboxID)
		}
		stdin := req.GetStdin()
		if stdin == nil {
			return status.Error(codes.InvalidArgument,
				"non-initial Exec frame must carry a StdinChunk")
		}
		if err := conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
			Payload: &dataplanev1alpha1.AgentRequest_Stdin{Stdin: stdin},
		}); err != nil {
			return status.Errorf(codes.Internal, "forward stdin: %v", err)
		}
	}
}

// forwardExecResponses drains the agent's responses and writes the
// matching ExecResponses to the gRPC stream. Returns nil on a clean
// terminal frame (ProcessResult), an error otherwise.
func (d *daemon) forwardExecResponses(
	ctx context.Context,
	stream dataplanev1alpha1.DaemonService_ExecServer,
	conv transport.Conversation,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-conv.Recv():
			if !ok {
				return status.Error(codes.Unavailable, "agent transport closed")
			}
			switch payload := resp.GetPayload().(type) {
			case *dataplanev1alpha1.AgentResponse_ExecResponse:
				if err := stream.Send(payload.ExecResponse); err != nil {
					return err
				}
				if payload.ExecResponse.GetProcessResult() != nil {
					// Terminal frame — exec is done.
					return nil
				}
			case *dataplanev1alpha1.AgentResponse_Error:
				return status.ErrorProto(payload.Error)
			default:
				return status.Errorf(codes.Internal,
					"unexpected agent response payload %T", payload)
			}
		}
	}
}

// UploadFile implements [dataplanev1alpha1.DaemonServiceServer]. The
// buf.validate interceptor has already enforced that sandbox_id,
// source, and dest are non-empty by the time the handler runs.
func (d *daemon) UploadFile(ctx context.Context, req *dataplanev1alpha1.UploadFileRequest) (*dataplanev1alpha1.UploadFileResponse, error) {
	tr, err := d.transportFor(ctx, req.GetSandboxId())
	if err != nil {
		return nil, err
	}
	conv := tr.OpenConversation()
	defer conv.Close()

	if err := conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
		Payload: &dataplanev1alpha1.AgentRequest_UploadFile{UploadFile: req},
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "forward UploadFile to agent: %v", err)
	}

	resp, err := awaitResponse(ctx, conv)
	if err != nil {
		return nil, err
	}
	upload := resp.GetUploadFile()
	if upload == nil {
		return nil, status.Errorf(codes.Internal,
			"unexpected agent response payload %T", resp.GetPayload())
	}
	return upload, nil
}

// DownloadFile implements [dataplanev1alpha1.DaemonServiceServer].
func (d *daemon) DownloadFile(ctx context.Context, req *dataplanev1alpha1.DownloadFileRequest) (*dataplanev1alpha1.DownloadFileResponse, error) {
	tr, err := d.transportFor(ctx, req.GetSandboxId())
	if err != nil {
		return nil, err
	}
	conv := tr.OpenConversation()
	defer conv.Close()

	if err := conv.Send(ctx, &dataplanev1alpha1.AgentRequest{
		Payload: &dataplanev1alpha1.AgentRequest_DownloadFile{DownloadFile: req},
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "forward DownloadFile to agent: %v", err)
	}

	resp, err := awaitResponse(ctx, conv)
	if err != nil {
		return nil, err
	}
	download := resp.GetDownloadFile()
	if download == nil {
		return nil, status.Errorf(codes.Internal,
			"unexpected agent response payload %T", resp.GetPayload())
	}
	return download, nil
}

// transportFor resolves a sandbox id to a vsock Transport. Maps a
// missing VM to NotFound and a failed VSock dial to Internal so the
// gRPC client can distinguish "no such sandbox" from "I have it but
// can't reach it right now".
func (d *daemon) transportFor(ctx context.Context, sandboxID string) (transport.Transport, error) {
	vm := d.firecracker.GetMicroVM(sandboxID)
	if vm == nil {
		return nil, status.Errorf(codes.NotFound, "sandbox %q not found on this node", sandboxID)
	}
	tr, err := vm.VSock()
	if err != nil {
		ctxzap.Extract(ctx).Error("daemon: vsock dial failed",
			zap.String("sandbox.id", sandboxID),
			zap.Error(err))
		if errors.Is(err, firecracker.ErrVMNotFound) {
			return nil, status.Errorf(codes.NotFound, "sandbox %q not found on this node", sandboxID)
		}
		return nil, status.Errorf(codes.Internal, "vsock dial failed: %v", err)
	}
	return tr, nil
}

// awaitResponse blocks until the agent emits the next AgentResponse on
// conv or ctx fires. An Error payload is translated into the matching
// gRPC status; a closed Recv channel surfaces as Unavailable.
func awaitResponse(ctx context.Context, conv transport.Conversation) (*dataplanev1alpha1.AgentResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-conv.Recv():
		if !ok {
			return nil, status.Error(codes.Unavailable, "agent transport closed")
		}
		if errPayload := resp.GetError(); errPayload != nil {
			return nil, status.ErrorProto(errPayload)
		}
		return resp, nil
	}
}

// Compile-time assertion that the embedded
// UnimplementedDaemonServiceServer plus our hand-written methods
// together satisfy DaemonServiceServer.
var _ dataplanev1alpha1.DaemonServiceServer = (*daemon)(nil)
