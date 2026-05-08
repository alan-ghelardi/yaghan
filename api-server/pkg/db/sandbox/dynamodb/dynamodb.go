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
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	control_planev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Top-level item attribute names. Kept in sync with
// api-server/dynamodb-tables/sandboxes.json.
//
// sandbox_status_phase is a non-indexed column used only by Update's
// conditional expression and diagnoseUpdateConflict. The GSI partitioned
// on phase keys off the composite gsi_namespace_phase_hk instead.
const (
	attrSandboxID               = "sandbox_id"
	attrSandboxNamespace        = "sandbox_namespace"
	attrSandboxStatusPhase      = "sandbox_status_phase"
	attrNodeRefID               = "node_ref_id"
	attrGSILmtSK                = "gsi_lmt_sk"
	attrGSINamespacePhaseHK     = "gsi_namespace_phase_hk"
	attrGSINamespaceNodeHK      = "gsi_namespace_node_hk"
	attrGSINodePhaseHK          = "gsi_node_phase_hk"
	attrGSINamespaceNodePhaseHK = "gsi_namespace_node_phase_hk"
	attrSandboxVersion          = "sandbox_version"
	attrSandboxPB               = "sandbox_pb"
	attrSandboxContentDigest    = "sandbox_content_digest"
)

// initialVersion is the Metadata.Version stamped on every freshly-created
// sandbox. The DB owns the version field: callers pass a zeroed Version to
// Create, Create writes initialVersion, and Update bumps to expected+1.
const initialVersion int64 = 1

type dynamoDB struct {
	client    dynamodb.Client
	tableName string
}

var _ sandboxdb.DB = (*dynamoDB)(nil)

// New returns a [sandbox.DB] instance backed by AWS DynamoDB.
func New(ctx context.Context, config *config.Bundle) sandboxdb.DB {
	return &dynamoDB{
		client:    dynamodb.Get(ctx),
		tableName: config.Database.AWS.SandboxesTableName,
	}
}

// Create implements [sandbox.DB].
//
// Versioning: the DB owns Metadata.Version. Callers pass a zeroed value;
// Create stamps initialVersion on the input proto and persists it. On
// successful return — including the idempotent-retry no-op — the input
// reflects the stored version.
//
// Wall clock: the DB also owns Metadata.CreatedAt and
// Metadata.LastModifiedAt. Both are stamped with the server's current
// time, overwriting whatever the caller supplied. The digest
// computation zeroes them, so idempotency is unaffected.
//
// Idempotency: the input is canonicalised (server-set wall-clock fields and
// version zeroed) and hashed; the resulting digest is stored alongside the
// row. On a conditional-check failure, the stored digest is compared to the
// input's; if they match, the call returns nil (the existing row is
// preserved). If they differ, [sandbox.ErrAlreadyExists] is returned.
func (d *dynamoDB) Create(ctx context.Context, sb *control_planev1alpha1.Sandbox) error {
	id := sb.GetMetadata().GetId()

	now := timestamppb.Now()
	sb.Metadata.CreatedAt = now
	sb.Metadata.LastModifiedAt = now

	digest, err := contentDigest(sb)
	if err != nil {
		return fmt.Errorf("sandbox %q: build digest: %w", id, err)
	}

	sb.Metadata.Version = initialVersion
	item, err := toItem(sb, digest)
	if err != nil {
		return fmt.Errorf("sandbox %q: build item: %w", id, err)
	}

	_, err = d.client.PutItem(ctx, &dynamodbservice.PutItemInput{
		TableName:                           aws.String(d.tableName),
		Item:                                item,
		ConditionExpression:                 aws.String("attribute_not_exists(" + attrSandboxID + ")"),
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	if err == nil {
		return nil
	}

	var cce *types.ConditionalCheckFailedException
	if !errors.As(err, &cce) {
		return fmt.Errorf("sandbox %q: put item: %w", id, err)
	}

	stored, err := d.fetchStoredItem(ctx, cce, id)
	if err != nil {
		return fmt.Errorf("sandbox %q: read stored row after conflict: %w", id, err)
	}
	storedDigest, ok := digestFromAttrs(stored)
	if !ok {
		return fmt.Errorf("sandbox %q: stored row has no content digest", id)
	}

	if bytes.Equal(digest, storedDigest) {
		return nil
	}
	return fmt.Errorf("sandbox %q: %w", id, db.ErrAlreadyExists)
}

// fetchStoredItem returns the prior item for the given sandbox id following a
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
		zap.String("sandbox_id", id),
	)

	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrSandboxID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return out.Item, nil
}

