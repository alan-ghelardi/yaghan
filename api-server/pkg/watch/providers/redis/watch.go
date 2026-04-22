package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.nuinfra.api-server/pkg/config"
	"golang.nuinfra.api-server/pkg/watch"
	"golang.nuinfra.net/commons/pkg/utilities"
	"google.golang.org/protobuf/proto"
)

const (
	// Template for the keys where we store the last event identifier a
	// given client has processed.
	lastEventIDFmt = "nuinfra/%d/last-event-id"

	eventKey = "event"

	// Symbol that references the last event in the stream.
	maxSeenID = "$"

	// xreadBlockInterval bounds how long XRead will block before returning
	// to the read loop's outer select. go-redis does not honour ctx
	// cancellation for blocking commands (the connection's read deadline is
	// pinned at command issue time), so a finite block is the only way to
	// guarantee the read loop revisits watcher.Closed() within a bounded
	// delay when a watcher with no in-flight events is shut down.
	xreadBlockInterval = 1 * time.Second
)

// SetEventIDFunc is a function to set the event unique identifier.
type SetEventIDFunc[E proto.Message] func(event E, id string)

// redisEventStream is a WatchableStream implementation built on top of Redis streams.
type redisEventStream[E proto.Message] struct {

	// Client to talk to Redis.
	client *redis.Client

	// Event stream's configurations.
	config *config.WatchStream

	// Redis stream we're going to read entries from.
	stream string

	// Registered watchers, keyed by watcher id.
	watchers map[int64]*watcherEntry[E]

	// setEventID is a func to set the event's unique identifier.
	setEventID SetEventIDFunc[E]

	mut sync.Mutex
}

// watcherEntry pairs a Watcher with the channel that signals its readLoop has
// fully exited; StopWatching waits on this channel so callers do not race with
// background logging.
type watcherEntry[E proto.Message] struct {
	watcher *watch.Watcher[E]
	done    chan struct{}
}

// NewFromConfig returns a new WatchableStream backed by a Redis server.
func NewFromConfig[E proto.Message](ctx context.Context, config *config.WatchStream, getEventIDFunc SetEventIDFunc[E]) (watch.WatchableStream[E], error) {
	opts := &redis.Options{
		Addr:                  config.Redis.Address,
		ContextTimeoutEnabled: true,
		ClientName:            "nuinfra",
		DB:                    int(config.Redis.Database),
	}

	if config.Redis.AuthToken != nil {
		token, err := config.Redis.AuthToken.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to load Redis auth token from %s: %w", config.Redis.AuthToken, err)
		}
		opts.Password = token
	}

	if config.Redis.TLS != nil {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("unable to load system certificate pool for Redis TLS: %w", err)
		}
		opts.TLSConfig = &tls.Config{
			RootCAs:            certPool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: config.Redis.TLS.InsecureSkipVerify, //nolint:gosec // Configurable for testing environments.
		}
	}

	eventStream := &redisEventStream[E]{
		client:     redis.NewClient(opts),
		config:     config,
		stream:     fmt.Sprintf("%s:%s", config.Redis.StreamName, config.Redis.StreamKey),
		watchers:   make(map[int64]*watcherEntry[E]),
		setEventID: getEventIDFunc,
	}

	if err := eventStream.client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return eventStream, nil
}

// GetWatchersCount implements watch.WatchableStream
func (r *redisEventStream[E]) GetWatchersCount() int {
	r.mut.Lock()
	defer r.mut.Unlock()
	return len(r.watchers)
}

// Publish implements watch.WatchableStream
func (r *redisEventStream[E]) Publish(ctx context.Context, event E) error {
	wire, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("unable to marshal event: %w", err)
	}

	data := base64.RawURLEncoding.EncodeToString(wire)

	_, err = r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.stream,
		Values: map[string]any{
			eventKey: data,
		},
		Approx: true,
		MaxLen: int64(r.config.Redis.MaxStreamLength),
	}).
		Result()
	if err != nil {
		return fmt.Errorf("unable to publish event to Redis stream %s: %w", r.stream, err)
	}

	return nil
}

// Watch implements watch.WatchableStream
func (r *redisEventStream[E]) Watch(ctx context.Context, watcher *watch.Watcher[E]) error {
	r.mut.Lock()
	existing, ok := r.watchers[watcher.GetID()]
	if ok {
		delete(r.watchers, watcher.GetID())
	}
	r.mut.Unlock()

	if ok {
		existing.watcher.Close()
		<-existing.done
	}

	readyChan := make(chan error, 1)
	done := make(chan struct{})
	go r.readLoop(ctx, watcher, readyChan, done)

	if err := <-readyChan; err != nil {
		<-done
		return fmt.Errorf("unable to check stream readiness: %w", err)
	}

	r.mut.Lock()
	r.watchers[watcher.GetID()] = &watcherEntry[E]{watcher: watcher, done: done}
	r.mut.Unlock()
	return nil
}

