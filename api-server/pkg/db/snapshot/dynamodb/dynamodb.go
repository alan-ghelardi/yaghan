package dynamodb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.nuinfra.api-server/pkg/config"
	"golang.nuinfra.api-server/pkg/db"
	snapshotdb "golang.nuinfra.api-server/pkg/db/snapshot"
	control_planev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Top-level item attribute names. Kept in sync with
// api-server/dynamodb-tables/snapshots.json.
const (
	attrSnapshotID            = "snapshot_id"
	attrSnapshotNamespace     = "snapshot_namespace"
	attrSandboxRefID          = "sandbox_ref_id"
	attrGSICtSK               = "gsi_ct_sk"
	attrSnapshotPB            = "snapshot_pb"
	attrSnapshotContentDigest = "snapshot_content_digest"
)

type dynamoDB struct {
	client    dynamodb.Client
	tableName string
}

var _ snapshotdb.DB = (*dynamoDB)(nil)

// New returns a [snapshot.DB] instance backed by AWS DynamoDB.
func New(ctx context.Context, config *config.Bundle) snapshotdb.DB {
	return &dynamoDB{
		client:    dynamodb.Get(ctx),
		tableName: config.Database.AWS.SnapshotsTableName,
	}
}

// Create implements [snapshot.DB].
//
// Wall clock: the DB owns Metadata.CreatedAt. It is stamped with the
// server's current time, overwriting whatever the caller supplied. The
// digest computation zeroes it, so idempotency is unaffected.
//
// Idempotency: the input is canonicalised (CreatedAt zeroed) and
// hashed; the resulting digest is stored alongside the row. On a
// conditional-check failure, the stored digest is compared to the
// input's; if they match, the call returns nil (the existing row is
// preserved). If they differ, [db.ErrAlreadyExists] is returned.
func (d *dynamoDB) Create(ctx context.Context, sn *control_planev1alpha1.Snapshot) error {
	id := sn.GetMetadata().GetId()

	sn.Metadata.CreatedAt = timestamppb.Now()

	digest, err := contentDigest(sn)
	if err != nil {
		return fmt.Errorf("snapshot %q: build digest: %w", id, err)
	}

	item, err := toItem(sn, digest)
	if err != nil {
		return fmt.Errorf("snapshot %q: build item: %w", id, err)
	}

	_, err = d.client.PutItem(ctx, &dynamodbservice.PutItemInput{
		TableName:                           aws.String(d.tableName),
		Item:                                item,
		ConditionExpression:                 aws.String("attribute_not_exists(" + attrSnapshotID + ")"),
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	if err == nil {
		return nil
	}

	var cce *types.ConditionalCheckFailedException
	if !errors.As(err, &cce) {
		return fmt.Errorf("snapshot %q: put item: %w", id, err)
	}

	stored, err := d.fetchStoredItem(ctx, cce, id)
	if err != nil {
		return fmt.Errorf("snapshot %q: read stored row after conflict: %w", id, err)
	}
	storedDigest, ok := digestFromAttrs(stored)
	if !ok {
		return fmt.Errorf("snapshot %q: stored row has no content digest", id)
	}

	if bytes.Equal(digest, storedDigest) {
		return nil
	}
	return fmt.Errorf("snapshot %q: %w", id, db.ErrAlreadyExists)
}

// fetchStoredItem returns the prior item for the given snapshot id following a
// conditional-check failure. When the exception carries the prior item (the
// expected fast path) it is returned directly; otherwise — some emulators do
// not implement ReturnValuesOnConditionCheckFailure — it falls back to a
// strongly consistent GetItem. An empty map is returned if the row genuinely
// does not exist.
func (d *dynamoDB) fetchStoredItem(ctx context.Context, cce *types.ConditionalCheckFailedException, id string) (map[string]types.AttributeValue, error) {
	if len(cce.Item) > 0 {
		return cce.Item, nil
	}

	ctxzap.Extract(ctx).Warn(
		"conditional-check failure did not include the prior item; falling back to GetItem",
		zap.String("snapshot_id", id),
	)

	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrSnapshotID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return out.Item, nil
}

// Get implements [snapshot.DB].
//
// Reads are strongly consistent so that callers observe their own writes,
// including writes made by Create on the same node.
func (d *dynamoDB) Get(ctx context.Context, id string) (*control_planev1alpha1.Snapshot, error) {
	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrSnapshotID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot %q: get item: %w", id, err)
	}
	if len(out.Item) == 0 {
		return nil, fmt.Errorf("snapshot %q: %w", id, db.ErrNotFound)
	}
	return snapshotFromAttrs(out.Item, id)
}

// Delete implements [snapshot.DB].
//
// DynamoDB's DeleteItem is naturally idempotent: deleting an absent key
// is not an error. We do not issue a condition expression — the
// contract is "row no longer exists after this call returns", which is
// already true when the row was absent to begin with.
func (d *dynamoDB) Delete(ctx context.Context, id string) error {
	_, err := d.client.DeleteItem(ctx, &dynamodbservice.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key:       map[string]types.AttributeValue{attrSnapshotID: &types.AttributeValueMemberS{Value: id}},
	})
	if err != nil {
		return fmt.Errorf("snapshot %q: delete item: %w", id, err)
	}
	return nil
}

