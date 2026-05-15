package snapshot

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"strings"
	"testing"

	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	s3 "github.com/alan-ghelardi/yaghan/commons/pkg/aws/s3"
	awstesting "github.com/alan-ghelardi/yaghan/commons/pkg/aws/testing"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s3Fixture bundles everything an s3Store test needs: the store under test,
// a raw client for assertions and one-off PutObject calls, plus the
// shared ctx/bucket so cases don't reconstruct them.
type s3Fixture struct {
	store      *s3Store
	rawClient  s3.Client
	endpoint   string
	bucketName string
	ctx        context.Context
}

// newS3Fixture spins up a fresh kumo emulator with a unique bucket per test
// and returns an s3Store wired to it. The emulator and bucket are torn down
// via t.Cleanup.
func newS3Fixture(t *testing.T) *s3Fixture {
	t.Helper()

	endpoint := awstesting.StartEmulator(t)
	bucketName := "snap-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	awstesting.CreateBucket(t, endpoint, bucketName)

	ctx := t.Context()
	cfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, cfg)

	s3Cfg := s3.Config{Endpoint: endpoint, UsePathStyle: true}
	uploader := s3.NewUploader(ctx, s3Cfg)
	downloader := s3.NewDownloader(ctx, s3Cfg)
	rawClient := s3.New(ctx, s3Cfg)

	store := &s3Store{
		uploader:   uploader,
		downloader: downloader,
		bucketName: bucketName,
	}
	return &s3Fixture{
		store:      store,
		rawClient:  rawClient,
		endpoint:   endpoint,
		bucketName: bucketName,
		ctx:        ctx,
	}
}

// readObject fetches an S3 object's bytes; used by Put tests to assert
// against expected payloads.
func (f *s3Fixture) readObject(t *testing.T, key string) ([]byte, error) {
	t.Helper()
	out, err := f.rawClient.GetObject(f.ctx, &s3sdk.GetObjectInput{
		Bucket: aws.String(f.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func TestS3Store_PutGetRoundTrip(t *testing.T) {
	fx := newS3Fixture(t)
	snapshotID := uuid.NewString()

	memPayload := []byte("memory contents — μ")
	statePayload := []byte("state contents — σ")

	require.NoError(t, fx.store.Put(fx.ctx, snapshotID, &ContentsReader{
		Memory: bytes.NewReader(memPayload),
		State:  bytes.NewReader(statePayload),
	}))

	// Inspect the bucket directly to verify the object keys we chose.
	gotMem, err := fx.readObject(t, memObjectKey(snapshotID))
	require.NoError(t, err)
	assert.Equal(t, memPayload, gotMem)

	gotState, err := fx.readObject(t, stateObjectKey(snapshotID))
	require.NoError(t, err)
	assert.Equal(t, statePayload, gotState)

	// Round-trip via Get and verify both halves come back identical.
	var memBuf, stateBuf manager.WriteAtBuffer
	require.NoError(t, fx.store.Get(fx.ctx, snapshotID, &ContentsWriter{
		Memory: &memBuf,
		State:  &stateBuf,
	}))
	assert.Equal(t, memPayload, memBuf.Bytes())
	assert.Equal(t, statePayload, stateBuf.Bytes())
}

func TestS3Store_GetMissingSnapshot(t *testing.T) {
	fx := newS3Fixture(t)

	var memBuf, stateBuf manager.WriteAtBuffer
	err := fx.store.Get(fx.ctx, uuid.NewString(), &ContentsWriter{
		Memory: &memBuf,
		State:  &stateBuf,
	})
	assert.ErrorIs(t, err, ErrSnapshotNotFound)
}

func TestS3Store_GetPartiallyMissingSnapshot(t *testing.T) {
	fx := newS3Fixture(t)
	snapshotID := uuid.NewString()

	// Stage only the mem half — state is missing.
	awstesting.PutObject(t, fx.endpoint, fx.bucketName, memObjectKey(snapshotID), bytes.NewReader([]byte("partial")))

	var memBuf, stateBuf manager.WriteAtBuffer
	err := fx.store.Get(fx.ctx, snapshotID, &ContentsWriter{
		Memory: &memBuf,
		State:  &stateBuf,
	})
	assert.ErrorIs(t, err, ErrSnapshotNotFound)
}

func TestS3Store_PutLargePayloadTriggersMultipart(t *testing.T) {
	fx := newS3Fixture(t)
	snapshotID := uuid.NewString()

	// 6 MiB exceeds the 5 MiB minimum part size — manager.Uploader
	// chunks it into at least two parts. We verify bytes via raw
	// GetObject; manager.Downloader's parallel ranged GETs aren't
	// fully supported by the in-process emulator and are already
	// covered by the small-body round-trip test.
	memPayload := make([]byte, 6*1024*1024)
	_, err := rand.Read(memPayload)
	require.NoError(t, err)
	statePayload := []byte("small")

	require.NoError(t, fx.store.Put(fx.ctx, snapshotID, &ContentsReader{
		Memory: bytes.NewReader(memPayload),
		State:  bytes.NewReader(statePayload),
	}))

	gotMem, err := fx.readObject(t, memObjectKey(snapshotID))
	require.NoError(t, err)
	assert.Equal(t, memPayload, gotMem)

	gotState, err := fx.readObject(t, stateObjectKey(snapshotID))
	require.NoError(t, err)
	assert.Equal(t, statePayload, gotState)
}
