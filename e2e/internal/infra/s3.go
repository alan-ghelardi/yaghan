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

// EmptyBucket deletes every object in the bucket. Used in suite
// teardown so snapshot artefacts from one run don't pile up across
// runs of the suite against the same MinIO container.
func EmptyBucket(ctx context.Context, client *s3.Client, name string) error {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(name),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			var noBucket *types.NoSuchBucket
			if errors.As(err, &noBucket) {
				return nil
			}
			return fmt.Errorf("list objects in %s: %w", name, err)
		}
		if len(page.Contents) == 0 {
			continue
		}
		ids := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
		}
		_, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(name),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("delete objects in %s: %w", name, err)
		}
	}
	return nil
}
