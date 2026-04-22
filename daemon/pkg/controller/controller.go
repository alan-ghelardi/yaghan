// Package controller maintains a long-lived bidirectional stream to
// the control-plane's ClusterService.EstablishSession RPC. Inbound
// Sandbox events are stored in an in-memory indexer keyed by sandbox
// id and pushed onto a deduplicating work queue; a worker goroutine
// pops keys, hands the latest version to the reconciler, and forwards
// the resulting diff back to the api-server through the same stream.
package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/daemon/pkg/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/util/workqueue"
)

// Reconciler is the subset of *reconciler.Reconciler the controller
// depends on. Defining the interface in this package lets tests fake
// reconcile outcomes without standing up a full firecracker / network
// surface, while production keeps using the concrete type.
type Reconciler interface {
	Reconcile(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error
}

// Controller orchestrates the daemon's reconciliation loop against
// the control plane. Run is the only public entry point; all other
// methods coordinate the stream reader, the indexer, the work queue,
// and the reconciliation worker.
type Controller struct {
	client     controlplanev1alpha1.ClusterServiceClient
	reconciler Reconciler
	config     *config.Bundle

	indexer *indexer
	queue   workqueue.TypedRateLimitingInterface[string]
	session *sessionStore

	// streamMu guards stream. The reader loop swaps it on connect /
	// disconnect; the worker reads it inside sendUpdate. Send concurrent
	// with Recv on a gRPC bidi stream is safe; two concurrent Sends are
	// not, and the mutex serialises them too.
	streamMu sync.Mutex
	stream   controlplanev1alpha1.ClusterService_EstablishSessionClient
}

// New constructs a Controller bound to the given client, reconciler
// and configuration. The session-id file is read lazily on the first
// connect attempt; constructing the Controller does no I/O.
func New(client controlplanev1alpha1.ClusterServiceClient, reconciler Reconciler, config *config.Bundle) *Controller {
	return &Controller{
		client:     client,
		reconciler: reconciler,
		config:     config,
		indexer:    newIndexer(),
		queue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		session: newSessionStore(config.Controller.SessionIDFile),
	}
}

// Run blocks until ctx is cancelled. It launches a worker goroutine
// to drain the reconcile queue and runs the connect / read loop on
// the calling goroutine, reconnecting with exponential backoff
// whenever the stream drops.
func (c *Controller) Run(ctx context.Context) error {
	logger := ctxzap.Extract(ctx)

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		c.runWorker(ctx)
	}()

	defer func() {
		c.queue.ShutDown()
		<-workerDone
	}()

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		stream, err := c.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("controller: establish session: %w", err)
		}

		c.setStream(stream)
		logger.Info("controller: session established")

		c.readLoop(ctx, stream)
		c.setStream(nil)

		if ctx.Err() != nil {
			return nil
		}
		logger.Warn("controller: stream lost, reconnecting")
	}
}

// connect opens a fresh EstablishSession stream, sends the initial
// ConnectionRequest (including the persisted session id, if any), and
// blocks on the server's acknowledgement before returning. The whole
// sequence is wrapped in cenkalti/v5 exponential backoff with jitter:
// while ctx is alive we retry indefinitely.
func (c *Controller) connect(ctx context.Context) (controlplanev1alpha1.ClusterService_EstablishSessionClient, error) {
	logger := ctxzap.Extract(ctx)

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.MaxInterval = c.config.Controller.ReconnectMaxInterval

	sessionID, err := c.session.Load()
	if err != nil {
		// A corrupt session id is preferable to losing the ability to
		// connect entirely; fall back to a fresh session.
		logger.Warn("controller: unable to load session id, falling back to fresh session",
			zap.Error(err))
		sessionID = 0
	}

	opts := []backoff.RetryOption{
		backoff.WithBackOff(expBackoff),
		backoff.WithNotify(func(retryErr error, retryAfter time.Duration) {
			logger.Warn("controller: connect failed, will retry",
				zap.Error(retryErr),
				zap.Duration("retry.after", retryAfter))
		}),
	}

	return backoff.Retry(ctx, func() (controlplanev1alpha1.ClusterService_EstablishSessionClient, error) {
		stream, err := c.client.EstablishSession(ctx)
		if err != nil {
			return nil, fmt.Errorf("open stream: %w", err)
		}
		req := &controlplanev1alpha1.EstablishSessionRequest{
			Operation: &controlplanev1alpha1.EstablishSessionRequest_Connect{
				Connect: &controlplanev1alpha1.ConnectionRequest{
					SessionId: sessionID,
					Node:      hardcodedNode(),
				},
			},
		}
		if err := stream.Send(req); err != nil {
			return nil, fmt.Errorf("send ConnectionRequest: %w", err)
		}
		resp, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("await acknowledgement: %w", err)
		}
		ack := resp.GetAcknowledge()
		if ack == nil {
			return nil, fmt.Errorf("first response was not a ConnectionResponse: %T", resp.GetMessage())
		}
		if ack.GetSessionId() != sessionID {
			if err := c.session.Save(ack.GetSessionId()); err != nil {
				logger.Warn("controller: unable to persist session id",
					zap.Int64("session.id", ack.GetSessionId()),
					zap.Error(err))
			}
			sessionID = ack.GetSessionId()
		}
		logger.Info("controller: server acknowledged",
			zap.Int64("session.id", ack.GetSessionId()))
		return stream, nil
	}, opts...)
}

