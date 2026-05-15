package infra

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Credentials carries the access key, secret, and region the e2e
// suite uses to talk to a local AWS-API-compatible service (DynamoDB
// Local or MinIO). The values match those exported by the dev/start.sh
// scripts so the binaries see the same backing store the suite
// provisions.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// DynamoDBLocalCreds are what api-server/dev/start.sh exports when
// driving DynamoDB Local. The values are dummies — DynamoDB Local
// scopes data by (creds, region) but accepts any non-empty value.
var DynamoDBLocalCreds = Credentials{
	AccessKeyID:     "local",
	SecretAccessKey: "local",
	Region:          "us-east-1",
}

// MinIOCreds match docker-compose.yml's MINIO_ROOT_USER /
// MINIO_ROOT_PASSWORD and daemon/dev/start.sh.
var MinIOCreds = Credentials{
	AccessKeyID:     "minioadmin",
	SecretAccessKey: "minioadmin",
	Region:          "us-east-1",
}

func loadAWSConfig(ctx context.Context, creds Credentials) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithRegion(creds.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID, creds.SecretAccessKey, "")),
	)
}

// NewDynamoDBClient returns a DynamoDB client wired to the given
// endpoint (e.g. http://localhost:8000 for DynamoDB Local).
func NewDynamoDBClient(ctx context.Context, endpoint string, creds Credentials) (*dynamodb.Client, error) {
	cfg, err := loadAWSConfig(ctx, creds)
	if err != nil {
		return nil, err
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	}), nil
}

// NewS3Client returns an S3 client wired to the given endpoint
// (e.g. http://localhost:9000 for MinIO). UsePathStyle is required for
// MinIO since it doesn't serve virtual-hosted bucket URLs.
func NewS3Client(ctx context.Context, endpoint string, creds Credentials) (*s3.Client, error) {
	cfg, err := loadAWSConfig(ctx, creds)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}
