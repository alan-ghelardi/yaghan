package s3

import (
	"context"
	"io"
	"net/url"

	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
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

// Uploader is the surface of *manager.Uploader used by callers that need
// concurrent multipart uploads (large memory snapshots, image layers, …).
// The interface mirrors *manager.Uploader's Upload signature so tests can
// substitute a fake without depending on the AWS SDK manager directly.
type Uploader interface {
	Upload(ctx context.Context, input *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

// Downloader is the surface of *manager.Downloader used by callers that
// need concurrent multipart downloads into a destination implementing
// io.WriterAt.
type Downloader interface {
	Download(ctx context.Context, w io.WriterAt, input *s3.GetObjectInput, opts ...func(*manager.Downloader)) (int64, error)
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
	return newSDKClient(ctx, config)
}

// NewUploader returns an Uploader backed by AWS SDK's manager.Uploader.
// Multipart uploads are concurrent by default; tune via the opts argument
// on Upload (e.g. PartSize, Concurrency).
func NewUploader(ctx context.Context, config Config) Uploader {
	return manager.NewUploader(newSDKClient(ctx, config))
}

// NewDownloader returns a Downloader backed by AWS SDK's manager.Downloader.
// Downloads write parts in parallel into the provided io.WriterAt.
func NewDownloader(ctx context.Context, config Config) Downloader {
	return manager.NewDownloader(newSDKClient(ctx, config))
}

// newSDKClient builds a concrete *s3.Client from the AWS config in ctx and
// the supplied Config. Shared by New, NewUploader, and NewDownloader so all
// three honor the same endpoint, path-style, and region overrides.
func newSDKClient(ctx context.Context, config Config) *s3.Client {
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

	if len(config.Region) != 0 {
		options = append(options, func(opts *s3.Options) {
			opts.Region = config.Region
		})
	}

	return s3.NewFromConfig(awsConfig, options...)
}