// Get implements [sandbox.DB].
//
// Reads are strongly consistent so that callers observe their own writes,
// including writes made by Create and Update on the same node.
func (d *dynamoDB) Get(ctx context.Context, id string) (*control_planev1alpha1.Sandbox, error) {
	out, err := d.client.GetItem(ctx, &dynamodbservice.GetItemInput{
		TableName:      aws.String(d.tableName),
		Key:            map[string]types.AttributeValue{attrSandboxID: &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox %q: get item: %w", id, err)
	}
	if len(out.Item) == 0 {
		return nil, fmt.Errorf("sandbox %q: %w", id, db.ErrNotFound)
	}
	return sandboxFromAttrs(out.Item, id)
}

// Update implements [sandbox.DB].
//
// Versioning: the DB owns Metadata.Version. The supplied value is the version
// the caller read; a conditional check ensures the stored row is still at
// that version, and on success the stored version — and the input proto — are
// bumped to expected+1 atomically.
//
// State machine: Status.Phase changes that imply a precondition on the saved
// phase — PHASE_PAUSING requires the saved phase to be PHASE_RUNNING and
// PHASE_RESUMING requires PHASE_PAUSED — are folded into the same conditional
// expression so DynamoDB enforces them in a single round-trip. The api-server
// writes both Status.Phase (the in-progress marker) and Intent.Phase (the
// terminal target) atomically; this layer keys the precondition off the new
// Status.Phase.
//
// Idempotency: a literal retry whose canonical content matches the stored
// row's digest is a no-op success, mirroring Create. The digest ignores the
// version and wall-clock fields, so a retry of the same logical Update — even
// after the first attempt has already applied — still matches.
func (d *dynamoDB) Update(ctx context.Context, sb *control_planev1alpha1.Sandbox) error {
	id := sb.GetMetadata().GetId()
	expectedVersion := sb.GetMetadata().GetVersion()
	targetStatus := sb.GetStatus().GetPhase()

	// The DB owns Metadata.LastModifiedAt: every successful Update bumps
	// it. CreatedAt is preserved from the input (which the caller will
	// have read via Get). The digest computation zeroes both wall-clock
	// fields, so the stamp doesn't affect idempotency.
	sb.Metadata.LastModifiedAt = timestamppb.Now()

	// contentDigest strips Version, so the digest is identical before and
	// after the bump. Compute it once here and reuse it for both the
	// conflict comparison and the stored row.
	digest, err := contentDigest(sb)
	if err != nil {
		return fmt.Errorf("sandbox %q: build digest: %w", id, err)
	}

	sb.Metadata.Version = expectedVersion + 1
	item, err := toItem(sb, digest)
	if err != nil {
		return fmt.Errorf("sandbox %q: build item: %w", id, err)
	}

	cond := "attribute_exists(" + attrSandboxID + ") AND " + attrSandboxVersion + " = :ev"
	values := map[string]types.AttributeValue{
		":ev": &types.AttributeValueMemberN{Value: strconv.FormatInt(expectedVersion, 10)},
	}
	if requiredPhase, ok := requiredPriorPhase(targetStatus); ok {
		cond += " AND " + attrSandboxStatusPhase + " = :rp"
		values[":rp"] = &types.AttributeValueMemberS{Value: requiredPhase.String()}
	}

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
		return fmt.Errorf("sandbox %q: put item: %w", id, err)
	}

	stored, err := d.fetchStoredItem(ctx, cce, id)
	if err != nil {
		return fmt.Errorf("sandbox %q: read stored row after conflict: %w", id, err)
	}
	if len(stored) == 0 {
		return fmt.Errorf("sandbox %q: %w", id, db.ErrNotFound)
	}

	if storedDigest, ok := digestFromAttrs(stored); ok && bytes.Equal(digest, storedDigest) {
		return nil
	}

	return diagnoseUpdateConflict(id, expectedVersion, targetStatus, stored)
}

// diagnoseUpdateConflict turns a conditional-check failure into the most
// specific sentinel error the prior-item attributes support. State-machine
// violations are reported in preference to version conflicts because they are
// usually a programming error in the caller (e.g. trying to pause a sandbox
// that is not yet running) whereas version conflicts are an expected concurrent
// modification.
func diagnoseUpdateConflict(id string, expectedVersion int64, targetStatus control_planev1alpha1.SandboxStatus_Phase, stored map[string]types.AttributeValue) error {
	requiredPhase, hasRequiredPhase := requiredPriorPhase(targetStatus)
	if hasRequiredPhase {
		if savedPhase, ok := stringFromAttrs(stored, attrSandboxStatusPhase); ok && savedPhase != requiredPhase.String() {
			return fmt.Errorf(
				"sandbox %q: cannot transition to status %s from saved phase %s (requires %s): %w",
				id, targetStatus.String(), savedPhase, requiredPhase.String(), sandboxdb.ErrInvalidPhaseTransition,
			)
		}
	}

	if storedVersion, ok := stringFromAttrs(stored, attrSandboxVersion); ok {
		return fmt.Errorf(
			"sandbox %q: expected version %d but stored version is %s: %w",
			id, expectedVersion, storedVersion, db.ErrVersionConflict,
		)
	}

	return fmt.Errorf("sandbox %q: %w", id, db.ErrVersionConflict)
}

// requiredPriorPhase maps a target Status.Phase (the in-progress marker the
// api-server is about to write) to the saved phase the stored row must be in
// for the transition to be legal. The boolean is false for target statuses
// that do not impose a precondition on the saved phase at this layer.
func requiredPriorPhase(targetStatus control_planev1alpha1.SandboxStatus_Phase) (control_planev1alpha1.SandboxStatus_Phase, bool) {
	switch targetStatus {
	case control_planev1alpha1.SandboxStatus_PHASE_PAUSING:
		return control_planev1alpha1.SandboxStatus_PHASE_RUNNING, true
	case control_planev1alpha1.SandboxStatus_PHASE_RESUMING:
		return control_planev1alpha1.SandboxStatus_PHASE_PAUSED, true
	default:
		return control_planev1alpha1.SandboxStatus_PHASE_UNSPECIFIED, false
	}
}

func sandboxFromAttrs(attrs map[string]types.AttributeValue, id string) (*control_planev1alpha1.Sandbox, error) {
	av, ok := attrs[attrSandboxPB]
	if !ok {
		return nil, fmt.Errorf("sandbox %q: stored row has no %s attribute", id, attrSandboxPB)
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, fmt.Errorf("sandbox %q: stored %s is not bytes", id, attrSandboxPB)
	}
	sb := &control_planev1alpha1.Sandbox{}
	if err := proto.Unmarshal(b.Value, sb); err != nil {
		return nil, fmt.Errorf("sandbox %q: unmarshal stored row: %w", id, err)
	}
	return sb, nil
}

// stringFromAttrs returns the value of a string- or number-typed attribute as
// a Go string. Number attributes are encoded as strings in the AWS SDK.
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

// toItem builds the DynamoDB item map for a Sandbox. The caller supplies the
// content digest so it is computed at most once per write — Update reuses the
// digest of the input across the version-bumped clone.
func toItem(sb *control_planev1alpha1.Sandbox, digest []byte) (map[string]types.AttributeValue, error) {
	pb, err := proto.Marshal(sb)
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox: %w", err)
	}

	meta := sb.GetMetadata()
	item := map[string]types.AttributeValue{
		attrSandboxID:            &types.AttributeValueMemberS{Value: meta.GetId()},
		attrSandboxNamespace:     &types.AttributeValueMemberS{Value: meta.GetNamespace()},
		attrSandboxStatusPhase:   &types.AttributeValueMemberS{Value: sb.GetStatus().GetPhase().String()},
		attrGSILmtSK:             &types.AttributeValueMemberS{Value: gsiLmtSK(sb)},
		attrGSINamespacePhaseHK:  &types.AttributeValueMemberS{Value: gsiNamespacePhaseHK(sb)},
		attrSandboxVersion:       &types.AttributeValueMemberN{Value: strconv.FormatInt(meta.GetVersion(), 10)},
		attrSandboxPB:            &types.AttributeValueMemberB{Value: pb},
		attrSandboxContentDigest: &types.AttributeValueMemberB{Value: digest},
	}

	// All node-related attributes are sparse: a row without an assigned
	// node must be absent from every node-keyed GSI. Gating them all
	// behind the same predicate keeps the four indexes consistent.
	if nodeID := sb.GetNode().GetId(); nodeID != "" {
		item[attrNodeRefID] = &types.AttributeValueMemberS{Value: nodeID}
		item[attrGSINamespaceNodeHK] = &types.AttributeValueMemberS{Value: gsiNamespaceNodeHK(sb)}
		item[attrGSINodePhaseHK] = &types.AttributeValueMemberS{Value: gsiNodePhaseHK(sb)}
		item[attrGSINamespaceNodePhaseHK] = &types.AttributeValueMemberS{Value: gsiNamespaceNodePhaseHK(sb)}
	}

	return item, nil
}