func (r *redisEventStream[E]) readLoop(parent context.Context, watcher *watch.Watcher[E], readyChan chan<- error, done chan struct{}) {
	defer close(done)

	// Bridge watcher closure to ctx cancellation so blocked redis I/O
	// returns immediately when the watcher is closed.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
		case <-watcher.Closed():
			cancel()
		}
	}()

	logger := ctxzap.Extract(ctx).Sugar().With(zap.Int64("watcher.id", watcher.GetID()))

	if err := r.checkReadiness(ctx); err != nil {
		readyChan <- err
		return
	}

	// Resolve the starting position BEFORE signalling readiness. For a fresh
	// watcher, getLastEventID falls back to "$" (the latest entry at XRead
	// time), which races with publishers that send events the instant Watch
	// returns. Pin to the current stream tail so events published after this
	// point are guaranteed to be picked up.
	startAfter := r.getLastEventID(ctx, watcher)
	if startAfter == maxSeenID {
		startAfter = r.streamTail(ctx)
	}

	close(readyChan)

	logger.Debug("Starting read loop")

	expBackoff := backoff.NewExponentialBackOff()

	var err error
	for {
		select {
		case <-ctx.Done():
			logger.Debugf("Closing watcher due to context cancellation: %v", err)
			return

		case <-watcher.Closed():
			logger.Debug("Closing watcher due to client request")
			return

		default:
			startAfter, err = r.readStream(ctx, watcher, startAfter)
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, watch.ErrWatcherClosed) {
					return
				}
				// Eviction closes the watcher synchronously but cancels the
				// bridged ctx via a goroutine, so check the watcher directly
				// to avoid a benign log race between Close() and cancel().
				select {
				case <-watcher.Closed():
					return
				default:
				}
				logger.Errorw("Error reading stream", zap.Error(err))

				delay := expBackoff.NextBackOff()
				select {
				case <-ctx.Done():
					return
				case <-watcher.Closed():
					return
				case <-time.After(delay):
				}
			} else {
				expBackoff.Reset()
			}
		}
	}
}

func (r *redisEventStream[E]) checkReadiness(ctx context.Context) error {
	logger := ctxzap.Extract(ctx)

	timeoutChan := time.After(r.config.Redis.StreamReadinessTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeoutChan:
			return errors.New("timed out awaiting for a successful connection with the event stream")

		default:
			err := r.client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{r.stream, "0"},
				Count:   1,
				Block:   -1,
			}).Err()
			if err == nil || errors.Is(err, redis.Nil) {
				return nil
			}
			logger.Error("Error verifying stream readiness", zap.Error(err))
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// streamTail returns the id of the most recent entry in the stream, or
// maxSeenID if the stream is empty or unreachable. Used to pin a fresh
// watcher's starting position so it does not race with publishers between
// signalling readiness and the first XRead call.
func (r *redisEventStream[E]) streamTail(parent context.Context) string {
	ctx, cancel := context.WithTimeout(parent, r.config.Redis.Timeout)
	defer cancel()

	entries, err := r.client.XRevRangeN(ctx, r.stream, "+", "-", 1).Result()
	if err != nil || len(entries) == 0 {
		return maxSeenID
	}
	return entries[0].ID
}

// getLastEventID returns the identifier of the last event processed by the
// watcher. If this watcher is connecting for the first time, returns the
// identifier of the last (most recent) event in the stream.
func (r *redisEventStream[E]) getLastEventID(parent context.Context, watcher *watch.Watcher[E]) string {
	logger := ctxzap.Extract(parent)

	ctx, cancel := context.WithTimeout(parent, r.config.Redis.Timeout)
	defer cancel()

	key := fmt.Sprintf(lastEventIDFmt, watcher.GetID())
	eventID, err := r.client.Get(ctx, key).Result()
	if err != nil {
		eventID = maxSeenID
		if !errors.Is(err, redis.Nil) {
			logger.Error("Unable to read Redis key", zap.String("redis.key", key), zap.Error(err))
		}
	}

	return eventID
}