func snapshotFromAttrs(attrs map[string]types.AttributeValue, id string) (*control_planev1alpha1.Snapshot, error) {
	av, ok := attrs[attrSnapshotPB]
	if !ok {
		return nil, fmt.Errorf("snapshot %q: stored row has no %s attribute", id, attrSnapshotPB)
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, fmt.Errorf("snapshot %q: stored %s is not bytes", id, attrSnapshotPB)
	}
	sn := &control_planev1alpha1.Snapshot{}
	if err := proto.Unmarshal(b.Value, sn); err != nil {
		return nil, fmt.Errorf("snapshot %q: unmarshal stored row: %w", id, err)
	}
	return sn, nil
}

// stringFromAttrs returns the value of a string-typed attribute as a Go
// string. Kept around for parity with the other entity packages, but
// snapshots have no numeric columns to also accept.
func stringFromAttrs(attrs map[string]types.AttributeValue, name string) (string, bool) {
	av, ok := attrs[name]
	if !ok {
		return "", false
	}
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return s.Value, true
}

// toItem builds the DynamoDB item map for a Snapshot. The caller supplies the
// content digest so it is computed at most once per write.
func toItem(sn *control_planev1alpha1.Snapshot, digest []byte) (map[string]types.AttributeValue, error) {
	pb, err := proto.Marshal(sn)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	meta := sn.GetMetadata()
	return map[string]types.AttributeValue{
		attrSnapshotID:            &types.AttributeValueMemberS{Value: meta.GetId()},
		attrSnapshotNamespace:     &types.AttributeValueMemberS{Value: meta.GetNamespace()},
		attrSandboxRefID:          &types.AttributeValueMemberS{Value: sn.GetSandbox().GetId()},
		attrGSICtSK:               &types.AttributeValueMemberS{Value: gsiCtSK(sn)},
		attrSnapshotPB:            &types.AttributeValueMemberB{Value: pb},
		attrSnapshotContentDigest: &types.AttributeValueMemberB{Value: digest},
	}, nil
}

// gsiCtSK builds the shared sort key used by both GSIs:
// "{created_at_rfc3339_nano_utc}#{id}". CreatedAt is DB-stamped on every
// Create, so this attribute is always present and orders rows
// monotonically within a partition.
func gsiCtSK(sn *control_planev1alpha1.Snapshot) string {
	meta := sn.GetMetadata()
	return meta.GetCreatedAt().AsTime().UTC().Format(time.RFC3339Nano) + "#" + meta.GetId()
}

// contentDigest returns the SHA-256 of a deterministic, canonicalised proto
// encoding of the snapshot. Server-managed fields are zeroed so that two
// retries of the same logical mutation produce the same digest — even
// when one of them has already been applied. The stripped field is:
//
//   - Metadata.CreatedAt: stamped by the DB on every successful Create.
//
// Determinism is required because Go's default proto marshalling does
// not guarantee stable map ordering.
func contentDigest(sn *control_planev1alpha1.Snapshot) ([]byte, error) {
	canonical := proto.CloneOf(sn)
	if meta := canonical.GetMetadata(); meta != nil {
		meta.CreatedAt = nil
	}

	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical snapshot: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// digestFromAttrs extracts the content digest attribute from a DynamoDB item.
func digestFromAttrs(attrs map[string]types.AttributeValue) ([]byte, bool) {
	if attrs == nil {
		return nil, false
	}
	av, ok := attrs[attrSnapshotContentDigest]
	if !ok {
		return nil, false
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return b.Value, true
}
