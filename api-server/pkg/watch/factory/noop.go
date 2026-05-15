package factory

import (
	"context"
	"errors"

	"github.com/alan-ghelardi/yaghan/api-server/pkg/watch"
	"google.golang.org/protobuf/proto"
)

type noopStream[E proto.Message] struct {
}

// GetWatchersCount implements WatchableStream.
func (n *noopStream[E]) GetWatchersCount() int {
	return 0
}

// Publish implements WatchableStream.
func (n *noopStream[E]) Publish(context.Context, E) error {
	return nil
}

// StopWatching implements WatchableStream.
func (n *noopStream[E]) StopWatching(context.Context, int64) error {
	return nil
}

// Watch implements WatchableStream.
func (n *noopStream[E]) Watch(context.Context, *watch.Watcher[E]) error {
	return errors.New("event stream is not enabled in this server")
}
