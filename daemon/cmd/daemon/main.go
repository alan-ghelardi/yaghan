// Command daemon is the production entrypoint for the data-plane gRPC
// service. It loads a YAML configuration file, builds the firecracker
// provider (recovering pre-existing MicroVMs from the chroot), the
// network driver, and a gRPC client to the control plane, then hands
// off to commons/pkg/server.Start.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/ec2"
	"golang.nuinfra.net/commons/pkg/aws/ec2imds"
	"golang.nuinfra.net/commons/pkg/logger"
	"golang.nuinfra.net/commons/pkg/server"
	"golang.nuinfra.net/daemon/pkg/config"
	"golang.nuinfra.net/daemon/pkg/firecracker"
	"golang.nuinfra.net/daemon/pkg/network"
	"golang.nuinfra.net/daemon/pkg/network/firewall"
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

	if bundle.NodeAgent.Runtime == config.NodeRuntimeEC2 || (bundle.Snapshots != nil && bundle.Snapshots.S3 != nil) {
		ctx = awsconfig.With(ctx, awsconfig.New(ctx))
	}

	if bundle.NodeAgent.Runtime == config.NodeRuntimeEC2 {
		ctx = ec2.With(ctx, ec2.New(ctx, ec2.Config{}))
		ctx = ec2imds.With(ctx, ec2imds.New(ctx, ec2imds.Config{}))
	}

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

	netOpts := []network.Option{
		network.WithTAPOwner(bundle.Firecracker.JailUID),
		network.WithTAPGroup(bundle.Firecracker.JailGID),
	}
	if bundle.Network != nil && bundle.Network.EgressEnabled {
		fw, err := setupEgress(ctx, bundle, zapLogger)
		if err != nil {
			zapLogger.Fatal("setup egress connectivity", zap.Error(err))
		}
		netOpts = append(netOpts, network.WithFirewall(fw))
	} else {
		zapLogger.Warn("egress connectivity disabled — sandboxes will not reach the outside network")
	}
	netDrv := network.NewLinuxDriver(netOpts...)

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
	snapshotClient := controlplanev1alpha1.NewSnapshotServiceClient(conn)

	server.Start(ctx, service.New(provider, netDrv, clusterClient, sandboxClient, snapshotClient, bundle))
}

// setupEgress wires up host-side egress connectivity: enables IPv4
// forwarding, resolves the upstream device (auto-detect when not set
// in config), and installs the host-wide MASQUERADE/FORWARD rules.
// Returns the firewall the per-VM driver will use for namespace
// configuration. Idempotent — safe to call on every daemon start.
func setupEgress(ctx context.Context, bundle *config.Bundle, logger *zap.Logger) (firewall.Firewall, error) {
	_ = ctx // reserved for future cancellation-aware steps

	if err := network.EnableHostIPForward(); err != nil {
		return nil, fmt.Errorf("enable host ip_forward: %w", err)
	}

	upstream := bundle.Network.UpstreamDevice
	if upstream == "" {
		detected, err := network.DefaultUpstreamDevice()
		if err != nil {
			return nil, fmt.Errorf("auto-detect upstream device (set network.upstream-device explicitly): %w", err)
		}
		upstream = detected
		logger.Info("auto-detected upstream egress device", zap.String("device", upstream))
	}

	fw, err := firewall.NewIPTables()
	if err != nil {
		return nil, fmt.Errorf("init firewall: %w", err)
	}
	if err := fw.EnsureHost(upstream, network.VMSubnet()); err != nil {
		return nil, fmt.Errorf("install host firewall rules on %s: %w", upstream, err)
	}
	logger.Info("egress connectivity configured",
		zap.String("upstream", upstream),
		zap.String("vm-subnet", network.VMSubnet().String()))
	return fw, nil
}
