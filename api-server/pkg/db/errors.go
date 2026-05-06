// Package db hosts sentinel errors shared by the api-server's per-entity
// persistence packages (sandbox, node, …). Each entity package re-uses
// these sentinels so the service layer can map error semantics to gRPC
// status codes uniformly, regardless of which DB was actually queried.
//
// Entity-specific errors stay in the owning package; only the shapes
// every entity needs (existence, version conflict, list-option
// validation, continuation-token validation) live here.
package db

import "errors"

// ErrAlreadyExists is returned when a write would conflict with an
// existing row whose stored content does not match the input. Callers
// translate this to gRPC AlreadyExists.
var ErrAlreadyExists = errors.New("entity already exists")

// ErrNotFound is returned when a read or update targets a row that
// does not exist. Callers translate this to gRPC NotFound.
var ErrNotFound = errors.New("entity not found")

// ErrVersionConflict is returned when an optimistic-locking write
// observes a stored version different from the one the caller read,
// indicating a concurrent modification. Callers translate this to gRPC
// Aborted (or FailedPrecondition).
var ErrVersionConflict = errors.New("entity version conflict")

// ErrInvalidListOptions is returned when the supplied List options
// fail a DB-layer invariant (e.g. a filter combination the layer
// rejects, PageSize <= 0, an unspecified sort order). Validation that
// should never fail in production reaches this layer only when an
// upstream check is missing; callers translate this to gRPC
// InvalidArgument.
var ErrInvalidListOptions = errors.New("invalid list options")

// ErrInvalidContinuationToken is returned when a List call receives a
// continuation token that cannot be decoded or does not match the
// supplied filter combination (e.g. it was issued for a different
// index). Callers translate this to gRPC InvalidArgument.
var ErrInvalidContinuationToken = errors.New("invalid continuation token")
