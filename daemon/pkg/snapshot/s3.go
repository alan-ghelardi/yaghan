package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	s3 "github.com/alan-ghelardi/yaghan/commons/pkg/aws/s3"
	"github.com/aws/aws-sdk-go-v2/aws"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"
)

type s3Store struct {
	uploader   s3.Uploader
	downloader s3.Downloader
	bucketName string
}

var _ DurableStore = (*s3Store)(nil)

// NewS3DurableStore returns a DurableStore backed by Amazon S3. The
// uploader and downloader should be constructed via s3.NewUploader /
// s3.NewDownloader so the underlying manager is reused across calls.
func NewS3DurableStore(uploader s3.Uploader, downloader s3.Downloader, bucketName string) DurableStore {
	return &s3Store{
		uploader:   uploader,
		downloader: downloader,
		bucketName: bucketName,
	}
}

// Put implements [DurableStore]. Mem and state are uploaded concurrently;
// if either upload fails, the errgroup cancels the other and the first
// error is returned.
func (s *s3Store) Put(ctx context.Context, snapshotID string, contents *ContentsReader) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.uploadObject(ctx, memObjectKey(snapshotID), contents.Memory)
	})
	g.Go(func() error {
		return s.uploadObject(ctx, stateObjectKey(snapshotID), contents.State)
	})
	return g.Wait()
}

// Get implements [DurableStore]. Mem and state are downloaded concurrently
// into contents.Memory and contents.State respectively. A missing object
// (NoSuchKey) or missing bucket (NoSuchBucket) maps to ErrSnapshotNotFound.
func (s *s3Store) Get(ctx context.Context, snapshotID string, contents *ContentsWriter) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.downloadObject(ctx, memObjectKey(snapshotID), contents.Memory)
	})
	g.Go(func() error {
		return s.downloadObject(ctx, stateObjectKey(snapshotID), contents.State)
	})
	if err := g.Wait(); err != nil {
		if isNotFound(err) {
			return ErrSnapshotNotFound
		}
		return err
	}
	return nil
}

func (s *s3Store) uploadObject(ctx context.Context, key string, body io.Reader) error {
	_, err := s.uploader.Upload(ctx, &s3sdk.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

func (s *s3Store) downloadObject(ctx context.Context, key string, w io.WriterAt) error {
	_, err := s.downloader.Download(ctx, w, &s3sdk.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("download %s: %w", key, err)
	}
	return nil
}

func memObjectKey(snapshotID string) string {
	return path.Join(snapshotsDir, snapshotID, memSnapFile)
}

func stateObjectKey(snapshotID string) string {
	return path.Join(snapshotsDir, snapshotID, stateSnapFile)
}

// isNotFound reports whether err is an S3 "object missing" or "bucket
// missing" condition. We also accept the legacy generic NotFound APIErrorCode
// some emulators return instead of the typed NoSuchKey/NoSuchBucket errors.
func isNotFound(err error) bool {
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nsb *s3types.NoSuchBucket
	if errors.As(err, &nsb) {
		return true
	}
	var apiErr interface{ ErrorCode() string }
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "NoSuchKey" || code == "NoSuchBucket" || code == "NotFound"
	}
	return false
}
