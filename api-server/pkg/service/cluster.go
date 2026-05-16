package service

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/alan-ghelardi/yaghan/api-server/pkg/watch"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EstablishSession implements [cpv1.ClusterServiceServer]. It is a long-lived
// bidirectional stream:
//
//   - The client must send a ConnectionRequest as its first message; the
//     server persists / refreshes the node's row, registers a Watcher
//     filtered by the node's id, and acknowledges with the auto-generated
//     (or resumed) session id.
//   - Subsequent client messages are PatchNodeRequest (metrics / status
//     patches against the persisted node row) or UpdateSandboxRequest
//     (sandbox updates the data plane is applying). Per-message errors are
//     surfaced in-band so the daemon can retry without reconnecting.
//   - Asynchronously, sandbox events emitted via the WatchableDB are
//     forwarded to the daemon as Event messages on the same stream.
func (a *apiServer) EstablishSession(stream cpv1.ClusterService_EstablishSessionServer) error {
	ctx := stream.Context()
	logger := ctxzap.Extract(ctx)

	connect, err := awaitConnectionRequest(stream)
	if err != nil {
		return err
	}
	if connect == nil {
		// Client closed before sending any message — nothing to do.
		return nil
	}
	node := connect.GetNode()
	nodeID := node.GetMetadata().GetId()

	if err := a.registerNode(ctx, node); err != nil {
		return dbErrToStatus(ctx, "node", "register", nodeID, err)
	}

	watcher, err := newSessionWatcher(connect.GetSessionId(), nodeID)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	if err := a.eventStream.Watch(ctx, watcher); err != nil {
		logger.Error("EstablishSession: unable to register watcher",
			zap.String("node.id", nodeID),
			zap.Error(err))
		return status.Error(codes.Internal, "failed to register session")
	}

	defer func() {
		if err := a.eventStream.StopWatching(ctx, watcher.GetID()); err != nil {
			logger.Warn("EstablishSession: unable to stop watching",
				zap.Int64("session.id", watcher.GetID()),
				zap.Error(err))
		}
	}()

	send := newSafeSender(stream)

	if err := send(&cpv1.EstablishSessionResponse{
		Message: &cpv1.EstablishSessionResponse_Acknowledge{
			Acknowledge: &cpv1.ConnectionResponse{SessionId: watcher.GetID()},
		},
	}); err != nil {
		return err
	}

	logger.Info("EstablishSession: session established",
		zap.String("node.id", nodeID),
		zap.Int64("session.id", watcher.GetID()))

	// Run forwarder and receiver concurrently. The first goroutine to error
	// terminates the handler; the other unblocks shortly after as the
	// stream closes (Recv) or the deferred cleanup closes the watcher
	// (forwarder's <-watcher.Closed()).
	errCh := make(chan error, 2)
	go func() { errCh <- forwardEvents(ctx, watcher, send) }()
	go func() { errCh <- a.receiveRequests(ctx, nodeID, stream, send) }()

	select {
	case <-a.closedChan:
		return status.Error(codes.Unavailable, "api-server terminated")
	case err := <-errCh:
		return err
	}
}

// awaitConnectionRequest reads the first message from the stream and validates
// that it is a ConnectionRequest with a node id set.
func awaitConnectionRequest(stream cpv1.ClusterService_EstablishSessionServer) (*cpv1.ConnectionRequest, error) {
	req, err := stream.Recv()
	if err != nil {
		// Graceful client-side close before sending anything is not an
		// error; gRPC translates a nil return into a clean stream close.
		if errors.Is(err, io.EOF) {
			return nil, nil //nolint:nilnil // sentinel for graceful close, handled below
		}
		return nil, err
	}
	connect := req.GetConnect()
	if connect == nil {
		return nil, status.Error(codes.InvalidArgument, "the first message must be a ConnectionRequest")
	}
	if connect.GetNode().GetMetadata().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "ConnectionRequest.node.metadata.id is required")
	}
	return connect, nil
}

