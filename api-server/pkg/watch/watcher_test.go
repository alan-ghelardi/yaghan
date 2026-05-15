package watch

import (
	"context"
	"fmt"
	"testing"
	"time"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"

	"github.com/google/go-cmp/cmp"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestNewWatcher(t *testing.T) {
	t.Run("rejects identifiers less than or equal to zero", func(t *testing.T) {
		for _, id := range []int64{0, -1, -42} {
			t.Run(fmt.Sprintf("id=%d", id), func(t *testing.T) {
				watcher, err := NewWatcher[*controlplanev1alpha1.Event](id)
				assert.Nil(t, watcher)
				assert.ErrorIs(t, err, ErrInvalidWatcherID)
			})
		}
	})

	t.Run("uses the supplied identifier", func(t *testing.T) {
		watcher, err := NewWatcher[*controlplanev1alpha1.Event](42)
		require.NoError(t, err)
		assert.Equal(t, int64(42), watcher.GetID())
	})
}

func TestNewAutoIDWatcher(t *testing.T) {
	t.Run("auto-generates a positive identifier", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)
		assert.Positive(t, watcher.GetID())
	})

	t.Run("returns distinct identifiers across instances", func(t *testing.T) {
		seen := make(map[int64]struct{}, 32)
		for range 32 {
			watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
			require.NoError(t, err)
			_, exists := seen[watcher.GetID()]
			require.False(t, exists, "auto-generated id %d collided", watcher.GetID())
			seen[watcher.GetID()] = struct{}{}
		}
	})
}

