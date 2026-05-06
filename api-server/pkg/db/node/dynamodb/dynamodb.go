package dynamodb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.nuinfra.api-server/pkg/config"
	"golang.nuinfra.api-server/pkg/db"
	nodedb "golang.nuinfra.api-server/pkg/db/node"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Top-level item attribute names. Kept in sync with
// api-server/dynamodb-tables/nodes.json.
const (
	attrNodeID            = "node_id"
	attrNodeStatusPhase   = "node_status_phase"
	attrGSIConstHK        = "gsi_const_hk"
	attrGSILmtSK          = "gsi_lmt_sk"
	attrNodeVersion       = "node_version"
	attrNodePB            = "node_pb"
	attrNodeContentDigest = "node_content_digest"
)

// allNodesPartition is the constant value written to gsi_const_hk on every
// row, so the all_nodes_index is keyed off a single partition. This is an
// intentional hot partition: cluster sizes are bounded (a few thousand
// nodes max, never millions) so the trade-off is acceptable in exchange
// for a simple "list every node" query.
const allNodesPartition = "node"

// initialVersion is the Metadata.Version stamped on every freshly-created
// node. The DB owns the version field: callers pass a zeroed Version on
// the first Put, the DB writes initialVersion, and subsequent Puts bump
// to expected+1.
const initialVersion int64 = 1

type dynamoDB struct {
	client    dynamodb.Client
	tableName string
}

var _ nodedb.DB = (*dynamoDB)(nil)

// New returns a [node.DB] instance backed by AWS DynamoDB.
func New(ctx context.Context, config *config.Bundle) nodedb.DB {
	return &dynamoDB{
		client:    dynamodb.Get(ctx),
		tableName: config.Database.AWS.NodesTableName,
	}
}

// Put implements [node.DB].
//
// Versioning: the DB owns Metadata.Version. The caller's Version
// disambiguates create from update — zero means "create new row", any
// positive value means "update the row that was at this version". The
// input proto is mutated in place: on success Metadata.Version reflects
// the stored value (initialVersion on create, expected+1 on update),
// Metadata.CreatedAt is stamped on create only, and
// Metadata.LastModifiedAt is stamped on every successful Put.
//
// Idempotency: the input is canonicalised (server-set wall-clock fields
// and version zeroed) and hashed; the digest is stored alongside the
// row. On a conditional-check failure, the stored digest is compared to
// the input's; if they match the call returns nil (the existing row is
// preserved). Otherwise:
//
//   - Create branch (Version == 0): returns [db.ErrAlreadyExists] when
//     a different body already lives under the same id.
//   - Update branch (Version > 0): returns [db.ErrNotFound] when no row
//     exists, or [db.ErrVersionConflict] when the stored version has
//     advanced past the caller's expected value.
func (d *dynamoDB) Put(ctx context.Context, node *cpv1.Node) error {
	id := node.GetMetadata().GetId()
	expectedVersion := node.GetMetadata().GetVersion()

	now := timestamppb.Now()
	if expectedVersion == 0 {
		node.Metadata.CreatedAt = now
	}
	node.Metadata.LastModifiedAt = now

	// contentDigest strips Version and wall-clock fields, so the digest
	// is identical before and after the bump and across retries.
	digest, err := contentDigest(node)
	if err != nil {
		return fmt.Errorf("node %q: build digest: %w", id, err)
	}

	if expectedVersion == 0 {
		node.Metadata.Version = initialVersion
	} else {
		node.Metadata.Version = expectedVersion + 1
	}

	item, err := toItem(node, digest)
	if err != nil {
		return fmt.Errorf("node %q: build item: %w", id, err)
	}

	cond, values := putCondition(expectedVersion)

	_, err = d.client.PutItem(ctx, &dynamodbservice.PutItemInput{
		TableName:                           aws.String(d.tableName),
		Item:                                item,
		ConditionExpression:                 aws.String(cond),
		ExpressionAttributeValues:           values,
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	if err == nil {
		return nil
	}

	var cce *types.ConditionalCheckFailedException
	if !errors.As(err, &cce) {
		return fmt.Errorf("node %q: put item: %w", id, err)
	}

	stored, err := d.fetchStoredItem(ctx, cce, id)
	if err != nil {
		return fmt.Errorf("node %q: read stored row after conflict: %w", id, err)
	}

	if storedDigest, ok := digestFromAttrs(stored); ok && bytes.Equal(digest, storedDigest) {
		return nil
	}

	return diagnosePutConflict(id, expectedVersion, stored)
}

// putCondition builds the conditional expression and attribute values
// for a Put. expectedVersion == 0 selects the create branch (the row
// must NOT exist); any positive value selects the update branch (the
// row must exist and its node_version must match).
func putCondition(expectedVersion int64) (string, map[string]types.AttributeValue) {
	if expectedVersion == 0 {
		return "attribute_not_exists(" + attrNodeID + ")", nil
	}
	return "attribute_exists(" + attrNodeID + ") AND " + attrNodeVersion + " = :ev",
		map[string]types.AttributeValue{
			":ev": &types.AttributeValueMemberN{Value: strconv.FormatInt(expectedVersion, 10)},
		}
}

// diagnosePutConflict turns a conditional-check failure into the most
// specific sentinel error the prior-item attributes support. On the
// create branch (expectedVersion == 0) the only failure mode is "a
// different body already lives under the same id". On the update branch
// it can be either "no such row" or "version mismatch".
func diagnosePutConflict(id string, expectedVersion int64, stored map[string]types.AttributeValue) error {
	if expectedVersion == 0 {
		return fmt.Errorf("node %q: %w", id, db.ErrAlreadyExists)
	}
	if len(stored) == 0 {
		return fmt.Errorf("node %q: %w", id, db.ErrNotFound)
	}
	if storedVersion, ok := stringFromAttrs(stored, attrNodeVersion); ok {
		return fmt.Errorf(
			"node %q: expected version %d but stored version is %s: %w",
			id, expectedVersion, storedVersion, db.ErrVersionConflict,
		)
	}
	return fmt.Errorf("node %q: %w", id, db.ErrVersionConflict)
}

// fetchStoredItem returns the prior item for the given node id following
// a conditional-check failure. When the exception carries the prior
// item (the expected fast path) it is returned directly; otherwise it
// falls back to a strongly consistent GetItem. An empty map is returned
// when the row genuinely does not exist.
func (d *dynamoDB) fetchStoredItem(ctx context.Context, cce *types.ConditionalCheckFailedException, id string) (map[string]types.AttributeValue, error) {
	if len(cce.Item) > 0 {
		return cce.Item, nil
	}

	ctxzap.Extract(ctx).Warn(
		"conditional-check failure did not include the prior item; falling back to GetItem",
		zap.String("node_id", id),
	)

	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrNodeID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return out.Item, nil
}

// Get implements [node.DB].
//
// Reads are strongly consistent so callers observe their own writes,
// including writes made by Put on the same api-server instance.
func (d *dynamoDB) Get(ctx context.Context, id string) (*cpv1.Node, error) {
	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrNodeID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("node %q: get item: %w", id, err)
	}
	if len(out.Item) == 0 {
		return nil, fmt.Errorf("node %q: %w", id, db.ErrNotFound)
	}
	return nodeFromAttrs(out.Item, id)
}

// toItem builds the DynamoDB item map for a Node. The caller supplies
// the content digest so it is computed at most once per write.
func toItem(node *cpv1.Node, digest []byte) (map[string]types.AttributeValue, error) {
	pb, err := proto.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("marshal node: %w", err)
	}

	meta := node.GetMetadata()
	return map[string]types.AttributeValue{
		attrNodeID:            &types.AttributeValueMemberS{Value: meta.GetId()},
		attrNodeStatusPhase:   &types.AttributeValueMemberS{Value: node.GetStatus().GetPhase().String()},
		attrGSIConstHK:        &types.AttributeValueMemberS{Value: allNodesPartition},
		attrGSILmtSK:          &types.AttributeValueMemberS{Value: gsiLmtSK(node)},
		attrNodeVersion:       &types.AttributeValueMemberN{Value: strconv.FormatInt(meta.GetVersion(), 10)},
		attrNodePB:            &types.AttributeValueMemberB{Value: pb},
		attrNodeContentDigest: &types.AttributeValueMemberB{Value: digest},
	}, nil
}

