package server

import (
	"context"

	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

type healthServer struct {
	grpc_health.UnimplementedHealthServer

	service Service
}

func (h *healthServer) Check(ctx context.Context, _ *grpc_health.HealthCheckRequest) (*grpc_health.HealthCheckResponse, error) {
	return h.service.HealthCheck(ctx)
}
