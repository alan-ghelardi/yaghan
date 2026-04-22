package secretsmanager

import (
	"context"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
)

// Config configures the Secrets Manager client.
type Config struct {
	Endpoint string
}

// Client declares the signature of relevant methods implemented by
// secretsmanager.Client.
type Client interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	BatchGetSecretValue(ctx context.Context, params *secretsmanager.BatchGetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.BatchGetSecretValueOutput, error)
}

type staticEndpointResolver struct {
	uri url.URL
}

var _ secretsmanager.EndpointResolverV2 = (*staticEndpointResolver)(nil)

// ResolveEndpoint implements secretsmanager.EndpointResolverV2.
func (s *staticEndpointResolver) ResolveEndpoint(_ context.Context, _ secretsmanager.EndpointParameters) (smithyendpoints.Endpoint, error) {
	return smithyendpoints.Endpoint{URI: s.uri}, nil
}

// key allows us to attach Client objects to the Context.
type key struct {
}

// Get returns the Client object attached to the provided Context or panics if
// no one can be found.
func Get(ctx context.Context) Client {
	client, ok := ctx.Value(key{}).(Client)
	if !ok {
		logger := ctxzap.Extract(ctx)
		logger.Panic("no Secrets Manager client could be found in the provided Context")
	}
	return client
}

// With returns a copy of the provided Context with the Client object in
// question attached to it.
func With(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, key{}, client)
}

// New returns a new Client to talk to Secrets Manager.
func New(ctx context.Context, config Config) Client {
	awsConfig := awsconfig.Get(ctx)
	options := []func(*secretsmanager.Options){}

	if len(config.Endpoint) != 0 {
		uri, err := url.Parse(config.Endpoint)
		if err != nil {
			ctxzap.Extract(ctx).Panic("invalid secretsmanager endpoint", zap.Error(err))
		}
		options = append(options, secretsmanager.WithEndpointResolverV2(&staticEndpointResolver{uri: *uri}))
	}

	return secretsmanager.NewFromConfig(awsConfig, options...)
}