// newSessionWatcher builds a Watcher whose filter accepts only events for
// sandboxes assigned to the given node. If sessionID > 0 the watcher resumes
// under that id; otherwise an id is auto-generated.
func newSessionWatcher(sessionID int64, nodeID string) (*watch.Watcher[*cpv1.Event], error) {
	filter := func(e *cpv1.Event) bool {
		return e.GetSandbox().GetNode().GetId() == nodeID
	}
	if sessionID > 0 {
		return watch.NewWatcher(sessionID, filter)
	}
	return watch.NewAutoIDWatcher(filter)
}

// newSafeSender returns a closure that serialises Send calls behind a mutex.
// gRPC bidi streams permit Send concurrent with Recv but not two Sends in
// parallel; the forwarder and receiver goroutines both write responses, so
// we serialise here.
func newSafeSender(stream cpv1.ClusterService_EstablishSessionServer) func(*cpv1.EstablishSessionResponse) error {
	var mu sync.Mutex
	return func(resp *cpv1.EstablishSessionResponse) error {
		mu.Lock()
		defer mu.Unlock()
		return stream.Send(resp)
	}
}

// forwardEvents drains the watcher's channel and writes each Event to the
// stream. Returns nil on graceful close (watcher closed, ctx done) or the
// Send error that terminated the loop.
func forwardEvents(ctx context.Context, watcher *watch.Watcher[*cpv1.Event], send func(*cpv1.EstablishSessionResponse) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-watcher.Closed():
			return nil
		case msg, ok := <-watcher.MessagesChan():
			if !ok {
				return nil
			}
			err := send(&cpv1.EstablishSessionResponse{
				Message: &cpv1.EstablishSessionResponse_Event{Event: msg.Event},
			})
			// Acknowledge regardless of Send outcome so a publisher waiting
			// on the ack channel does not stall when the stream collapses.
			msg.Acknowledge()
			if err != nil {
				return err
			}
		}
	}
}

// receiveRequests handles the second-and-subsequent messages. PatchNodeRequest
// updates the persisted node row in place; UpdateSandboxRequest writes through
// the sandbox DB. Per-message errors (proto violations, version conflicts, …)
// are returned as in-band Error responses so the caller can retry without
// reconnecting. A duplicate ConnectionRequest is treated as a protocol
// violation and terminates the stream.
func (a *apiServer) receiveRequests(ctx context.Context, nodeID string, stream cpv1.ClusterService_EstablishSessionServer, send func(*cpv1.EstablishSessionResponse) error) error {
	logger := ctxzap.Extract(ctx)

	for {
		req, err := stream.Recv()
		if err != nil {
			// Half-close from the client is graceful; surface it as a
			// nil error so the handler returns cleanly.
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch op := req.GetOperation().(type) {
		case *cpv1.EstablishSessionRequest_Connect:
			return status.Error(codes.FailedPrecondition, "session is already established; do not send a second ConnectionRequest")

		case *cpv1.EstablishSessionRequest_PatchNode:
			if patchErr := a.applyNodePatch(ctx, nodeID, op.PatchNode); patchErr != nil {
				grpcErr := dbErrToStatus(ctx, "node", "patch", nodeID, patchErr)
				if err := send(errorResponse(grpcErr)); err != nil {
					return err
				}
			}

		case *cpv1.EstablishSessionRequest_UpdateSandbox:
			sb := op.UpdateSandbox.GetSandbox()
			if updateErr := a.db.Update(ctx, sb); updateErr != nil {
				grpcErr := dbErrToStatus(ctx, "sandbox", "update", sb.GetMetadata().GetId(), updateErr)
				if err := send(errorResponse(grpcErr)); err != nil {
					return err
				}
			}

		default:
			logger.Warn("EstablishSession: ignoring unknown operation",
				zap.Any("operation", op))
			if err := send(errorResponse(status.Error(codes.InvalidArgument, "unknown operation"))); err != nil {
				return err
			}
		}
	}
}

// errorResponse wraps a gRPC error in an EstablishSessionResponse with the
// {error} oneof set so it can be sent in-band on the stream.
func errorResponse(grpcErr error) *cpv1.EstablishSessionResponse {
	s, ok := status.FromError(grpcErr)
	if !ok {
		s = status.New(codes.Unknown, grpcErr.Error())
	}
	return &cpv1.EstablishSessionResponse{
		Message: &cpv1.EstablishSessionResponse_Error{Error: s.Proto()},
	}
}
