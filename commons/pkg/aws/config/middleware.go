package config

import (
	"context"
	"time"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap/zapcore"
)

type apiMetadata struct {
	serviceID     string
	operationName string
}

type apiMetadataKey struct{}

var tracer = middleware.InitializeMiddlewareFunc("nuinfra.tracer", func(ctx context.Context, input middleware.InitializeInput, next middleware.InitializeHandler) (output middleware.InitializeOutput, metadata middleware.Metadata, err error) {
	startTime := time.Now()
	logger := ctxzap.Extract(ctx).Sugar()
	output, metadata, err = next.HandleInitialize(ctx, input)
	if logger.Level() == zapcore.DebugLevel {
		apiMeta := metadata.Get(apiMetadataKey{}).(apiMetadata)

		logger.Debugf("Called %s.%s in %s", apiMeta.serviceID, apiMeta.operationName, time.Since(startTime).String())
	}

	return
})

var metadata = middleware.InitializeMiddlewareFunc("nuinfra.metadata", func(ctx context.Context, input middleware.InitializeInput, next middleware.InitializeHandler) (output middleware.InitializeOutput, metadata middleware.Metadata, err error) {
	output, metadata, err = next.HandleInitialize(ctx, input)

	metadata.Set(apiMetadataKey{}, apiMetadata{
		serviceID:     awsmiddleware.GetServiceID(ctx),
		operationName: awsmiddleware.GetOperationName(ctx),
	})

	return
})