// gsiLmtSK builds the shared sort key used by all four GSIs:
// "{last_modified_at_rfc3339_nano_utc}#{id}". LastModifiedAt is DB-stamped on
// every Create/Update, so this attribute is always present and orders rows
// monotonically within a partition.
func gsiLmtSK(sb *control_planev1alpha1.Sandbox) string {
	meta := sb.GetMetadata()
	return meta.GetLastModifiedAt().AsTime().UTC().Format(time.RFC3339Nano) + "#" + meta.GetId()
}

// gsiNamespacePhaseHK builds the hash key of by_status_phase_index:
// "{namespace}#{status_phase}". Embedding the namespace eliminates the hot
// partition that a phase-only HK produces — every Query is implicitly bounded
// to a single namespace, which matches the only access pattern callers need.
func gsiNamespacePhaseHK(sb *control_planev1alpha1.Sandbox) string {
	return sb.GetMetadata().GetNamespace() + "#" + sb.GetStatus().GetPhase().String()
}

// gsiNamespaceNodeHK builds the hash key of by_namespace_node_index:
// "{namespace}#{node_ref_id}". Sparse: callers must only emit this attribute
// when Node.Id is set, mirroring node_ref_id's sparseness.
func gsiNamespaceNodeHK(sb *control_planev1alpha1.Sandbox) string {
	return sb.GetMetadata().GetNamespace() + "#" + sb.GetNode().GetId()
}

