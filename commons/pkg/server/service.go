package server

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.nuinfra.net/commons/pkg/config"
	"google.golang.org/grpc"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

// Service defines the lifecycle hooks and registration methods for a gRPC-based service.
// It is intended to be implemented by components that expose gRPC APIs and want to integrate
// with the standard server startup and shutdown pipeline.
type Service interface {
	// GetConfig returns the configuration for the service, including ports, TLS, and auth settings.
	GetConfig() config.Base

	// Setup is called before the gRPC server starts. It can be used to initialize dependencies,
	// load data, validate configuration, or prepare internal state.
	Setup(ctx context.Context) error

	// BeforeShutdown is called when the context is canceled but before the gRPC server stops.
	// Use it to begin graceful termination, notify subsystems, or cancel background tasks.
	BeforeShutdown(ctx context.Context)

	// AfterShutdown is called after the gRPC server has fully stopped.
	// Use it to release resources, close clients, or perform final cleanup.
	AfterShutdown(ctx context.Context) error

	// RegisterGRPC registers the gRPC service implementation(s) with the given server.
	// This is typically where generated gRPC code (e.g., pb.RegisterFooServer) is invoked.
	RegisterGRPC(ctx context.Context, grpcServer *grpc.Server) error

	// RegisterRESTGateway registers HTTP handlers with the grpc-gateway mux to expose
	// the gRPC service over a REST/JSON API.
	RegisterRESTGateway(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

	// HealthCheck returns the current health status of the service.
	// This is used by the standard gRPC health check endpoint.
	HealthCheck(ctx context.Context) (*grpc_health.HealthCheckResponse, error)

	// UnaryServerInterceptors returns additional unary interceptors to be included in
	// the gRPC middleware chain.
	UnaryServerInterceptors(ctx context.Context) []grpc.UnaryServerInterceptor

	// StreamServerInterceptors returns additional stream interceptors to be included in
	// the gRPC middleware chain.
	StreamServerInterceptors(ctx context.Context) []grpc.StreamServerInterceptor
}

// BaseService provides default implementations for most methods of the Service interface.
// Implementors can embed BaseService in their own structs and override only the methods
// they need to customize. Note that BaseService does not implement GetConfig, RegisterGRPC,
// or RegisterRESTGateway—these must be implemented by the concrete service.
type BaseService struct {
}

// AfterShutdown implements Service.
func (b *BaseService) AfterShutdown(context.Context) error {
	return nil
}

// BeforeShutdown implements Service.
func (b *BaseService) BeforeShutdown(context.Context) {
}

// HealthCheck implements Service.
func (b *BaseService) HealthCheck(context.Context) (*grpc_health.HealthCheckResponse, error) {
	return &grpc_health.HealthCheckResponse{Status: grpc_health.HealthCheckResponse_SERVING}, nil
}

// Setup implements Service.
func (b *BaseService) Setup(context.Context) error {
	return nil
}

// StreamServerInterceptors implements Service.
func (b *BaseService) StreamServerInterceptors(context.Context) []grpc.StreamServerInterceptor {
	return nil
}

// UnaryServerInterceptors implements Service.
func (b *BaseService) UnaryServerInterceptors(context.Context) []grpc.UnaryServerInterceptor {
	return nil
}
