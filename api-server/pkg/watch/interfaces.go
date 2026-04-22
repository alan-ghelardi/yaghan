package watch

import (
	"context"

	"google.golang.org/protobuf/proto"
)

type WatchableStream[E proto.Message] interface {

	// GetWatchersCount returns the number of watchers associated to this
	// WatchableStream.
	GetWatchersCount() int

	// Publish sends the event to the event stream, enabling all interested
	// watchers to receive it.
	Publish(ctx context.Context, event E) error

	// Watch registers a watcher to receive events it is interested in.
	Watch(ctx context.Context, watcher *Watcher[E]) error

	// StopWatching unregisters a watcher by its ID, cleaning up resources
	// and stopping further event delivery.
	StopWatching(ctx context.Context, watcherID int64) error
}
