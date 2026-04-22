package watch

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

const (
	// Size of the buffer used for exchanging messages.
	defaultBufferCapacity = 100
)

var (
	ErrWatcherClosed    = errors.New("watcher has been closed and cannot receive further events")
	ErrInvalidWatcherID = errors.New("watcher id must be greater than zero")
)

// Message is the wrapper used to transport events from a WatchableStream to a Watcher.
type Message[E proto.Message] struct {
	acknowledgeChan chan<- struct{}
	closeOnce       *sync.Once
	Event           E
}

// Acknowledge indicates that the message has been delivered and processed. It is
// safe to call multiple times; only the first call closes the underlying
// channel.
func (m *Message[E]) Acknowledge() {
	if m.acknowledgeChan == nil || m.closeOnce == nil {
		return
	}
	m.closeOnce.Do(func() {
		close(m.acknowledgeChan)
	})
}

// FilterFunc takes an Event and returns true if it should be delivered to a
// given Watcher.
type FilterFunc[E proto.Message] func(event E) bool

// and combines various FilterFuncs in a logical conjunction. Panics are not
// recovered here; the Watcher recovers at the Receive boundary so the panic
// can be logged with watcher context.
func and[E proto.Message](filters ...FilterFunc[E]) FilterFunc[E] {
	return func(event E) bool {
		for _, filter := range filters {
			if !filter(event) {
				return false
			}
		}
		return true
	}
}

// Watcher represents a client that subscribes to events triggered by changes in the system.
type Watcher[E proto.Message] struct {
	// Identifier of this Watcher.
	id int64

	// Filter to restrict the events that this Watcher will receive.
	filter FilterFunc[E]

	// Channel for delivering messages.
	messagesChan chan *Message[E]

	// Channel that indicates if the Watcher has been closed.
	closedChan chan struct{}

	// Guards Close so that it is safe to invoke multiple times.
	closeOnce sync.Once
}

// NewWatcher creates a Watcher with the supplied identifier. The id must be
// greater than zero; use NewAutoIDWatcher to obtain a randomly generated id.
// Optional filters restrict the events the Watcher receives and are combined
// using a logical AND.
func NewWatcher[E proto.Message](id int64, filters ...FilterFunc[E]) (*Watcher[E], error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidWatcherID, id)
	}
	return newWatcher[E](id, filters...), nil
}

// NewAutoIDWatcher creates a Watcher with a randomly generated identifier.
// Optional filters restrict the events the Watcher receives and are combined
// using a logical AND.
func NewAutoIDWatcher[E proto.Message](filters ...FilterFunc[E]) (*Watcher[E], error) {
	// rand.Int returns a value in [0, max); shift by 1 so the id is strictly
	// positive and never collides with the sentinel rejected by NewWatcher.
	i, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
	if err != nil {
		return nil, fmt.Errorf("unable to auto-generate watcher id: %w", err)
	}
	return newWatcher[E](i.Int64()+1, filters...), nil
}

func newWatcher[E proto.Message](id int64, filters ...FilterFunc[E]) *Watcher[E] {
	return &Watcher[E]{
		id:           id,
		filter:       and(filters...),
		messagesChan: make(chan *Message[E], defaultBufferCapacity),
		closedChan:   make(chan struct{}),
	}
}

// GetID returns the Watcher's unique identifier.
func (w *Watcher[E]) GetID() int64 {
	return w.id
}

// Closed returns a read-only channel that is closed when the work managed by
// this Watcher is complete. The Watcher transitions to a done state when
// Close() is called.
func (w *Watcher[E]) Closed() <-chan struct{} {
	return w.closedChan
}

// Receive is invoked by the WatchableStream when a new event arrives for this
// watcher. It returns:
//
//   - ErrWatcherClosed if the Watcher has been closed.
//   - ctx.Err() if the caller's context is cancelled while waiting for the
//     buffer to make room.
//
// Events that do not pass the configured filter are discarded immediately and
// the returned acknowledge channel is already closed.
func (w *Watcher[E]) Receive(ctx context.Context, event E) (<-chan struct{}, error) {
	logger := ctxzap.Extract(ctx)
	ackChan := make(chan struct{})

	// Closed-watcher check runs before the filter so the contract is uniform:
	// a closed Watcher always returns ErrWatcherClosed regardless of filter
	// outcome.
	select {
	case <-w.closedChan:
		close(ackChan)
		return ackChan, ErrWatcherClosed
	default:
	}

	if !w.applyFilter(logger, event) {
		if logger.Level().Enabled(zapcore.DebugLevel) {
			logger.Debug("Discarding event due to configured filter", zap.Any("event", event))
		}
		close(ackChan)
		return ackChan, nil
	}

	msg := &Message[E]{
		acknowledgeChan: ackChan,
		closeOnce:       &sync.Once{},
		Event:           event,
	}
	select {
	case <-ctx.Done():
		close(ackChan)
		return ackChan, ctx.Err()

	case <-w.closedChan:
		close(ackChan)
		return ackChan, ErrWatcherClosed

	case w.messagesChan <- msg:
		if logger.Level().Enabled(zapcore.DebugLevel) {
			logger.Debug("Received event", zap.Any("event", event))
		}
		return ackChan, nil
	}
}

func (w *Watcher[E]) applyFilter(logger *zap.Logger, event E) (matched bool) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Filter panicked; treating event as filtered out",
				zap.Any("event", event),
				zap.Any("panic", r))
			matched = false
		}
	}()
	return w.filter(event)
}

// MessagesChan returns a read-only channel for receiving messages from this watcher.
func (w *Watcher[E]) MessagesChan() <-chan *Message[E] {
	return w.messagesChan
}

// Close marks this Watcher as closed. It is safe to call from multiple
// goroutines and may be called multiple times; subsequent calls are no-ops.
// Subsequent calls to Receive will return ErrWatcherClosed. Consumers reading
// from MessagesChan should also select on Closed to detect shutdown.
func (w *Watcher[E]) Close() {
	w.closeOnce.Do(func() {
		close(w.closedChan)
	})
}
