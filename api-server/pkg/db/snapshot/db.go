// Package snapshot declares the persistence contract for snapshots. The
// shared sentinel errors (AlreadyExists, NotFound, InvalidListOptions,
// InvalidContinuationToken) live in the parent [db] package; snapshots
// have no entity-specific sentinels of their own.
//
// Snapshots are immutable: once created, the row never changes. There is
// no Update method and no Metadata.Version field — the optimistic-locking
// scaffolding used by sandboxes and nodes is unnecessary here.
package snapshot

//go:generate mockgen -source=pkg/db/snapshot/db.go -destination=pkg/db/snapshot/mocks/db.go -package=mocks

import (
	"context"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// ListOptions parameterises a [DB.List] call. The handler layer is
// responsible for applying request-level defaults (page-size fallback,
// sort-order substitution) before constructing this struct, so the DB
// layer can treat zero values literally.
//
// Namespace and SandboxID are mutually exclusive and at least one must be
// set; the underlying DynamoDB schema only provides GSIs partitioned on
// one of them. The proto enforces the same constraint at the RPC layer,
// so reaching the DB without satisfying it indicates a missing upstream
// check.
type ListOptions struct {
	// Namespace bounds the query to a single namespace. May be empty
	// only when SandboxID is set.
	Namespace string

	// SandboxID bounds the query to snapshots taken from this sandbox.
	// May be empty only when Namespace is set.
	SandboxID string

	// SortOrder controls the order of results by Metadata.CreatedAt.
	// The DB does not impose a default; pass an explicit order.
	SortOrder controlplanev1alpha1.ListSnapshotsRequest_Order

	// PageSize caps the number of items returned in a single page.
	// Must be > 0.
	PageSize int32

	// ContinuationToken is the opaque token returned by a prior List
	// call, or empty to start from the first page. The token is only
	// valid when paired with the same filter combination it was issued
	// for; mismatched tokens surface as an error from this method.
	ContinuationToken string
}

// DB is the persistence contract the api-server uses to store snapshots.
// Implementations are expected to be safe for concurrent use; the
// idempotency semantics declared on each method are the contract callers
// rely on.
//
// The DB owns Metadata.CreatedAt on every Snapshot proto it accepts.
// CreatedAt is stamped server-side and any caller-supplied value is
// overwritten.
type DB interface {
	// Create persists a new snapshot under sn.Metadata.Id. On success,
	// Metadata.CreatedAt is stamped with the server's current time on
	// the input proto.
	//
	// Idempotency: a retry whose canonical content (everything except
	// CreatedAt) matches the stored row is a no-op success. A retry
	// with a different body returns db.ErrAlreadyExists.
	Create(ctx context.Context, snapshot *controlplanev1alpha1.Snapshot) error

	// Get returns the snapshot identified by id. Reads are strongly
	// consistent so callers observe their own writes — including writes
	// made by Create on the same node. db.ErrNotFound is returned when
	// no row with that id exists.
	Get(ctx context.Context, id string) (*controlplanev1alpha1.Snapshot, error)

	// List returns a page of snapshots matching opts plus an opaque
	// continuation token. An empty token means there are no more pages.
	// Callers translate transport-specific errors (invalid arguments,
	// invalid continuation token) the same way they translate Get errors.
	List(ctx context.Context, opts ListOptions) ([]*controlplanev1alpha1.Snapshot, string, error)

	// Delete removes the snapshot identified by id. The operation is
	// synchronous and idempotent: a Delete on a missing row returns
	// nil. Snapshots are immutable artifacts with no runtime state to
	// drain, so a two-phase tombstoned delete (as used by sandboxes
	// and nodes) would add complexity without benefit.
	//
	// This method only removes the DB row. Cleaning up the snapshot's
	// associated artifact in durable storage is a separate concern
	// owned by the daemon-side store; orphaned artifacts left behind
	// by a Delete here are wasted storage but never user-visible.
	Delete(ctx context.Context, id string) error
}
