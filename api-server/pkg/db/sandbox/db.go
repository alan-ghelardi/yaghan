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

type DB interface {
	Create(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error

	Update(ctx context.Context, sandbox *controlplanev1alpha1.Sandbox) error

	Get(ctx context.Context, id string) (*controlplanev1alpha1.Sandbox, error)
}
