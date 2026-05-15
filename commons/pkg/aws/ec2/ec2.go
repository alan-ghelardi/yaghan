package ec2

import (
	"context"
	"net/url"

	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
)

// Config configures the EC2 client.
type Config struct {
	Endpoint string
}

// Client declares the signature of relevant methods implemented by
// ec2.Client.
type Client interface {
	DescribeInstanceStatus(ctx context.Context, params *ec2.DescribeInstanceStatusInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceStatusOutput, error)
}

type staticEndpointResolver struct {
	uri url.URL
}

var _ ec2.EndpointResolverV2 = (*staticEndpointResolver)(nil)

// ResolveEndpoint implements ec2.EndpointResolverV2.
func (s *staticEndpointResolver) ResolveEndpoint(context.Context, ec2.EndpointParameters) (smithyendpoints.Endpoint, error) {
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
		logger.Panic("no ec2 client could be found in the provided Context")
	}
	return client
}

// With returns a copy of the provided Context with the Client object in
// question attached to it.
func With(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, key{}, client)
}

// New returns a new Client to talk to EC2.
func New(ctx context.Context, config Config) Client {
	awsConfig := awsconfig.Get(ctx)
	options := []func(*ec2.Options){}

	if len(config.Endpoint) != 0 {
		options = append(options, func(opts *ec2.Options) {
			opts.BaseEndpoint = aws.String(config.Endpoint)
		})
	}

	return ec2.NewFromConfig(awsConfig, options...)
}