func (r *redisEventStream[E]) readStream(ctx context.Context, watcher *watch.Watcher[E], startingAfter string) (string, error) {
	logger := ctxzap.Extract(ctx)

	streams, err := r.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{r.stream, startingAfter},
		Block:   xreadBlockInterval,
	}).Result()
	if err != nil {
		// redis.Nil signals an empty poll (no new entries within the
		// block window). Treat it as success with no progress so the
		// read loop iterates and re-checks watcher.Closed().
		if errors.Is(err, redis.Nil) {
			return startingAfter, nil
		}
		return maxSeenID, fmt.Errorf("unable to read data from stream %s: %w", r.stream, err)
	}

	messages := streams[0].Messages
	if len(messages) == 0 {
		return startingAfter, nil
	}

	lastEventID := startingAfter

	for _, message := range messages {
		messageID := message.ID
		logger := logger.With(zap.String("message.id", messageID))

		value, found := message.Values[eventKey]
		if !found {
			logger.Warn("Malformed message: event is missing")
			continue
		}

		data, ok := value.(string)
		if !ok {
			logger.Error("Malformed message: event cannot be converted to string")
			continue
		}

		wire, err := base64.RawURLEncoding.DecodeString(data)
		if err != nil {
			logger.Error("Malformed message: unable to decode event", zap.Error(err))
			continue
		}

		event := utilities.NewObject[E]()
		if err := proto.Unmarshal(wire, event); err != nil {
			logger.Error("Malformed event", zap.Error(err))
			continue
		}

		if err := r.deliver(ctx, watcher, event, messageID); err != nil {
			return lastEventID, err
		}

		lastEventID = messageID
	}

	return lastEventID, nil
}

func (r *redisEventStream[E]) deliver(parent context.Context, watcher *watch.Watcher[E], event E, eventID string) error {
	r.setEventID(event, eventID)

	logger := ctxzap.Extract(parent)

	ctx, cancel := context.WithTimeout(parent, r.config.EventDeliveryTimeout)
	defer cancel()

	ack, err := watcher.Receive(ctx, event)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil {
			r.evictWatcher(parent, watcher, "buffer was full beyond the delivery deadline")
		}
		return err
	}

	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
			r.evictWatcher(parent, watcher, "consumer did not acknowledge within the delivery deadline")
		}
		return ctx.Err()

	case <-ack:
		logger.Info("Event has been delivered successfully", zap.String("event.id", eventID))

		// Decouple from parent's cancellation: closing the watcher must not
		// race with persisting the last delivered event id, otherwise a
		// reconnecting watcher with the same id would skip pending events.
		if err := r.setLastEventID(context.WithoutCancel(parent), watcher, eventID); err != nil {
			logger.Error("Unable to record event id for future recovery", zap.String("event.id", eventID), zap.Error(err))
		}

		return nil
	}
}

// evictWatcher is invoked by the readLoop goroutine itself when a delivery
// deadline is exceeded. It removes the watcher from the registry and closes
// it, but does not wait for the readLoop to exit (it would deadlock — the
// caller IS the readLoop).
func (r *redisEventStream[E]) evictWatcher(ctx context.Context, watcher *watch.Watcher[E], reason string) {
	ctxzap.Extract(ctx).Warn("Evicting watcher",
		zap.Int64("watcher.id", watcher.GetID()),
		zap.String("reason", reason))

	r.mut.Lock()
	delete(r.watchers, watcher.GetID())
	r.mut.Unlock()

	watcher.Close()
}

func (r *redisEventStream[E]) setLastEventID(parent context.Context, watcher *watch.Watcher[E], eventID string) error {
	ctx, cancel := context.WithTimeout(parent, r.config.Redis.Timeout)
	defer cancel()

	key := fmt.Sprintf(lastEventIDFmt, watcher.GetID())

	if err := r.client.Set(ctx, key, eventID, r.config.Redis.KeyTTL).Err(); err != nil {
		return fmt.Errorf("unable to set key %s: %w", key, err)
	}

	return nil
}

// StopWatching implements watch.WatchableStream. It closes the watcher and
// waits for its readLoop goroutine to exit, so callers can be sure no further
// background work is in flight when this returns.
func (r *redisEventStream[E]) StopWatching(_ context.Context, watcherID int64) error {
	r.mut.Lock()
	entry, ok := r.watchers[watcherID]
	if ok {
		delete(r.watchers, watcherID)
	}
	r.mut.Unlock()

	if !ok {
		return fmt.Errorf("no such watcher #%d", watcherID)
	}

	entry.watcher.Close()
	<-entry.done
	return nil
}
