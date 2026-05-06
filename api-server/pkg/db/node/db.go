// Package node declares the persistence contract for nodes. The shared
// sentinel errors (AlreadyExists, NotFound, VersionConflict,
// InvalidListOptions, InvalidContinuationToken) live in the parent [db]
// package; node-specific concerns are declared here.
//
// Nodes use a single Put method instead of distinct Create/Update calls:
// the operation is dispatched on Metadata.Version (zero = create, > 0 =
// update) and there is no node phase state-machine to enforce.
package node

import (
	"context"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// ListOptions parameterises a [DB.List] call. The handler layer is
// responsible for applying request-level defaults (page-size fallback,
// sort-order substitution) before constructing this struct, so the DB
// layer can treat zero values literally.
type ListOptions struct {
	// StatusPhase, when not PHASE_UNSPECIFIED, restricts results to
	// nodes in this phase. The DB picks the most-selective index for
	// the supplied filter combination.
	StatusPhase controlplanev1alpha1.NodeStatus_Phase

	// SortOrder controls the order of results by Metadata.LastModifiedAt.
	// The DB does not impose a default; pass an explicit order.
	SortOrder controlplanev1alpha1.ListNodesRequest_Order

	// PageSize caps the number of items the DB asks DynamoDB to scan in
	// a single Query call. Must be > 0.
	PageSize int32

	// ContinuationToken is the opaque token returned by a prior List
	// call, or empty to start from the first page. The token is only
	// valid when paired with the same filter combination it was issued
	// for; mismatched tokens surface as an error from this method.
	ContinuationToken string
}

// DB is the persistence contract the api-server uses to store nodes.
// Implementations are expected to be safe for concurrent use; the
// optimistic-locking and idempotency semantics declared on each method
// are the contract callers rely on.
//
// The DB owns three fields on every Node proto it accepts:
// Metadata.Version, Metadata.CreatedAt and Metadata.LastModifiedAt.
// Callers must pass a zeroed Version on the very first Put and the
// previously read Version on subsequent Puts; CreatedAt and
// LastModifiedAt are stamped server-side and any caller-supplied
// values are overwritten.
type DB interface {
	// Put creates or updates the node identified by node.Metadata.Id.
	// The caller passes Version == 0 to create; the DB inserts the row
	// and stamps Metadata.Version with the initial version,
	// Metadata.CreatedAt and Metadata.LastModifiedAt with the server's
	// current time. The caller passes the previously read Version to
	// update; the DB rejects the call with db.ErrVersionConflict when
	// the stored row has since advanced, otherwise it bumps
	// Metadata.Version and stamps Metadata.LastModifiedAt.
	//
	// Idempotency: a retry whose canonical content (everything except
	// the DB-owned wall-clock and version fields) matches the stored
	// row is a no-op success. On the create branch a retry with a
	// different body returns db.ErrAlreadyExists; on the update branch,
	// db.ErrVersionConflict. db.ErrNotFound is returned when an update
	// targets a row that does not exist.
	Put(ctx context.Context, node *controlplanev1alpha1.Node) error

	// Get returns the node identified by id. Reads are strongly
	// consistent so callers observe their own writes. db.ErrNotFound
	// is returned when no row with that id exists.
	Get(ctx context.Context, id string) (*controlplanev1alpha1.Node, error)

	// List returns a page of nodes matching opts plus an opaque
	// continuation token. An empty token means there are no more
	// pages. Callers translate transport-specific errors (invalid
	// arguments, invalid continuation token) the same way they
	// translate Get errors.
	List(ctx context.Context, opts ListOptions) ([]*controlplanev1alpha1.Node, string, error)
}
