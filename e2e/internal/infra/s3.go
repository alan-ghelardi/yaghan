package infra

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// EnsureBucket creates the named bucket if it does not yet exist.
// Mirrors the head-bucket-then-create dance in daemon/dev/start.sh.
func EnsureBucket(ctx context.Context, client *s3.Client, name string) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)})
	if err == nil {
		return nil
	}
	var notFound *types.NotFound
	if !errors.As(err, &notFound) {
		return fmt.Errorf("head bucket %s: %w", name, err)
	}
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)}); err != nil {
		return fmt.Errorf("create bucket %s: %w", name, err)
	}
	return nil
}
