package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alan-ghelardi/yaghan/commons/pkg/server"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	// Embeds the Zap logger development configuration
	_ "embed"

	baseconfig "github.com/alan-ghelardi/yaghan/commons/pkg/config"
	"github.com/alan-ghelardi/yaghan/commons/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

// //go:embed zap-config.json
var zapDevConfig string

// StartServer starts the specified gRPC service in a background goroutine for use in tests.
//
// It initializes the logger, then invokes server.Start to
// boot the service. Once the service is started,
// StartServer waits until the gRPC health endpoint reports that the server is
// ready to serve requests.
//
// The function registers a cleanup hook with t.Cleanup to ensure the background
// goroutine terminates when the test completes.
//
// Example:
//
//	func TestMyService(t *testing.T) {
//	    ctx := context.Background()
//	    StartServer(ctx, t, myService)
//	    // ... perform test logic ...
//	}
func StartServer(ctx context.Context, t *testing.T, service server.Service) {
	StartServerWithCredentials(ctx, t, service, insecure.NewCredentials())
}

// StartServerWithCredentials starts the specified gRPC service in a background goroutine for use in tests.
//
// It initializes the logger, then invokes server.Start to
// boot the service using the provided credentials. Once the service is started,
// StartServerWithCredentials waits until the gRPC health endpoint reports that the server is
// ready to serve requests.
//
// The function registers a cleanup hook with t.Cleanup to ensure the background
// goroutine terminates when the test completes.
//
// Example:
//
//	func TestMyService(t *testing.T) {
//	    ctx := context.Background()
//	    creds := insecure.NewCredentials()
//	    StartServerWithCredentials(ctx, t, myService, creds)
//	    // ... perform test logic ...
//	}
func StartServerWithCredentials(ctx context.Context, t *testing.T, service server.Service, creds credentials.TransportCredentials) {
	t.Helper()

	config := service.GetConfig()
	loggerConfig := config.Logger
	if loggerConfig == nil {
		loggerConfig = &logger.Config{
			LogLevel:  zap.NewAtomicLevelAt(zapcore.DebugLevel),
			ZapConfig: json.RawMessage(zapDevConfig),
		}
	}

	zapLogger, err := logger.New("test", loggerConfig)
	if err != nil {
		t.Fatalf("failed to create Zap logger: %v", err)
	}

	ctx = ctxzap.ToContext(ctx, zapLogger)

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Start(ctx, service)
	}()

	t.Cleanup(func() {
		<-done
	})

	waitForServerHealth(ctx, t, config, creds)
}

// waitForServerHealth blocks until the gRPC server reports a healthy state
// via the standard gRPC health checking service.
//
// It attempts to connect to the server using the provided configuration and
// credentials, then calls the Health.Check RPC. The function fails the test if
// the connection cannot be established, the RPC call fails, or the returned
// health status is not SERVING.
func waitForServerHealth(ctx context.Context, t *testing.T, config baseconfig.Base, creds credentials.TransportCredentials) {
	t.Helper()

	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", config.GRPCPort),
		grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("failed to create health check client: %v", err)
	}

	healthClient := grpc_health.NewHealthClient(conn)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	response, err := healthClient.Check(ctx, &grpc_health.HealthCheckRequest{},
		grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("failed to verify server health: %v", err)
	}

	if status := response.Status; status != grpc_health.HealthCheckResponse_SERVING {
		t.Fatalf("GRPC server is unhealthy: status is %v", status)
	}
}
