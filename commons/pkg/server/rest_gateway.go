package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	readTimeout = 15 * time.Second
)

// startRESTGateway starts the REST gateway with the provided options.
// The current Goroutine remains blocked until the supplied `ctx` is closed.
func startRESTGateway(ctx context.Context, service Service) error {
	mux := http.NewServeMux()

	gateway, err := newGateway(ctx, service)
	if err != nil {
		return err
	}

	config := service.GetConfig()

	mux.HandleFunc("/v1alpha1/openapi/v2", openAPIHandler(config.OpenAPIV2Path))

	mux.Handle("/", gateway)

	httpServer := &http.Server{
		Addr: fmt.Sprintf(":%d", config.RestPort),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		Handler:     cors.New(cors.Options{AllowedHeaders: []string{"authorization"}}).Handler(mux),
		ReadTimeout: readTimeout,
	}

	logger := ctxzap.Extract(ctx)

	go func() {
		logger.Info("Starting REST gateway", zap.Uint("rest.port", config.RestPort))

		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Error starting REST gateway", zap.Error(err))
		}
	}()

	<-ctx.Done()

	logger.Info("Shutting down REST gateway")

	if err := httpServer.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("unable to shut down REST gateway: %w", err)
	}

	return nil
}

func newGateway(ctx context.Context, service Service) (*runtime.ServeMux, error) {
	config := service.GetConfig()
	conn, err := grpc.NewClient(fmt.Sprintf("0.0.0.0:%d", config.GRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	healthCheckClient := grpc_health_v1.NewHealthClient(conn)

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:     true,
				EmitDefaultValues: false,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
		runtime.WithHealthEndpointAt(healthCheckClient, "/health"),
		runtime.WithMiddlewares(recoveryRESTHandler),
	)

	err = service.RegisterRESTGateway(ctx, mux, conn)
	if err != nil {
		return nil, err
	}

	return mux, nil
}

// recoveryRESTHandler is a middleware that recovers from a panic inside the gateway
// handlers.
func recoveryRESTHandler(handler runtime.HandlerFunc) runtime.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request, pathParams map[string]string) {
		defer func() {
			value := recover()
			if value != nil {
				logger := ctxzap.Extract(request.Context())
				logger.Error("recovered from panic inside REST gateway handlers", zap.Any("error", value))

				response.Header().Set("Content-Type", "text/plain")
				http.Error(response, "there's been a glitch handling this request", http.StatusInternalServerError)
			}
		}()

		handler(response, request, pathParams)
	}
}

// openAPIHandler returns OpenAPI specification files located under "/openapiv2/"
func openAPIHandler(filePath string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, err := os.Stat(filePath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.Error(response, "no Open API document available", http.StatusNotFound)
			} else {
				http.Error(response, "unable to load Open API document", http.StatusInternalServerError)
			}
			return
		}
		http.ServeFile(response, request, filePath)
	}
}