// gsiLmtSK builds the shared sort key used by both GSIs:
// "{last_modified_at_rfc3339_nano_utc}#{id}". LastModifiedAt is
// DB-stamped on every successful Put, so this attribute is always
// present and orders rows monotonically within a partition.
func gsiLmtSK(node *cpv1.Node) string {
	meta := node.GetMetadata()
	return meta.GetLastModifiedAt().AsTime().UTC().Format(time.RFC3339Nano) + "#" + meta.GetId()
}

func nodeFromAttrs(attrs map[string]types.AttributeValue, id string) (*cpv1.Node, error) {
	av, ok := attrs[attrNodePB]
	if !ok {
		return nil, fmt.Errorf("node %q: stored row has no %s attribute", id, attrNodePB)
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, fmt.Errorf("node %q: stored %s is not bytes", id, attrNodePB)
	}
	n := &cpv1.Node{}
	if err := proto.Unmarshal(b.Value, n); err != nil {
		return nil, fmt.Errorf("node %q: unmarshal stored row: %w", id, err)
	}
	return n, nil
}

// stringFromAttrs returns the value of a string- or number-typed
// attribute as a Go string. Number attributes are encoded as strings
// in the AWS SDK.
func stringFromAttrs(attrs map[string]types.AttributeValue, name string) (string, bool) {
	av, ok := attrs[name]
	if !ok {
		return "", false
	}
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value, true
	case *types.AttributeValueMemberN:
		return v.Value, true
	default:
		return "", false
	}
}

// contentDigest returns the SHA-256 of a deterministic, canonicalised
// proto encoding of the node. Server-set wall-clock fields and Version
// are zeroed so two retries of the same logical Put produce the same
// digest — even when one of them has already been applied and the
// stored row's version has been incremented. Determinism is required
// because Go's default proto marshalling does not guarantee stable map
// ordering.
func contentDigest(node *cpv1.Node) ([]byte, error) {
	canonical := proto.CloneOf(node)
	if meta := canonical.GetMetadata(); meta != nil {
		meta.CreatedAt = nil
		meta.LastModifiedAt = nil
		meta.Version = 0
	}

	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical node: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// digestFromAttrs extracts the content digest attribute from a DynamoDB
// item.
func digestFromAttrs(attrs map[string]types.AttributeValue) ([]byte, bool) {
	if attrs == nil {
		return nil, false
	}
	av, ok := attrs[attrNodeContentDigest]
	if !ok {
		return nil, false
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return b.Value, true
}
