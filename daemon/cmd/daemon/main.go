// Command daemon is the production entrypoint for the data-plane gRPC
// service. It loads a YAML configuration file, builds the firecracker
// provider (recovering pre-existing MicroVMs from the chroot), the
// network driver, and a gRPC client to the control plane, then hands
// off to commons/pkg/server.Start.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/commons/pkg/logger"
	"golang.nuinfra.net/commons/pkg/server"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	configPath := flag.String("config", "",
		"path to the YAML configuration file (required)")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("daemon: -config is required")
	}

	bundle, err := config.NewFromFile(*configPath)
	if err != nil {
		log.Fatalf("daemon: load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	zapLogger, err := logger.New("daemon", bundle.Logger)
	if err != nil {
		log.Fatalf("daemon: init logger: %v", err)
	}
	ctx = ctxzap.ToContext(ctx, zapLogger)

	provider, err := firecracker.New(
		firecracker.WithFirecrackerPath(filepath.Join(bundle.AssetsDir, bundle.Firecracker.BinaryName)),
		firecracker.WithJailerPath(filepath.Join(bundle.AssetsDir, bundle.Firecracker.JailerBinaryName)),
		firecracker.WithJailUID(bundle.Firecracker.JailUID),
		firecracker.WithJailGID(bundle.Firecracker.JailGID),
		firecracker.WithChrootBaseDir(bundle.Firecracker.ChrootBaseDir),
		firecracker.WithAttachConsole(bundle.Firecracker.AttachConsole),
	)
	if err != nil {
		zapLogger.Fatal("init firecracker provider", zap.Error(err))
	}

	// Re-attach to MicroVMs that survived a previous daemon process. A
	// failure here doesn't block startup — a fresh node has nothing to
	// recover, and an operator can always inspect the chroot manually
	// if recovery surfaces something unexpected.
	if err := provider.Recover(ctx); err != nil {
		zapLogger.Warn("firecracker recover", zap.Error(err))
	}

	netDrv := network.NewLinuxDriver(
		network.WithTAPOwner(bundle.Firecracker.JailUID),
		network.WithTAPGroup(bundle.Firecracker.JailGID),
	)

	// grpc.NewClient is non-blocking. The controller's connect loop
	// handles a temporarily unreachable api-server with backoff, so we
	// don't gate startup on api-server availability.
	conn, err := grpc.NewClient(
		bundle.APIServer.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		zapLogger.Fatal("dial api-server", zap.Error(err))
	}
	defer conn.Close()
	clusterClient := controlplanev1alpha1.NewClusterServiceClient(conn)
	sandboxClient := controlplanev1alpha1.NewSandboxServiceClient(conn)

	server.Start(ctx, service.New(provider, netDrv, clusterClient, sandboxClient, bundle))
}
