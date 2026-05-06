package service

import (
	"context"
	"fmt"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.nuinfra.api-server/pkg/config"
	nodedb "golang.nuinfra.api-server/pkg/db/node"
	nodedynamodb "golang.nuinfra.api-server/pkg/db/node/dynamodb"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	"golang.nuinfra.api-server/pkg/db/sandbox/dynamodb"
	"golang.nuinfra.api-server/pkg/watch"
	"golang.nuinfra.api-server/pkg/watch/factory"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	commonsconfig "golang.nuinfra.net/commons/pkg/config"
	"golang.nuinfra.net/commons/pkg/server"
	"google.golang.org/grpc"
)

type apiServer struct {
	server.BaseService
	cpv1.UnimplementedClusterServiceServer
	cpv1.UnimplementedSandboxServiceServer

	// config is a bundle containing the server configurations.
	config *config.Bundle

	// db is the database instance to persist and retrieve sandboxes; backed by
	// a *WatchableDB so successful writes also emit events on eventStream.
	db sandboxdb.DB

	// nodeDB is the database instance to persist and retrieve nodes. Reads
	// (GetNode, ListNodes) flow straight through; there is no Watchable
	// wrapper yet because no node-write RPC exists today.
	nodeDB nodedb.DB

	// eventStream is the WatchableStream EstablishSession registers watchers
	// against. Reads of past events go through the redis stream's last-event-id
	// machinery; new events are published via the *WatchableDB above.
	eventStream watch.WatchableStream[*cpv1.Event]

	// nodes tracks Nodes that currently hold an active EstablishSession.
	nodes *nodeRegistry
}

var _ server.Service = (*apiServer)(nil)

// New returns a new [server.Service] instance.
func New(config *config.Bundle) server.Service {
	return &apiServer{config: config}
}

// GetConfig implements [server.Service].
func (a *apiServer) GetConfig() commonsconfig.Base {
	return a.config.Base
}

// RegisterGRPC implements [server.Service].
func (a *apiServer) RegisterGRPC(_ context.Context, grpcServer *grpc.Server) error {
	cpv1.RegisterSandboxServiceServer(grpcServer, a)
	cpv1.RegisterClusterServiceServer(grpcServer, a)
	return nil
}

// RegisterRESTGateway implements [server.Service].
func (a *apiServer) RegisterRESTGateway(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
	if err := cpv1.RegisterSandboxServiceHandler(ctx, mux, conn); err != nil {
		return err
	}
	return cpv1.RegisterClusterServiceHandler(ctx, mux, conn)
}

// Setup implements [server.Service].
func (a *apiServer) Setup(ctx context.Context) error {
	rawDB := dynamodb.New(ctx, a.config)

	stream, err := factory.NewEventStream(ctx, a.config, setSandboxEventID)
	if err != nil {
		return fmt.Errorf("unable to create event stream: %w", err)
	}

	a.eventStream = stream
	a.db = NewWatchableDB(rawDB, stream)
	a.nodeDB = nodedynamodb.New(ctx, a.config)
	a.nodes = newNodeRegistry()
	return nil
}

// setSandboxEventID is the SetEventIDFunc passed to the redis-backed event
// stream: the provider stamps each delivered Event with the corresponding
// redis stream message id so consumers can resume from a known offset.
func setSandboxEventID(event *cpv1.Event, eventID string) {
	event.Id = eventID
}
