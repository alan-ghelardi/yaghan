package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alan-ghelardi/yaghan/api-server/pkg/config"
	"github.com/alan-ghelardi/yaghan/api-server/pkg/watch"
	redistesting "github.com/alan-ghelardi/yaghan/api-server/pkg/watch/providers/redis/testing"
	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/alan-ghelardi/yaghan/commons/pkg/utilities"
	"github.com/google/go-cmp/cmp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/testing/protocmp"
)

const testTimeout = 5 * time.Second

func TestRedisEventStream(t *testing.T) {
	redisEndpoint := redistesting.WithRedis(t)

	ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t, zaptest.Level(zapcore.ErrorLevel)))
	config := &config.WatchStream{
		EventDeliveryTimeout: 5 * time.Second,
		Redis: &config.Redis{
			Address:                redisEndpoint,
			StreamName:             "test",
			StreamKey:              "test",
			KeyTTL:                 5 * time.Minute,
			Timeout:                15 * time.Second,
			StreamReadinessTimeout: 5 * time.Second,
		},
	}
	setEventID := func(event *controlplanev1alpha1.Event, eventID string) {
		event.Id = eventID
	}

	t.Run("broadcast events to various watchers", func(t *testing.T) {
		watchableStream, err := NewFromConfig(ctx, config, setEventID)
		if err != nil {
			t.Fatal(err)
		}

		watchers := make([]*watch.Watcher[*controlplanev1alpha1.Event], 3)
		for i := range watchers {
			watchers[i] = mustCreateWatcher(t, 0)
		}

		events := make([]*controlplanev1alpha1.Event, 3)

		eventsChan, errChan := receiveEventsFromNWatchers(ctx, t, watchableStream, len(events), watchers...)

		for i := range len(events) {
			event := &controlplanev1alpha1.Event{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{
							Id: utilities.RandomString(7),
						},
						Status: &controlplanev1alpha1.SandboxStatus{
							Phase: controlplanev1alpha1.SandboxStatus_PHASE_RUNNING,
						},
					},
				},
			}

			if err := watchableStream.Publish(ctx, event); err != nil {
				t.Fatal(err)
			}

			events[i] = event
		}

		checkedWatchers := 0
		for {
			select {

			case receivedEvents := <-eventsChan:
				assertEventsAreEqual(t, events, receivedEvents)
				checkedWatchers++

			case err := <-errChan:
				if err != nil {
					t.Fatal(err)
				}

			default:
				if len(watchers) == checkedWatchers {
					return
				}
			}
		}
	})

	t.Run("deliver pending events to a watcher that reconnects after a failure", func(t *testing.T) {
		watchableStream, err := NewFromConfig(ctx, config, setEventID)
		if err != nil {
			t.Fatal(err)
		}

		watcher := mustCreateWatcher(t, 0)

		eventsChan, errChan := receiveEventsFromNWatchers(ctx, t, watchableStream, 1, watcher)

		events := make([]*controlplanev1alpha1.Event, 0, 3)
		receivedEvents := make([]*controlplanev1alpha1.Event, 0, 3)

		for range 3 {
			events = append(events, &controlplanev1alpha1.Event{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{
							Id: utilities.RandomString(7),
						},
					},
				},
			})
		}

		if err := watchableStream.Publish(ctx, events[0]); err != nil {
			t.Fatal(err)
		}

		receivedEvents = append(receivedEvents, mustReceiveEvents(ctx, t, eventsChan, errChan)...)

		<-watcher.Closed()

		// At this point the watcher has been shut down. Publish two new
		// events and register again using the same auto-generated
		// id. The watcher must receive the two new events.

		for _, event := range events[1:] {
			if err := watchableStream.Publish(ctx, event); err != nil {
				t.Fatal(err)
			}
		}

		watcher = mustCreateWatcher(t, watcher.GetID())
		newEvents, err := receiveEvents(ctx, watchableStream, watcher, 2)
		if err != nil {
			t.Fatal(err)
		}

		receivedEvents = append(receivedEvents, newEvents...)

		assertEventsAreEqual(t, events, receivedEvents)
	})

	t.Run("filter received events", func(t *testing.T) {
		watchableStream, err := NewFromConfig(ctx, config, setEventID)
		if err != nil {
			t.Fatal(err)
		}

		filterByRunningSandboxes := func(event *controlplanev1alpha1.Event) bool {
			return event.GetSandbox().Status.Phase == controlplanev1alpha1.SandboxStatus_PHASE_RUNNING
		}

		watcher := mustCreateWatcher(t, 0, filterByRunningSandboxes)

		eventsChan, errChan := receiveEventsFromNWatchers(ctx, t, watchableStream, 1, watcher)

		phases := []controlplanev1alpha1.SandboxStatus_Phase{
			controlplanev1alpha1.SandboxStatus_PHASE_DELETED,
			controlplanev1alpha1.SandboxStatus_PHASE_PENDING,
			controlplanev1alpha1.SandboxStatus_PHASE_RUNNING,
			controlplanev1alpha1.SandboxStatus_PHASE_RESUMING,
		}
		for _, phase := range phases {
			event := &controlplanev1alpha1.Event{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{
							Id: utilities.RandomString(7),
						},
						Status: &controlplanev1alpha1.SandboxStatus{
							Phase: phase,
						},
					},
				},
			}

			if err := watchableStream.Publish(ctx, event); err != nil {
				t.Fatal(err)
			}
		}

		receivedEvents := mustReceiveEvents(ctx, t, eventsChan, errChan)

		if phase := receivedEvents[0].GetSandbox().Status.Phase; phase != controlplanev1alpha1.SandboxStatus_PHASE_RUNNING {
			t.Errorf("Received an unexpected sandbox with phase %v", phase)
		}
	})
}

