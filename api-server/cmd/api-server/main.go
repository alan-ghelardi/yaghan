// Command api-server is the production entrypoint for the control-plane
// gRPC service. It loads a YAML configuration file, attaches AWS
// dependencies to the context that pkg/service.Setup expects, and
// hands off to commons/pkg/server.Start which owns the gRPC + REST
// gateway lifecycle.
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"golang.nuinfra.api-server/pkg/config"
	"golang.nuinfra.api-server/pkg/service"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	awsdynamodb "golang.nuinfra.net/commons/pkg/aws/dynamodb"
	"golang.nuinfra.net/commons/pkg/logger"
	"golang.nuinfra.net/commons/pkg/server"
)

func main() {
	configPath := flag.String("config", "",
		"path to the YAML configuration file (required)")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("api-server: -config is required")
	}

	bundle, err := config.NewFromFile(*configPath)
	if err != nil {
		log.Fatalf("api-server: load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	zap, err := logger.New("api-server", bundle.Logger)
	if err != nil {
		log.Fatalf("api-server: init logger: %v", err)
	}
	ctx = ctxzap.ToContext(ctx, zap)

	// Setup expects an AWS SDK config and a DynamoDB client on the
	// context. The DynamoDB client honours bundle.Database.AWS.EndpointURL
	// — empty for real AWS, populated for DynamoDB Local in dev.
	awsCfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, awsCfg)
	ddb := awsdynamodb.New(ctx, awsdynamodb.Config{
		Endpoint: bundle.Database.AWS.EndpointURL,
	})
	ctx = awsdynamodb.With(ctx, ddb)

	server.Start(ctx, service.New(bundle))
}
