package factory

import (
	"context"

	"golang.nuinfra.api-server/pkg/config"
	"golang.nuinfra.api-server/pkg/watch"
	"golang.nuinfra.api-server/pkg/watch/providers/redis"
	"google.golang.org/protobuf/proto"
)

// NewEventStream returns a new WatchableStream based on the supplied configuration.
func NewEventStream[E proto.Message](ctx context.Context, config *config.Bundle, setEventID redis.SetEventIDFunc[E]) (watch.WatchableStream[E], error) {
	if config.WatchStream == nil || config.WatchStream.Redis == nil || len(config.WatchStream.Redis.Address) == 0 {
		return &noopStream[E]{}, nil
	}
	return redis.NewFromConfig[E](ctx, config.WatchStream, setEventID)
}
