package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/alan-ghelardi/yaghan/api-server/pkg/db"
	snapshotdb "github.com/alan-ghelardi/yaghan/api-server/pkg/db/snapshot"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// GSI names. Kept in sync with api-server/dynamodb-tables/snapshots.json.
const (
	indexByNamespace = "by_namespace_index"
	indexBySandbox   = "by_sandbox_index"
)

// List implements [snapshot.DB].
//
// Index selection picks the GSI that covers the supplied filter
// (Namespace XOR SandboxID). The proto enforces mutual exclusion and
// at least-one requirement; this layer still validates both because the
// DB is the last line of defense against an unbounded scan:
//
//	(namespace)  -> by_namespace_index (hk = snapshot_namespace)
//	(sandbox_id) -> by_sandbox_index   (hk = sandbox_ref_id)
//
// Sort order is encoded as the GSI's ScanIndexForward flag against the
// gsi_ct_sk range key, which embeds CreatedAt first.
//
// Continuation tokens embed the index name they were issued for;
// replaying a token against a different filter combination is rejected
// with [db.ErrInvalidContinuationToken] rather than silently returning
// wrong data.
func (d *dynamoDB) List(ctx context.Context, opts snapshotdb.ListOptions) ([]*cpv1.Snapshot, string, error) {
	if opts.Namespace == "" && opts.SandboxID == "" {
		return nil, "", fmt.Errorf("namespace or sandbox_id required: %w", db.ErrInvalidListOptions)
	}
	if opts.Namespace != "" && opts.SandboxID != "" {
		return nil, "", fmt.Errorf("namespace and sandbox_id are mutually exclusive: %w", db.ErrInvalidListOptions)
	}
	if opts.PageSize <= 0 {
		return nil, "", fmt.Errorf("page_size must be > 0: %w", db.ErrInvalidListOptions)
	}

	scanForward, err := scanIndexForward(opts.SortOrder)
	if err != nil {
		return nil, "", err
	}

	indexName, hkName, hkValue := selectIndex(opts)

	exclusiveStartKey, err := decodeContinuationToken(opts.ContinuationToken, indexName)
	if err != nil {
		return nil, "", err
	}

	out, err := d.client.Query(ctx, &dynamodbservice.QueryInput{
		TableName:                aws.String(d.tableName),
		IndexName:                aws.String(indexName),
		KeyConditionExpression:   aws.String("#hk = :hk"),
		ExpressionAttributeNames: map[string]string{"#hk": hkName},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":hk": &types.AttributeValueMemberS{Value: hkValue},
		},
		ScanIndexForward:  aws.Bool(scanForward),
		Limit:             aws.Int32(opts.PageSize),
		ExclusiveStartKey: exclusiveStartKey,
	})
	if err != nil {
		return nil, "", fmt.Errorf("list snapshots: query %s: %w", indexName, err)
	}

	snapshots := make([]*cpv1.Snapshot, 0, len(out.Items))
	for _, item := range out.Items {
		id, _ := stringFromAttrs(item, attrSnapshotID)
		sn, err := snapshotFromAttrs(item, id)
		if err != nil {
			return nil, "", fmt.Errorf("list snapshots: %w", err)
		}
		snapshots = append(snapshots, sn)
	}

	nextToken, err := encodeContinuationToken(indexName, out.LastEvaluatedKey)
	if err != nil {
		return nil, "", fmt.Errorf("list snapshots: encode continuation token: %w", err)
	}
	return snapshots, nextToken, nil
}

// scanIndexForward maps a SortOrder to DynamoDB's ScanIndexForward flag.
// gsi_ct_sk encodes the timestamp first, so descending the SK is "newest
// first" and ascending is "oldest first". ORDER_UNSPECIFIED is rejected
// at this layer; the handler is responsible for substituting a default
// before calling List.
func scanIndexForward(order cpv1.ListSnapshotsRequest_Order) (bool, error) {
	switch order {
	case cpv1.ListSnapshotsRequest_ORDER_NEWEST_FIRST:
		return false, nil
	case cpv1.ListSnapshotsRequest_ORDER_OLDEST_FIRST:
		return true, nil
	default:
		return false, fmt.Errorf("sort_order must be NEWEST_FIRST or OLDEST_FIRST: %w", db.ErrInvalidListOptions)
	}
}

// selectIndex picks the GSI that covers the supplied filter and returns
// its name plus the hash-key attribute and the concrete partition value
// to query. Callers must ensure exactly one of opts.Namespace or
// opts.SandboxID is set.
func selectIndex(opts snapshotdb.ListOptions) (indexName, hkName, hkValue string) {
	if opts.Namespace != "" {
		return indexByNamespace, attrSnapshotNamespace, opts.Namespace
	}
	return indexBySandbox, attrSandboxRefID, opts.SandboxID
}

// continuationTokenPayload is the JSON shape encoded into the opaque token
// returned to clients. The embedded index name lets List reject tokens
// reused across mismatched filter combinations with a precise error.
type continuationTokenPayload struct {
	IndexName string            `json:"i"`
	Key       map[string]string `json:"k"`
}

// encodeContinuationToken returns base64(JSON(payload)) for a non-empty LEK,
// or "" when DynamoDB indicates no further pages. All keys on this table
// are strings, so the LEK is safely flattened into a string-valued map.
func encodeContinuationToken(indexName string, lek map[string]types.AttributeValue) (string, error) {
	if len(lek) == 0 {
		return "", nil
	}
	flat := make(map[string]string, len(lek))
	for k, v := range lek {
		s, ok := v.(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("unexpected non-string LEK attribute %q", k)
		}
		flat[k] = s.Value
	}
	payload := continuationTokenPayload{IndexName: indexName, Key: flat}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal continuation token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// decodeContinuationToken decodes a token produced by encodeContinuationToken
// and returns the corresponding ExclusiveStartKey. An empty token returns
// nil. Tokens whose embedded index does not match the index selected for
// the current request are rejected with [db.ErrInvalidContinuationToken].
func decodeContinuationToken(token, indexName string) (map[string]types.AttributeValue, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode continuation token: %s: %w", err.Error(), db.ErrInvalidContinuationToken)
	}
	var payload continuationTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal continuation token: %s: %w", err.Error(), db.ErrInvalidContinuationToken)
	}
	if payload.IndexName != indexName {
		return nil, fmt.Errorf("continuation token was issued for index %q, current request targets %q: %w",
			payload.IndexName, indexName, db.ErrInvalidContinuationToken)
	}
	out := make(map[string]types.AttributeValue, len(payload.Key))
	for k, v := range payload.Key {
		out[k] = &types.AttributeValueMemberS{Value: v}
	}
	return out, nil
}
