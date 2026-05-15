package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	// Sets GOMAXPROCS to match the Linux container CPU quota so callers
	// don't need to import the automaxprocs package in their main.go files.
	_ "go.uber.org/automaxprocs"

	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"

	"github.com/alan-ghelardi/yaghan/commons/pkg/auth"
	"github.com/alan-ghelardi/yaghan/commons/pkg/validation"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

// listenerKey is the context key for an optional pre-bound gRPC listener.
type listenerKey struct{}

// WithListener returns a context that carries a pre-bound net.Listener for the
// gRPC server to use instead of binding its own. Intended for tests that need
// to allocate a free port atomically and pass it to the server, avoiding the
// race window between picking a free port and the server binding to it.
//
// Production callers should not use this — leave it unset and the server will
// bind to GRPCPort from its config as usual.
func WithListener(ctx context.Context, listener net.Listener) context.Context {
	return context.WithValue(ctx, listenerKey{}, listener)
}

// listenerFromCtx returns the pre-bound listener attached via WithListener, or
// nil if none has been set.
func listenerFromCtx(ctx context.Context) net.Listener {
	l, _ := ctx.Value(listenerKey{}).(net.Listener)
	return l
}

// Start launches the gRPC service along with the optional REST gateway (if configured).
// It initializes the service, starts the gRPC server in a background goroutine, and
// optionally starts the grpc-gateway-based REST server.
//
// The function blocks until the provided context is canceled. Upon cancellation,
// it triggers the service shutdown lifecycle, including BeforeShutdown and AfterShutdown.
//
// This function is intended to be called from the main function of a service binary.
func Start(ctx context.Context, service Service) {
	var shutdownGroup sync.WaitGroup

	logger := ctxzap.Extract(ctx)

	shutdownGroup.Add(1)
	go func() {
		defer shutdownGroup.Done()

		if err := startGRPCServer(ctx, service); err != nil {
			logger.Fatal("failed to start gRPC server", zap.Error(err))
		}
	}()

	if service.GetConfig().RestPort > 0 {
		shutdownGroup.Add(1)
		go func() {
			defer shutdownGroup.Done()

			if err := startRESTGateway(ctx, service); err != nil {
				logger.Fatal("failed to start rest gateway", zap.Error(err))
			}
		}()
	}

	shutdownGroup.Wait()
}

func startGRPCServer(ctx context.Context, service Service) error {
	logger := ctxzap.Extract(ctx)

	err := service.Setup(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize service: %w", err)
	}

	defer func() {
		if err := service.AfterShutdown(ctx); err != nil {
			logger.Error("failed to execute after shutdown hook", zap.Error(err))
		}
	}()

	config := service.GetConfig()

	creds := insecure.NewCredentials()
	if tls := config.TLS; tls != nil {
		cert, err := tls.LoadCertificate()
		if err != nil {
			return err
		}
		creds = credentials.NewServerTLSFromCert(cert)
	}

	listener := listenerFromCtx(ctx)
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", config.GRPCPort))
		if err != nil {
			return fmt.Errorf("failed to listen on port %d", config.GRPCPort)
		}
	}

	serviceUnaryInterceptors := service.UnaryServerInterceptors(ctx)
	unaryServerInterceptors := make([]grpc.UnaryServerInterceptor, 0, 5+len(serviceUnaryInterceptors))
	unaryServerInterceptors = append(unaryServerInterceptors,
		grpc_ctxtags.UnaryServerInterceptor(),
		grpc_zap.UnaryServerInterceptor(logger),
		auth.UnaryServerInterceptor(ctx, config.Auth),
		validation.UnaryServerInterceptor(ctx),
	)
	unaryServerInterceptors = append(unaryServerInterceptors, serviceUnaryInterceptors...)
	unaryServerInterceptors = append(unaryServerInterceptors, grpc_recovery.UnaryServerInterceptor(gRPCRecoveryHandler()))

	serviceStreamInterceptors := service.StreamServerInterceptors(ctx)
	streamServerInterceptors := make([]grpc.StreamServerInterceptor, 0, 4+len(serviceStreamInterceptors))
	streamServerInterceptors = append(streamServerInterceptors,
		grpc_ctxtags.StreamServerInterceptor(),
		grpc_zap.StreamServerInterceptor(logger),
		auth.StreamServerInterceptor(ctx, config.Auth),
	)
	streamServerInterceptors = append(streamServerInterceptors, serviceStreamInterceptors...)
	streamServerInterceptors = append(streamServerInterceptors, grpc_recovery.StreamServerInterceptor(gRPCRecoveryHandler()))

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             config.KeepAlive.MinTime,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Timeout: config.KeepAlive.Timeout,
		}),
		grpc.MaxConcurrentStreams(config.MaxConcurrentStreams),
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(unaryServerInterceptors...)),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(streamServerInterceptors...),
		))

	if err := service.RegisterGRPC(ctx, grpcServer); err != nil {
		return fmt.Errorf("failed to register gRPC service: %w", err)
	}

	grpc_health.RegisterHealthServer(grpcServer, &healthServer{service: service})
	reflection.Register(grpcServer)

	go func() {
		logger.Info("Starting GRPC server", zap.Any("grpc.address", listener.Addr()))
		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal("failed to serve", zap.Error(err))
		}
	}()

	<-ctx.Done()

	logger.Info("Shutting down GRPC server")

	service.BeforeShutdown(ctx)

	grpcServer.GracefulStop()
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("failed to close TCP listener: %w", err)
	}

	return nil
}

// gRPCRecoveryHandler is a custom handler for the GRPC recovery middleware. It logs
// the panic's message and returns an internal error.
func gRPCRecoveryHandler() grpc_recovery.Option {
	return grpc_recovery.WithRecoveryHandlerContext(func(ctx context.Context, panicMsg any) (err error) {
		log := ctxzap.Extract(ctx)
		log.Error("Recovered from panic", zap.Any("error", panicMsg))
		return status.Errorf(codes.Internal, "there's been a glitch handling this message")
	})
}
