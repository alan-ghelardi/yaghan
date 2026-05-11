package service

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	dataplanepb "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
	"golang.nuinfra.net/commons/pkg/aws/s3"
	commonsconfig "golang.nuinfra.net/commons/pkg/config"
	"golang.nuinfra.net/commons/pkg/server"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/controller"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/node"
	"golang.nuinfra.net/daemon/pkg/node/metrics"
	"golang.nuinfra.net/daemon/pkg/reconciler"
	"golang.nuinfra.net/daemon/pkg/snapshot"
	"google.golang.org/grpc"
)

type daemon struct {
	server.BaseService
	dataplanepb.UnimplementedDaemonServiceServer

	firecracker firecracker.Provider

	networkDriver network.Driver

	clusterServiceClient controlplanev1alpha1.ClusterServiceClient
	sandboxServiceClient controlplanev1alpha1.SandboxServiceClient

	// config is a bundle containing the server configurations.
	config *config.Bundle
}

var _ server.Service = (*daemon)(nil)

// New returns a new [server.Service] instance.
func New(firecracker firecracker.Provider, networkDriver network.Driver, clusterServiceClient controlplanev1alpha1.ClusterServiceClient, sandboxServiceClient controlplanev1alpha1.SandboxServiceClient, config *config.Bundle) server.Service {
	return &daemon{
		firecracker:          firecracker,
		networkDriver:        networkDriver,
		clusterServiceClient: clusterServiceClient,
		sandboxServiceClient: sandboxServiceClient,
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
	// Snapshot durable storage is optional: deployments that don't need
	// off-host snapshots (local development, tests) leave Snapshots unset
	// and the reconciler is wired with a nil store. The snapshot
	// reconcile path is only entered when Intent.CreateSnapshot is true,
	// so nil is safe for configurations that never request one.
	var snapshotStore *snapshot.Store
	if d.config.Snapshots != nil && d.config.Snapshots.S3 != nil {
		s3Config := s3.Config{
			Endpoint:     d.config.Snapshots.S3.Endpoint,
			UsePathStyle: d.config.Snapshots.S3.UsePathStyle,
		}
		uploader := s3.NewUploader(ctx, s3Config)
		downloader := s3.NewDownloader(ctx, s3Config)
		durableStore := snapshot.NewS3DurableStore(uploader, downloader, d.config.Snapshots.S3.BucketName)
		snapshotStore = snapshot.NewStore(durableStore)
	}

	reconciler := reconciler.New(d.config, d.firecracker, d.networkDriver, snapshotStore)

	// The controller and node Agent share a circular dependency: the
	// Agent needs the controller as its node.Reporter, while the
	// controller needs the Agent to build the Node identity at connect
	// time. We resolve it by constructing the controller first, then
	// wiring the Agent via SetAgent before Run.
	ctrl := controller.New(d.clusterServiceClient, d.sandboxServiceClient, reconciler, d.config)
	metricsCollector := metrics.NewCollector(d.config)
	agent := node.NewAgent(ctx, d.config, d.firecracker, metricsCollector, ctrl)
	ctrl.SetAgent(agent)

	go func() {
		err := ctrl.Run(ctx)
		if err == nil {
			return
		}
		logger := ctxzap.Extract(ctx)
		// A non-nil error after ctx cancellation is normal shutdown
		// (e.g. an in-flight metrics collection wrapping ctx.Err()).
		// Don't Fatal in that case — it would tear the process down
		// during graceful shutdown and turn clean exits into errors
		// in tests.
		if ctx.Err() != nil {
			logger.Warn("setup: controller exited during shutdown", zap.Error(err))
			return
		}
		logger.Fatal("setup: failed to start controller", zap.Error(err))
	}()

	return nil
}
