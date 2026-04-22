package service

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	dataplanepb "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	commonsconfig "golang.nuinfra.net/commons/pkg/config"
	"golang.nuinfra.net/commons/pkg/server"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/controller"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/reconciler"
	"google.golang.org/grpc"
)

type daemon struct {
	server.BaseService
	dataplanepb.UnimplementedDaemonServiceServer

	firecracker firecracker.Provider

	networkDriver network.Driver

	clusterServiceClient controlplanev1alpha1.ClusterServiceClient

	// config is a bundle containing the server configurations.
	config *config.Bundle
}

var _ server.Service = (*daemon)(nil)

// New returns a new [server.Service] instance.
func New(firecracker firecracker.Provider, networkDriver network.Driver, clusterServiceClient controlplanev1alpha1.ClusterServiceClient, config *config.Bundle) server.Service {
	return &daemon{
		firecracker:          firecracker,
		networkDriver:        networkDriver,
		clusterServiceClient: clusterServiceClient,
		config:               config,
	}
}

// GetConfig implements [server.Service].
func (d *daemon) GetConfig() commonsconfig.Base {
	return d.config.Base
}

// RegisterGRPC implements [server.Service].
func (d *daemon) RegisterGRPC(_ context.Context, grpcServer *grpc.Server) error {
	dataplanepb.RegisterDaemonServiceServer(grpcServer, d)
	return nil
}

// RegisterRESTGateway implements [server.Service].
func (d *daemon) RegisterRESTGateway(context.Context, *runtime.ServeMux, *grpc.ClientConn) error {
	return nil
}

// Setup implements [server.Service].
func (d *daemon) Setup(ctx context.Context) error {
	reconciler := reconciler.New(d.firecracker, d.networkDriver, d.config)

	controller := controller.New(d.clusterServiceClient, reconciler, d.config)
	go func() {
		err := controller.Run(ctx)
		if err != nil {
			logger := ctxzap.Extract(ctx)
			logger.Fatal("setup: failed to start controller", zap.Error(err))
		}
	}()

	return nil
}
