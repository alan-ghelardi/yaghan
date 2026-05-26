// Command daemon is the production entrypoint for the data-plane gRPC
// service. It loads a YAML configuration file, builds the firecracker
// provider (recovering pre-existing MicroVMs from the chroot), the
// network driver, and a gRPC client to the control plane, then hands
// off to commons/pkg/server.Start.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os/signal"
	"path/filepath"
	"syscall"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	"github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2"
	"github.com/alan-ghelardi/yaghan/commons/pkg/aws/ec2imds"
	"github.com/alan-ghelardi/yaghan/commons/pkg/logger"
	"github.com/alan-ghelardi/yaghan/commons/pkg/server"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/config"
	yaghandns "github.com/alan-ghelardi/yaghan/daemon/pkg/dns"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/firecracker"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/network"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/network/firewall"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/service"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	miekgdns "github.com/miekg/dns"
	"go.uber.org/zap"
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
	var fw firewall.Firewall
	if bundle.Network != nil && bundle.Network.EgressEnabled {
		setupFw, err := setupEgress(ctx, bundle, zapLogger)
		if err != nil {
			zapLogger.Fatal("setup egress connectivity", zap.Error(err))
		}
		fw = setupFw
		netOpts = append(netOpts, network.WithFirewall(fw))
	} else {
		zapLogger.Warn("egress connectivity disabled — sandboxes will not reach the outside network")
	}

	// DNS responder: enforces per-sandbox domain_names egress
	// policies. The PolicyStore is nil when the responder is
	// disabled — the reconciler treats that as "domain_names is
	// unenforceable" and rejects boots that need it. The dummy
	// interface and resolver IP are owned by the daemon, so the
	// network driver must know the resolver address before the
	// first sandbox is provisioned — that's why this runs before
	// NewLinuxDriver.
	var (
		dnsPolicies *yaghandns.PolicyStore
		resolverIP  netip.Addr
	)
	if bundle.Network != nil && bundle.Network.DNS != nil && bundle.Network.DNS.Enabled {
		dnsPolicies, resolverIP = setupDNS(ctx, bundle, zapLogger)
		netOpts = append(netOpts, network.WithResolverIP(resolverIP))
		// Punch a hole in the host's INPUT chain so DNS traffic
		// from the sandbox veth pool reaches the responder. Skipped
		// when the firewall package is disabled — the operator is
		// then responsible for installing the equivalent rule.
		if fw != nil {
			if err := fw.EnsureDNSAccess(resolverIP, network.VMSubnet()); err != nil {
				zapLogger.Fatal("dns: install INPUT carve-out", zap.Error(err))
			}
		}
	} else {
		zapLogger.Warn("DNS responder disabled — sandboxes referencing domain_names egress targets will fail to boot")
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

	server.Start(ctx, service.New(bundle, provider, netDrv, dnsPolicies, clusterClient, sandboxClient, snapshotClient))
}

// setupDNS wires the in-process DNS responder:
//
//  1. Reserves a daemon-owned IP on a dummy interface so the bind
//     dodges host-resolver conflicts (typically systemd-resolved on
//     127.0.0.53).
//  2. Builds the PolicyStore (shared with the reconciler), an
//     upstream forwarder, and an ipset-backed snooper.
//  3. Starts UDP+TCP listeners bound to the dummy IP.
//
// Returns the PolicyStore the reconciler should populate and the
// resolver IP (handed to the network driver as
// [network.WithResolverIP] and used in [reconciler.buildBootArgs]).
//
// Fatal on any setup error: the daemon was explicitly configured to
// enforce domain_names, so silently coming up without a responder
// would be a security regression.
func setupDNS(ctx context.Context, bundle *config.Bundle, zapLogger *zap.Logger) (*yaghandns.PolicyStore, netip.Addr) {
	cfg := bundle.Network.DNS
	listenIP, err := netip.ParseAddr(cfg.ListenIP)
	if err != nil {
		zapLogger.Fatal("dns: invalid network.dns.listen-ip", zap.String("value", cfg.ListenIP), zap.Error(err))
	}
	// The dummy interface carries the listen-IP. The kernel will
	// route packets destined to it locally; the listener can bind
	// directly to the address without colliding with whatever
	// owns the host's default IPs on port 53.
	if err := network.EnsureDummyInterface(cfg.DummyInterface, listenIP); err != nil {
		zapLogger.Fatal("dns: provision dummy interface", zap.String("iface", cfg.DummyInterface), zap.Error(err))
	}

	store := yaghandns.NewPolicyStore()
	upstream, err := yaghandns.NewDefaultUpstream(cfg.Upstream, cfg.ForwardTimeout)
	if err != nil {
		zapLogger.Fatal("dns: build upstream forwarder", zap.Error(err))
	}
	snooper := yaghandns.NewIPSetSnooper(firewall.NewIPSet())
	handler, err := yaghandns.NewHandler(yaghandns.HandlerOptions{
		Store:          store,
		Upstream:       upstream,
		Snooper:        snooper,
		MinEntryTTL:    cfg.MinEntryTTL,
		MaxEntryTTL:    cfg.MaxEntryTTL,
		ForwardTimeout: cfg.ForwardTimeout,
		Logger:         zapAdapter{logger: zapLogger},
	})
	if err != nil {
		zapLogger.Fatal("dns: build handler", zap.Error(err))
	}
	bind := fmt.Sprintf("%s:%d", listenIP, cfg.Port)
	srv, err := yaghandns.NewServer(yaghandns.ServerOptions{
		Bind:    bind,
		Handler: miekgdns.HandlerFunc(handler.ServeDNS),
	})
	if err != nil {
		zapLogger.Fatal("dns: build server", zap.Error(err))
	}
	if err := srv.Start(ctx); err != nil {
		zapLogger.Fatal("dns: start server", zap.Error(err))
	}
	zapLogger.Info("DNS responder started",
		zap.String("bind", bind),
		zap.String("iface", cfg.DummyInterface),
		zap.Strings("upstream", cfg.Upstream))

	// Surface a listener failure asynchronously. The error is rare
	// (typically EADDRINUSE on startup, which the Fatal in Start
	// would have already caught) so we log it loudly and let the
	// daemon supervisor restart us rather than tearing down here.
	go func() {
		if err := srv.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
			zapLogger.Error("dns: listener exited", zap.Error(err))
		}
	}()
	return store, listenIP
}

// zapAdapter bridges the dns package's narrow [yaghandns.Logger] to
// zap. Kept local so the dns package stays free of a zap dep.
type zapAdapter struct {
	logger *zap.Logger
}

func (a zapAdapter) Warn(msg string, fields ...any)  { a.logger.Warn(msg, toZapFields(fields)...) }
func (a zapAdapter) Debug(msg string, fields ...any) { a.logger.Debug(msg, toZapFields(fields)...) }

// toZapFields converts a flat "k, v, k, v" slice into typed zap
// fields. Mismatched-length input is tolerated (extra orphan key is
// dropped) so a programming error in a log site doesn't blow up the
// path it's logging.
func toZapFields(fields []any) []zap.Field {
	out := make([]zap.Field, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		k, ok := fields[i].(string)
		if !ok {
			continue
		}
		out = append(out, zap.Any(k, fields[i+1]))
	}
	return out
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
