package ec2imds

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
)

// Config configures the EC2 instance metadata service client.
type Config struct {
	Endpoint string
}

// Client declares the signature of relevant methods implemented by
// ec2imds.Client.
type Client interface {
	GetInstanceIdentityDocument(ctx context.Context, params *imds.GetInstanceIdentityDocumentInput, optFns ...func(*imds.Options)) (*imds.GetInstanceIdentityDocumentOutput, error)
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
		logger.Panic("no EC2 instance metadata service client could be found in the provided Context")
	}
	return client
}

// With returns a copy of the provided Context with the Client object in
// question attached to it.
func With(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, key{}, client)
}

// New returns a new Client to talk to EC2 instance metadata API.
func New(ctx context.Context, config Config) Client {
	awsConfig := awsconfig.Get(ctx)
	options := []func(*imds.Options){
		func(opts *imds.Options) {
			opts.Endpoint = config.Endpoint
		},
	}
	return imds.NewFromConfig(awsConfig, options...)
}
