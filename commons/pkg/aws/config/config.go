package config

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awslogging "github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

// loggerAdapter wraps a zap.SugaredLogger by adapting it to the Logger and
// ContextLogger interfaces from the awslogging package.
type loggerAdapter struct {
	logger *zap.SugaredLogger
}

var (
	_ awslogging.Logger        = (*loggerAdapter)(nil)
	_ awslogging.ContextLogger = (*loggerAdapter)(nil)
)

// Logf implements awslogging.Logger.
func (a *loggerAdapter) Logf(_ awslogging.Classification, format string, args ...any) {
	a.logger.Warnf(format, args...)
}

// WithContext implements awslogging.ContextLogger.
func (a *loggerAdapter) WithContext(ctx context.Context) awslogging.Logger {
	return &loggerAdapter{ctxzap.Extract(ctx).Sugar()}
}

// key attaches aws.Config objects to the Context.
type key struct {
}

// Get returns the aws.Config object attached to the supplied Context or panics
// if no one can be found.
func Get(ctx context.Context) aws.Config {
	config, ok := ctx.Value(key{}).(aws.Config)
	if !ok {
		logger := ctxzap.Extract(ctx)
		logger.Panic("no aws.Config could be found in the provided Context")
	}
	return config
}

// With attaches the aws.Config object to the context.
func With(ctx context.Context, awsConfig aws.Config) context.Context {
	return context.WithValue(ctx, key{}, awsConfig)
}

// New returns a new aws.Config object or panics if the config cannot be loaded.
func New(ctx context.Context) aws.Config {
	logger := ctxzap.Extract(ctx).Sugar()

	awsConfig, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithClientLogMode(aws.LogRetries),
		awsconfig.WithLogger(&loggerAdapter{logger: logger}),
		awsconfig.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(opts *retry.StandardOptions) {
				opts.MaxAttempts = 7
			})
		}),
	)

	if err != nil {
		logger.Panicw("error loading AWS config", zap.Error(err))
	}

	awsConfig.APIOptions = append(awsConfig.APIOptions, func(stack *middleware.Stack) error {
		return stack.Initialize.Add(tracer, middleware.Before)
	},
		func(stack *middleware.Stack) error {
			return stack.Initialize.Add(metadata, middleware.After)
		})

	return awsConfig
}