// readLoop drains responses from stream until it errors or ctx is
// cancelled. Events are routed into the indexer and queued; per-message
// Error responses (e.g. version conflicts) are logged and ignored —
// the api-server will re-deliver an updated Event so the indexer
// converges naturally.
func (c *Controller) readLoop(ctx context.Context, stream controlplanev1alpha1.ClusterService_EstablishSessionClient) {
	logger := ctxzap.Extract(ctx)
	for {
		resp, err := stream.Recv()
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				logger.Info("controller: server closed the stream")
			case ctx.Err() != nil || status.Code(err) == codes.Canceled:
				logger.Debug("controller: stream cancelled")
			default:
				logger.Warn("controller: stream Recv error", zap.Error(err))
			}
			return
		}
		switch msg := resp.GetMessage().(type) {
		case *controlplanev1alpha1.EstablishSessionResponse_Event:
			c.dispatch(msg.Event)
		case *controlplanev1alpha1.EstablishSessionResponse_Error:
			logger.Warn("controller: api-server reported an error",
				zap.Int32("code", msg.Error.GetCode()),
				zap.String("message", msg.Error.GetMessage()))
		case *controlplanev1alpha1.EstablishSessionResponse_Acknowledge:
			// Stray ack outside the connect handshake — ignore.
			logger.Debug("controller: received an unexpected acknowledgement")
		default:
			logger.Debug("controller: ignoring unknown response", zap.Any("message", msg))
		}
	}
}

// dispatch routes an Event into the indexer and onto the work queue.
// Older versions are dropped silently — the indexer's Put returns
// false in that case so we do not even enqueue.
func (c *Controller) dispatch(event *controlplanev1alpha1.Event) {
	sandbox := event.GetSandbox()
	if sandbox == nil {
		return
	}
	if c.indexer.Put(sandbox) {
		c.queue.Add(sandbox.GetMetadata().GetId())
	}
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNext(ctx) {
	}
}

func (c *Controller) processNext(ctx context.Context) bool {
	id, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(id)

	logger := ctxzap.Extract(ctx).With(zap.String("sandbox.id", id))
	if err := c.processItem(ctx, id); err != nil {
		retries := c.queue.NumRequeues(id)
		if retries < c.config.Controller.MaxRetries {
			logger.Warn("controller: reconcile failed, requeueing",
				zap.Error(err),
				zap.Int("attempt", retries+1))
			c.queue.AddRateLimited(id)
			return true
		}
		logger.Error("controller: reconcile gave up after max retries",
			zap.Error(err),
			zap.Int("attempts", retries+1))
		c.markFailed(ctx, id, err)
		c.queue.Forget(id)
		return true
	}
	c.queue.Forget(id)
	return true
}

// processItem runs a single reconciliation pass for id. It clones the
// indexed sandbox so the original is preserved for diff comparison,
// invokes the reconciler against the clone, and forwards the result
// to the api-server when it differs from the original.
func (c *Controller) processItem(ctx context.Context, id string) error {
	stored := c.indexer.Get(id)
	if stored == nil {
		// The indexer was emptied between Add and Get (terminal phase
		// or out-of-order delete). Nothing to reconcile.
		return nil
	}

	updated := proto.CloneOf(stored)
	if err := c.reconciler.Reconcile(ctx, updated); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	if proto.Equal(stored, updated) {
		return nil
	}

	if err := c.sendUpdate(updated); err != nil {
		return fmt.Errorf("send update: %w", err)
	}

	// The api-server will re-emit the canonical Sandbox event after
	// applying the update; advance the local copy so subsequent diffs
	// don't keep firing on the same achieved state.
	c.indexer.Put(updated)
	return nil
}

// markFailed pushes a PHASE_FAILED status back to the api-server with
// the reconcile error in the message field, after the controller has
// exhausted its retry budget for a sandbox. Best-effort: a failure to
// send is logged and dropped — we have already given up on the work
// item.
func (c *Controller) markFailed(ctx context.Context, id string, reconcileErr error) {
	logger := ctxzap.Extract(ctx)
	stored := c.indexer.Get(id)
	if stored == nil {
		return
	}
	failed := proto.CloneOf(stored)
	failed.Status = &controlplanev1alpha1.SandboxStatus{
		Phase:   controlplanev1alpha1.SandboxStatus_PHASE_FAILED,
		Message: reconcileErr.Error(),
	}
	if err := c.sendUpdate(failed); err != nil {
		logger.Error("controller: unable to report PHASE_FAILED",
			zap.String("sandbox.id", id),
			zap.Error(err))
	}
}

// sendUpdate forwards a Sandbox to the api-server as an UpdateSandbox
// over the current stream. Returns an error when the stream is not
// connected; the worker re-enqueues in that case.
func (c *Controller) sendUpdate(sandbox *controlplanev1alpha1.Sandbox) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.stream == nil {
		return errors.New("stream is not connected")
	}
	return c.stream.Send(&controlplanev1alpha1.EstablishSessionRequest{
		Operation: &controlplanev1alpha1.EstablishSessionRequest_UpdateSandbox{
			UpdateSandbox: &controlplanev1alpha1.UpdateSandboxRequest{Sandbox: sandbox},
		},
	})
}

func (c *Controller) setStream(stream controlplanev1alpha1.ClusterService_EstablishSessionClient) {
	c.streamMu.Lock()
	c.stream = stream
	c.streamMu.Unlock()
}

// hardcodedNode is a placeholder identity for the daemon's host.
// TODO: derive from /etc/hostname / /proc/cpuinfo / config once the
// node-discovery story is settled.
func hardcodedNode() *controlplanev1alpha1.Node {
	return &controlplanev1alpha1.Node{
		Metadata: &controlplanev1alpha1.NodeMeta{Id: "node-local"},
		Resources: &controlplanev1alpha1.NodeResources{
			VcpuCount: 4,
			MemoryMib: 8192,
		},
	}
}
