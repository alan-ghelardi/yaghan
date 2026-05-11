package testing

import (
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3service "github.com/aws/aws-sdk-go-v2/service/s3"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/s3"
)

func CreateBucket(t *testing.T, endpoint, bucketName string) {
	t.Helper()

	ctx := t.Context()
	config := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, config)
	s3Client := s3.New(ctx, s3.Config{Endpoint: endpoint, UsePathStyle: true})

	_, err := s3Client.CreateBucket(ctx, &s3service.CreateBucketInput{Bucket: aws.String(bucketName)})
	if err != nil {
		t.Fatal(err)
	}
}

func PutObject(t *testing.T, endpoint, bucketName, key string, content io.Reader) {
	t.Helper()

	ctx := t.Context()
	config := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, config)
	s3Client := s3.New(ctx, s3.Config{Endpoint: endpoint, UsePathStyle: true})

	_, err := s3Client.PutObject(ctx, &s3service.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   content,
	})
	if err != nil {
		t.Fatal(err)
	}
}
