package dynamodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/alan-ghelardi/yaghan/api-server/pkg/db"
	nodedb "github.com/alan-ghelardi/yaghan/api-server/pkg/db/node"
	cpv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// GSI names. Kept in sync with api-server/dynamodb-tables/nodes.json.
const (
	indexAllNodes      = "all_nodes_index"
	indexByStatusPhase = "by_status_phase_index"
)

// List implements [node.DB].
//
// Index selection picks the most-selective GSI for the supplied
// StatusPhase filter:
//
//	(no phase) -> all_nodes_index            (hk = constant)
//	(phase)    -> by_status_phase_index      (hk = node_status_phase)
//
// all_nodes_index is intentionally a hot partition: cluster sizes are
// bounded, so paying for cross-partition fan-out for the rare "list
// every node" query would be the wrong trade-off.
//
// Sort order is encoded as the GSI's ScanIndexForward flag against the
// gsi_lmt_sk range key.
//
// Continuation tokens embed the index name they were issued for;
// replaying a token against a different filter combination is rejected
// with [db.ErrInvalidContinuationToken] rather than silently returning
// wrong data.
func (d *dynamoDB) List(ctx context.Context, opts nodedb.ListOptions) ([]*cpv1.Node, string, error) {
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
		return nil, "", fmt.Errorf("list nodes: query %s: %w", indexName, err)
	}

	nodes := make([]*cpv1.Node, 0, len(out.Items))
	for _, item := range out.Items {
		id, _ := stringFromAttrs(item, attrNodeID)
		n, err := nodeFromAttrs(item, id)
		if err != nil {
			return nil, "", fmt.Errorf("list nodes: %w", err)
		}
		nodes = append(nodes, n)
	}

	nextToken, err := encodeContinuationToken(indexName, out.LastEvaluatedKey)
	if err != nil {
		return nil, "", fmt.Errorf("list nodes: encode continuation token: %w", err)
	}
	return nodes, nextToken, nil
}

// scanIndexForward maps a SortOrder to DynamoDB's ScanIndexForward flag.
// gsi_lmt_sk encodes the timestamp first, so descending the SK is
// "newest first" and ascending is "oldest first". ORDER_UNSPECIFIED is
// rejected at this layer; the handler is responsible for substituting
// a default before calling List.
func scanIndexForward(order cpv1.ListNodesRequest_Order) (bool, error) {
	switch order {
	case cpv1.ListNodesRequest_ORDER_NEWEST_FIRST:
		return false, nil
	case cpv1.ListNodesRequest_ORDER_OLDEST_FIRST:
		return true, nil
	default:
		return false, fmt.Errorf("sort_order must be NEWEST_FIRST or OLDEST_FIRST: %w", db.ErrInvalidListOptions)
	}
}

// selectIndex picks the GSI for the supplied filter combination and
// returns its name plus the hash-key attribute and the concrete
// partition value to query.
func selectIndex(opts nodedb.ListOptions) (indexName, hkName, hkValue string) {
	if opts.StatusPhase != cpv1.NodeStatus_PHASE_UNSPECIFIED {
		return indexByStatusPhase, attrNodeStatusPhase, opts.StatusPhase.String()
	}
	return indexAllNodes, attrGSIConstHK, allNodesPartition
}

// continuationTokenPayload is the JSON shape encoded into the opaque
// token returned to clients. The embedded index name lets List reject
// tokens reused across mismatched filter combinations with a precise
// error.
type continuationTokenPayload struct {
	IndexName string            `json:"i"`
	Key       map[string]string `json:"k"`
}

// encodeContinuationToken returns base64(JSON(payload)) for a non-empty
// LEK, or "" when DynamoDB indicates no further pages. All keys on this
// table are strings, so the LEK is safely flattened into a
// string-valued map.
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

// decodeContinuationToken decodes a token produced by
// encodeContinuationToken and returns the corresponding
// ExclusiveStartKey. An empty token returns nil. Tokens whose embedded
// index does not match the index selected for the current request are
// rejected with [db.ErrInvalidContinuationToken].
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