func TestWatcher(t *testing.T) {
	ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t))

	t.Run("receive events", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		expectedEvents := make([]*controlplanev1alpha1.Event, 0, 3)
		receivedEvents := make([]*controlplanev1alpha1.Event, 0, cap(expectedEvents))

		go func() {
			for {
				select {
				case <-watcher.Closed():
					return
				case message := <-watcher.MessagesChan():
					receivedEvents = append(receivedEvents, message.Event)
					message.Acknowledge()
				}
			}
		}()

		for i := range cap(expectedEvents) {
			event := &controlplanev1alpha1.Event{Id: fmt.Sprintf("e-%d", i+1)}
			expectedEvents = append(expectedEvents, event)

			ack, err := watcher.Receive(ctx, event)
			require.NoError(t, err)

			<-ack
		}

		diff := cmp.Diff(expectedEvents, receivedEvents, protocmp.Transform())
		assert.Empty(t, diff, "Mismatch (-want +got):\n%s", diff)
	})

	t.Run("close the watcher and no longer receive events", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)

		watcher.Close()

		select {
		case <-watcher.Closed():
		default:
			require.Fail(t, "Watcher must have been closed")
		}

		_, err = watcher.Receive(ctx, &controlplanev1alpha1.Event{})
		assert.ErrorIs(t, err, ErrWatcherClosed)
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)

		require.NotPanics(t, func() {
			watcher.Close()
			watcher.Close()
			watcher.Close()
		})
	})

	t.Run("a closed watcher rejects events even when the filter would discard them", func(t *testing.T) {
		filter := func(*controlplanev1alpha1.Event) bool { return false }

		watcher, err := NewAutoIDWatcher(filter)
		require.NoError(t, err)
		watcher.Close()

		_, err = watcher.Receive(ctx, &controlplanev1alpha1.Event{})
		assert.ErrorIs(t, err, ErrWatcherClosed)
	})

	t.Run("filter events", func(t *testing.T) {
		filter := func(event *controlplanev1alpha1.Event) bool {
			return event.GetSandbox().Status.Phase == controlplanev1alpha1.SandboxStatus_PHASE_RUNNING
		}

		watcher, err := NewAutoIDWatcher(filter)
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		var receivedEvent *controlplanev1alpha1.Event
		go func() {
			message := <-watcher.MessagesChan()
			receivedEvent = message.Event
			message.Acknowledge()
		}()

		ack, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{
			InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
				Sandbox: &controlplanev1alpha1.Sandbox{
					Status: &controlplanev1alpha1.SandboxStatus{
						Phase: controlplanev1alpha1.SandboxStatus_PHASE_DELETED,
					},
				},
			},
		})
		require.NoError(t, err)

		_, isOpen := <-ack
		assert.False(t, isOpen, "Acknowledge channel must be closed for filtered-out events")

		ack, err = watcher.Receive(ctx, &controlplanev1alpha1.Event{
			InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
				Sandbox: &controlplanev1alpha1.Sandbox{
					Status: &controlplanev1alpha1.SandboxStatus{
						Phase: controlplanev1alpha1.SandboxStatus_PHASE_RUNNING,
					},
				},
			},
		})
		require.NoError(t, err)

		<-ack

		require.NotNil(t, receivedEvent)
		assert.Equal(t, controlplanev1alpha1.SandboxStatus_PHASE_RUNNING, receivedEvent.GetSandbox().Status.Phase)
	})

	t.Run("combine multiple filters with logical AND", func(t *testing.T) {
		hasRunningPhase := func(event *controlplanev1alpha1.Event) bool {
			return event.GetSandbox().Status.Phase == controlplanev1alpha1.SandboxStatus_PHASE_RUNNING
		}
		hasMatchingID := func(event *controlplanev1alpha1.Event) bool {
			return event.GetSandbox().Metadata.Id == "match"
		}

		watcher, err := NewAutoIDWatcher(hasRunningPhase, hasMatchingID)
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		var receivedEvent *controlplanev1alpha1.Event
		go func() {
			message := <-watcher.MessagesChan()
			receivedEvent = message.Event
			message.Acknowledge()
		}()

		events := []*controlplanev1alpha1.Event{
			{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{Id: "miss"},
						Status:   &controlplanev1alpha1.SandboxStatus{Phase: controlplanev1alpha1.SandboxStatus_PHASE_RUNNING},
					},
				},
			},
			{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{Id: "match"},
						Status:   &controlplanev1alpha1.SandboxStatus{Phase: controlplanev1alpha1.SandboxStatus_PHASE_DELETED},
					},
				},
			},
			{
				InvolvedObject: &controlplanev1alpha1.Event_Sandbox{
					Sandbox: &controlplanev1alpha1.Sandbox{
						Metadata: &controlplanev1alpha1.SandboxMeta{Id: "match"},
						Status:   &controlplanev1alpha1.SandboxStatus{Phase: controlplanev1alpha1.SandboxStatus_PHASE_RUNNING},
					},
				},
			},
		}
		for _, event := range events {
			ack, err := watcher.Receive(ctx, event)
			require.NoError(t, err)
			<-ack
		}

		require.NotNil(t, receivedEvent)
		assert.Equal(t, "match", receivedEvent.GetSandbox().Metadata.Id)
		assert.Equal(t, controlplanev1alpha1.SandboxStatus_PHASE_RUNNING, receivedEvent.GetSandbox().Status.Phase)
	})

	t.Run("a panicking filter is treated as a non-match", func(t *testing.T) {
		boom := func(*controlplanev1alpha1.Event) bool {
			panic("boom")
		}

		watcher, err := NewAutoIDWatcher(boom)
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		ack, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{})
		require.NoError(t, err)

		_, isOpen := <-ack
		assert.False(t, isOpen, "Acknowledge channel must be closed for filtered-out events")

		select {
		case message, ok := <-watcher.MessagesChan():
			require.Fail(t, "MessagesChan must not deliver an event when the filter panicked",
				"received: %+v open=%v", message, ok)
		default:
		}
	})

	t.Run("Acknowledge is a no-op when no acknowledgement channel is set", func(t *testing.T) {
		message := &Message[*controlplanev1alpha1.Event]{}
		require.NotPanics(t, message.Acknowledge)
	})

	t.Run("Acknowledge is idempotent", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		ack, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{})
		require.NoError(t, err)

		message := <-watcher.MessagesChan()
		require.NotPanics(t, func() {
			message.Acknowledge()
			message.Acknowledge()
			message.Acknowledge()
		})

		_, isOpen := <-ack
		assert.False(t, isOpen, "Acknowledge channel must be closed after the first call")
	})

	t.Run("close while a Receive call is blocked on a full buffer", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)

		for range defaultBufferCapacity {
			_, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{})
			require.NoError(t, err)
		}

		receiveErr := make(chan error, 1)
		go func() {
			_, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{})
			receiveErr <- err
		}()

		// Give the goroutine time to enter the slow-path select before closing.
		time.Sleep(50 * time.Millisecond)

		watcher.Close()

		select {
		case err := <-receiveErr:
			assert.ErrorIs(t, err, ErrWatcherClosed)
		case <-time.After(time.Second):
			require.Fail(t, "Receive did not return after Close was called")
		}
	})

	t.Run("Receive returns ctx.Err() when the buffer is full and the context is cancelled", func(t *testing.T) {
		watcher, err := NewAutoIDWatcher[*controlplanev1alpha1.Event]()
		require.NoError(t, err)
		t.Cleanup(watcher.Close)

		for range defaultBufferCapacity {
			_, err := watcher.Receive(ctx, &controlplanev1alpha1.Event{})
			require.NoError(t, err)
		}

		cancellableCtx, cancel := context.WithCancel(ctx)

		receiveErr := make(chan error, 1)
		go func() {
			_, err := watcher.Receive(cancellableCtx, &controlplanev1alpha1.Event{})
			receiveErr <- err
		}()

		// Give the goroutine time to enter the slow-path select before cancelling.
		time.Sleep(50 * time.Millisecond)

		cancel()

		select {
		case err := <-receiveErr:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(time.Second):
			require.Fail(t, "Receive did not return after the context was cancelled")
		}
	})
}
