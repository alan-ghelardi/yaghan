package sandbox

import (
	"context"
	"errors"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// ErrAlreadyExists is returned by [DB.Create] when a sandbox with the same id
// already exists and its stored content is not equivalent to the input. Callers
// translate this to a transport-appropriate code (e.g. gRPC AlreadyExists).
var ErrAlreadyExists = errors.New("sandbox already exists")

// ErrNotFound is returned by [DB.Get] and [DB.Update] when no sandbox with the
// requested id exists. Callers translate this to gRPC NotFound.
var ErrNotFound = errors.New("sandbox not found")

// ErrVersionConflict is returned by [DB.Update] when the supplied
// Metadata.Version does not match the currently stored version, indicating a
// concurrent modification. Callers translate this to gRPC Aborted (or
// FailedPrecondition).
var ErrVersionConflict = errors.New("sandbox version conflict")

// ErrInvalidPhaseTransition is returned by [DB.Update] when the requested
// Intent.Phase cannot be applied from the sandbox's current saved phase (e.g.
// pausing a sandbox that is not running). Callers translate this to gRPC
// FailedPrecondition.
var ErrInvalidPhaseTransition = errors.New("invalid sandbox phase transition")

// ErrInvalidListOptions is returned by [DB.List] when the supplied options
// fail the DB layer's invariants (e.g. neither Namespace nor NodeID set,
// PageSize <= 0). Validation that should never fail in production reaches
// this layer only when an upstream check is missing; callers translate to
// gRPC InvalidArgument.
var ErrInvalidListOptions = errors.New("invalid list options")

// ErrInvalidContinuationToken is returned by [DB.List] when the supplied
// continuation token cannot be decoded or does not match the supplied
// filter combination (e.g. it was issued for a different index). Callers
// translate to gRPC InvalidArgument.
var ErrInvalidContinuationToken = errors.New("invalid continuation token")

// ListOptions parameterises a [DB.List] call. The handler layer is responsible
// for applying request-level defaults (e.g. page-size fallbacks) before
// constructing this struct, so the DB layer can treat zero values literally.
type ListOptions struct {
	// Namespace bounds the query to a single namespace. May be empty only
	// when NodeID is set; the DB layer rejects fully unbounded calls.
	Namespace string

	// NodeID bounds the query to sandboxes assigned to this node. May be
	// empty only when Namespace is set.
	NodeID string

	// StatusPhase, when not PHASE_UNSPECIFIED, restricts results to
	// sandboxes in this phase. The DB picks the most-selective index for
	// the supplied (Namespace, NodeID, StatusPhase) tuple and applies an
	// in-application filter on phase when no single index covers the
	// combination — pages may therefore contain fewer than PageSize items.
	StatusPhase controlplanev1alpha1.SandboxStatus_Phase

	// SortOrder controls the order of results by Metadata.LastModifiedAt.
	// The DB does not impose a default; pass an explicit order.
	SortOrder controlplanev1alpha1.ListSandboxesRequest_Order

	// PageSize caps the number of items the DB asks DynamoDB to scan in a
	// single Query call. With phase filtering, fewer items may be
	// returned. Must be > 0.
	PageSize int32

	// ContinuationToken is the opaque token returned by a prior List
	// call, or empty to start from the first page. The token is only
	// valid when paired with the same filter combination it was issued
	// for; mismatched tokens surface as an error from this method.
	ContinuationToken string
}

type DB interface {
	Create(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error

	Update(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error

	Get(ctx context.Context, id string) (*controlplanev1alpha1.Sandbox, error)

	// List returns a page of sandboxes matching opts plus an opaque
	// continuation token. An empty token means there are no more pages.
	// Callers translate transport-specific errors (invalid arguments,
	// invalid continuation token) the same way they translate Get errors.
	List(ctx context.Context, opts ListOptions) ([]*controlplanev1alpha1.Sandbox, string, error)
}
