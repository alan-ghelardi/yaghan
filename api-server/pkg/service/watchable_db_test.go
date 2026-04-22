package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	sandboxmocks "golang.nuinfra.api-server/pkg/db/sandbox/mocks"
	watchmocks "golang.nuinfra.api-server/pkg/watch/mocks"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestWatchableDB(t *testing.T) {
	t.Run("Create publishes an event wrapping the sandbox", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t))

		db := sandboxmocks.NewMockDB(ctrl)
		stream := watchmocks.NewMockWatchableStream[*controlplanev1alpha1.Event](ctrl)
		w := &WatchableDB{db: db, watchableStream: stream}

		sandbox := &controlplanev1alpha1.Sandbox{
			Metadata: &controlplanev1alpha1.SandboxMeta{Id: "sb-1"},
		}

		var published *controlplanev1alpha1.Event
		db.EXPECT().Create(ctx, sandbox).Return(nil)
		stream.EXPECT().Publish(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, e *controlplanev1alpha1.Event) error {
				published = e
				return nil
			})

		require.NoError(t, w.Create(ctx, sandbox))

		require.NotNil(t, published)
		assert.True(t, proto.Equal(sandbox, published.GetSandbox()),
			"published event should wrap the input sandbox")
		assert.NotNil(t, published.EmittedAt, "EmittedAt must be set")
	})

	t.Run("Update publishes an event wrapping the sandbox", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t))

		db := sandboxmocks.NewMockDB(ctrl)
		stream := watchmocks.NewMockWatchableStream[*controlplanev1alpha1.Event](ctrl)
		w := &WatchableDB{db: db, watchableStream: stream}

		sandbox := &controlplanev1alpha1.Sandbox{
			Metadata: &controlplanev1alpha1.SandboxMeta{Id: "sb-2"},
		}

		var published *controlplanev1alpha1.Event
		db.EXPECT().Update(ctx, sandbox).Return(nil)
		stream.EXPECT().Publish(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, e *controlplanev1alpha1.Event) error {
				published = e
				return nil
			})

		require.NoError(t, w.Update(ctx, sandbox))

		require.NotNil(t, published)
		assert.True(t, proto.Equal(sandbox, published.GetSandbox()))
		assert.NotNil(t, published.EmittedAt)
	})

	t.Run("DB error short-circuits the publish", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t))

		db := sandboxmocks.NewMockDB(ctrl)
		stream := watchmocks.NewMockWatchableStream[*controlplanev1alpha1.Event](ctrl)
		w := &WatchableDB{db: db, watchableStream: stream}

		sandbox := &controlplanev1alpha1.Sandbox{
			Metadata: &controlplanev1alpha1.SandboxMeta{Id: "sb-3"},
		}

		dbErr := errors.New("db is on fire")
		db.EXPECT().Create(ctx, sandbox).Return(dbErr)
		// stream.Publish must not be called — gomock will fail the test if it is.

		assert.ErrorIs(t, w.Create(ctx, sandbox), dbErr)
	})

	t.Run("Publish retries on transient errors and eventually succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ctx := ctxzap.ToContext(context.Background(), zaptest.NewLogger(t))

		db := sandboxmocks.NewMockDB(ctrl)
		stream := watchmocks.NewMockWatchableStream[*controlplanev1alpha1.Event](ctrl)
		w := &WatchableDB{
			db:                    db,
			watchableStream:       stream,
			publishMaxElapsedTime: time.Second,
			publishBackOff: func() backoff.BackOff {
				return backoff.NewConstantBackOff(time.Millisecond)
			},
		}

		sandbox := &controlplanev1alpha1.Sandbox{
			Metadata: &controlplanev1alpha1.SandboxMeta{Id: "sb-4"},
		}

		transient := errors.New("redis unavailable")
		db.EXPECT().Create(ctx, sandbox).Return(nil)
		gomock.InOrder(
			stream.EXPECT().Publish(ctx, gomock.Any()).Return(transient),
			stream.EXPECT().Publish(ctx, gomock.Any()).Return(transient),
			stream.EXPECT().Publish(ctx, gomock.Any()).Return(nil),
		)

		require.NoError(t, w.Create(ctx, sandbox))
	})

	t.Run("Exhausted backoff is logged but not propagated", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		core, observed := observer.New(zapcore.ErrorLevel)
		ctx := ctxzap.ToContext(context.Background(), zap.New(core))

		db := sandboxmocks.NewMockDB(ctrl)
		stream := watchmocks.NewMockWatchableStream[*controlplanev1alpha1.Event](ctrl)
		w := &WatchableDB{
			db:                    db,
			watchableStream:       stream,
			publishMaxElapsedTime: 50 * time.Millisecond,
			publishBackOff: func() backoff.BackOff {
				return backoff.NewConstantBackOff(time.Millisecond)
			},
		}

		sandbox := &controlplanev1alpha1.Sandbox{
			Metadata: &controlplanev1alpha1.SandboxMeta{Id: "sb-5"},
		}

		permanent := errors.New("redis is down")
		db.EXPECT().Create(ctx, sandbox).Return(nil)
		stream.EXPECT().Publish(ctx, gomock.Any()).Return(permanent).MinTimes(1)

		require.NoError(t, w.Create(ctx, sandbox), "Create must not propagate publish errors")

		errorLogs := observed.FilterLevelExact(zapcore.ErrorLevel).All()
		require.Len(t, errorLogs, 1, "expected exactly one error log")

		entry := errorLogs[0]
		assert.Contains(t, entry.Message, "publish")
		fields := entry.ContextMap()
		assert.Equal(t, "sb-5", fields["sandbox.id"])
	})
}
