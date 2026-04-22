package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
)

// Config configures the Dynamo DB client.
type Config struct {
	Endpoint string
}

// Client declares the signature of relevant methods implemented by
// dynamodb.Client.
type Client interface {
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DeleteTable(ctx context.Context, params *dynamodb.DeleteTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

type staticEndpointResolver struct {
	uri url.URL
}

var _ dynamodb.EndpointResolverV2 = (*staticEndpointResolver)(nil)

// ResolveEndpoint implements dynamodb.EndpointResolverV2.
func (s *staticEndpointResolver) ResolveEndpoint(context.Context, dynamodb.EndpointParameters) (smithyendpoints.Endpoint, error) {
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
		logger.Panic("no DynamoDB client could be found in the provided Context")
	}
	return client
}

// With returns a copy of the provided Context with the Client object in
// question attached to it.
func With(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, key{}, client)
}

// New returns a new Client to talk to Dynamo DB.
func New(ctx context.Context, config Config) Client {
	awsConfig := awsconfig.Get(ctx)
	options := []func(*dynamodb.Options){}

	if len(config.Endpoint) != 0 {
		uri, err := url.Parse(config.Endpoint)
		if err != nil {
			ctxzap.Extract(ctx).Panic("invalid dynamodb endpoint", zap.Error(err))
		}
		options = append(options, dynamodb.WithEndpointResolverV2(&staticEndpointResolver{uri: *uri}))
	}

	return dynamodb.NewFromConfig(awsConfig, options...)
}

func CreateTableInputFromFile(base string, schemaFile string) (*dynamodb.CreateTableInput, error) {
	schemaPath, err := filepath.Abs(filepath.Join(base, schemaFile))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve DynamoDB schema file: %w", err)
	}

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, err
	}

	createTableInput := &dynamodb.CreateTableInput{}
	if err := json.Unmarshal(data, createTableInput); err != nil {
		return nil, fmt.Errorf("error parsing schema at %s: %w", schemaPath, err)
	}
	return createTableInput, nil
}