// gsiNodePhaseHK builds the hash key of by_node_phase_index:
// "{node_ref_id}#{status_phase}". Sparse: emitted only when Node.Id is set.
// Lets list-by-node-and-phase queries hit a dedicated index instead of
// filtering in-app after a by_node_index Query.
func gsiNodePhaseHK(sb *control_planev1alpha1.Sandbox) string {
	return sb.GetNode().GetId() + "#" + sb.GetStatus().GetPhase().String()
}

// gsiNamespaceNodePhaseHK builds the hash key of
// by_namespace_node_phase_index: "{namespace}#{node_ref_id}#{status_phase}".
// Sparse: emitted only when Node.Id is set. Lets list-by-namespace-node-
// and-phase queries hit a dedicated index instead of filtering in-app after
// a by_namespace_node_index Query.
func gsiNamespaceNodePhaseHK(sb *control_planev1alpha1.Sandbox) string {
	return sb.GetMetadata().GetNamespace() + "#" + sb.GetNode().GetId() + "#" + sb.GetStatus().GetPhase().String()
}

// contentDigest returns the SHA-256 of a deterministic, canonicalised proto
// encoding of the sandbox. Server-managed fields are zeroed so that two
// retries of the same logical mutation produce the same digest — even
// when one of them has already been applied. The stripped fields are:
//
//   - Metadata.Version, Metadata.CreatedAt, Metadata.LastModifiedAt: stamped
//     by the DB on every successful write.
//   - Node: assigned by the scheduler before Create. The random scheduler
//     may pick a different node on retry, and the dedicated Node-change
//     RPC is the only sanctioned path to alter Node post-Create — so
//     callers must treat Node as server-owned for idempotency purposes.
//
// Determinism is required because Go's default proto marshalling does
// not guarantee stable map ordering.
func contentDigest(sb *control_planev1alpha1.Sandbox) ([]byte, error) {
	canonical := proto.CloneOf(sb)
	if meta := canonical.GetMetadata(); meta != nil {
		meta.CreatedAt = nil
		meta.LastModifiedAt = nil
		meta.Version = 0
	}
	canonical.Node = nil

	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical sandbox: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// digestFromAttrs extracts the content digest attribute from a DynamoDB item.
func digestFromAttrs(attrs map[string]types.AttributeValue) ([]byte, bool) {
	if attrs == nil {
		return nil, false
	}
	av, ok := attrs[attrSandboxContentDigest]
	if !ok {
		return nil, false
	}
	b, ok := av.(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return b.Value, true
}
