package service

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	"golang.nuinfra.api-server/pkg/watch"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultPublishMaxElapsedTime is the default upper bound on retrying a single
// emitEvent publish. Past this point the data plane's periodic resync is the
// recovery mechanism, so we stop trying and let the RPC return.
const defaultPublishMaxElapsedTime = 30 * time.Second

type WatchableDB struct {
	db              sandboxdb.DB
	watchableStream watch.WatchableStream[*controlplanev1alpha1.Event]

	// publishMaxElapsedTime caps the cumulative retry duration for emitEvent.
	// Zero means use defaultPublishMaxElapsedTime.
	publishMaxElapsedTime time.Duration

	// publishBackOff returns a fresh BackOff state per emitEvent call. Nil
	// means use a default ExponentialBackOff. A factory (rather than a
	// shared instance) is required because BackOff is stateful and
	// concurrent emitEvent calls would otherwise race on its internal
	// counters.
	publishBackOff func() backoff.BackOff
}

var _ sandboxdb.DB = (*WatchableDB)(nil)

// NewWatchableDB wraps the given sandbox DB so that successful writes are also
// emitted as events on the supplied stream. emitEvent failures are logged but
// not propagated; the data plane recovers via periodic resync.
func NewWatchableDB(db sandboxdb.DB, stream watch.WatchableStream[*controlplanev1alpha1.Event]) *WatchableDB {
	return &WatchableDB{db: db, watchableStream: stream}
}

// Create implements [sandbox.DB].
func (w *WatchableDB) Create(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	if err := w.db.Create(ctx, sandbox); err != nil {
		return err
	}
	w.emitEvent(ctx, sandbox)
	return nil
}

func (w *WatchableDB) emitEvent(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) {
	event := &controlplanev1alpha1.Event{
		EmittedAt: timestamppb.Now(),
		InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
			Sandbox: sandbox,
		},
	}

	maxElapsed := w.publishMaxElapsedTime
	if maxElapsed == 0 {
		maxElapsed = defaultPublishMaxElapsedTime
	}

	var bo backoff.BackOff
	if w.publishBackOff != nil {
		bo = w.publishBackOff()
	} else {
		bo = backoff.NewExponentialBackOff()
	}

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		return struct{}{}, w.watchableStream.Publish(ctx, event)
	},
		backoff.WithBackOff(bo),
		backoff.WithMaxElapsedTime(maxElapsed),
	)
	if err != nil {
		ctxzap.Extract(ctx).Error("Unable to publish sandbox event; relying on resync to recover",
			zap.String("sandbox.id", sandbox.GetMetadata().GetId()),
			zap.Error(err),
		)
	}
}

// Get implements [sandbox.DB].
func (w *WatchableDB) Get(ctx context.Context, id string) (*controlplanev1alpha1.Sandbox, error) {
	return w.db.Get(ctx, id)
}

// List implements [sandbox.DB]. Read-only — no events are emitted.
func (w *WatchableDB) List(ctx context.Context, opts sandboxdb.ListOptions) ([]*controlplanev1alpha1.Sandbox, string, error) {
	return w.db.List(ctx, opts)
}

// Update implements [sandbox.DB].
func (w *WatchableDB) Update(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error {
	if err := w.db.Update(ctx, sandbox); err != nil {
		return err
	}
	w.emitEvent(ctx, sandbox)
	return nil
}