func mustCreateWatcher(t *testing.T, id int64, filters ...watch.FilterFunc[*controlplanev1alpha1.Event]) *watch.Watcher[*controlplanev1alpha1.Event] {
	t.Helper()

	var (
		watcher *watch.Watcher[*controlplanev1alpha1.Event]
		err     error
	)
	if id == 0 {
		watcher, err = watch.NewAutoIDWatcher(filters...)
	} else {
		watcher, err = watch.NewWatcher(id, filters...)
	}
	if err != nil {
		t.Fatal(err)
	}
	return watcher
}

func receiveEventsFromNWatchers(ctx context.Context, t *testing.T, stream watch.WatchableStream[*controlplanev1alpha1.Event], expectedEventsCount int, watchers ...*watch.Watcher[*controlplanev1alpha1.Event]) (eventsChan chan []*controlplanev1alpha1.Event, errChan chan error) {
	t.Helper()

	eventsChan = make(chan []*controlplanev1alpha1.Event)
	errChan = make(chan error)

	for i := range watchers {
		go func() {
			events, err := receiveEvents(ctx, stream, watchers[i], expectedEventsCount)
			if err != nil {
				errChan <- err
			} else {
				eventsChan <- events
			}
		}()
	}

	timeoutChan := time.After(testTimeout)
	for {
		select {
		case <-timeoutChan:
			t.Fatalf("Timed out awaiting for watchers to register: %d watchers are available", stream.GetWatchersCount())

		default:
			if len(watchers) == stream.GetWatchersCount() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func receiveEvents(ctx context.Context, stream watch.WatchableStream[*controlplanev1alpha1.Event], watcher *watch.Watcher[*controlplanev1alpha1.Event], expectedEventsCount int) ([]*controlplanev1alpha1.Event, error) {
	err := stream.Watch(ctx, watcher)
	if err != nil {
		return nil, err
	}

	events := []*controlplanev1alpha1.Event{}
	errChan := make(chan error)

	go func() {
		defer close(errChan)

		timeoutChan := time.After(testTimeout)
		for {
			select {
			case message := <-watcher.MessagesChan():
				events = append(events, message.Event)
				message.Acknowledge()
				if len(events) == expectedEventsCount {
					if err := stream.StopWatching(ctx, watcher.GetID()); err != nil {
						errChan <- err
					}
					return
				}

			case <-timeoutChan:
				errChan <- errors.New("timed out awaiting for message")
				return
			}
		}
	}()

	return events, <-errChan
}

func assertEventsAreEqual(t *testing.T, want, got []*controlplanev1alpha1.Event) {
	t.Helper()
	if diff := cmp.Diff(want, got,
		protocmp.Transform(),
		protocmp.IgnoreFields(&controlplanev1alpha1.Event{}, "id")); diff != "" {
		t.Errorf("Mismatch (-want +got):\n%s", diff)
	}
}

func mustReceiveEvents(ctx context.Context, t *testing.T, eventsChan chan []*controlplanev1alpha1.Event, errChan chan error) []*controlplanev1alpha1.Event {
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())

	case <-time.After(testTimeout):
		t.Fatal("Timed out awaiting for event")

	case err := <-errChan:
		t.Fatal(err)

	case events := <-eventsChan:
		return events
	}
	return nil
}
