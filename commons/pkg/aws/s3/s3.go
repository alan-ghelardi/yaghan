package s3

import (
	"context"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
)

// Config configures the S3 client.
type Config struct {
	Endpoint     string
	UsePathStyle bool
	Region       string
}

// Client declares the signature of relevant methods implemented by
// s3.Client.
type Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

type staticEndpointResolver struct {
	uri url.URL
}

var _ s3.EndpointResolverV2 = (*staticEndpointResolver)(nil)

// ResolveEndpoint implements s3.EndpointResolverV2.
func (s *staticEndpointResolver) ResolveEndpoint(context.Context, s3.EndpointParameters) (smithyendpoints.Endpoint, error) {
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
		logger.Panic("no s3 client could be found in the provided Context")
	}
	return client
}

// With returns a copy of the provided Context with the Client object in
// question attached to it.
func With(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, key{}, client)
}

// New returns a new Client to talk to S3.
func New(ctx context.Context, config Config) Client {
	awsConfig := awsconfig.Get(ctx)
	options := []func(*s3.Options){}

	if len(config.Endpoint) != 0 {
		options = append(options, func(opts *s3.Options) {
			opts.BaseEndpoint = aws.String(config.Endpoint)
		})
	}

	if config.UsePathStyle {
		options = append(options, func(opts *s3.Options) {
			opts.UsePathStyle = true
		})
	}

	if config.Region != "" {
		options = append(options, func(opts *s3.Options) {
			opts.Region = config.Region
		})
	}

	return s3.NewFromConfig(awsConfig, options...)
}
