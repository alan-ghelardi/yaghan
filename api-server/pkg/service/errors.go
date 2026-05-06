package service

import (
	"context"
	"errors"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"golang.nuinfra.api-server/pkg/db"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dbErrToStatus maps a DB-layer sentinel error to a gRPC status. Unknown
// errors are logged with the supplied op and sandbox id, then surfaced as
// codes.Internal so server internals do not leak to clients.
//
// Shared sentinels (existence, version conflict, list-option /
// continuation-token validation) live in [db]. Sandbox-specific
// sentinels (e.g. invalid phase transition) live in [sandbox].
func dbErrToStatus(ctx context.Context, op, sandboxID string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, db.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, db.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, db.ErrVersionConflict):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, sandboxdb.ErrInvalidPhaseTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, db.ErrInvalidListOptions),
		errors.Is(err, db.ErrInvalidContinuationToken):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		ctxzap.Extract(ctx).Error(op+" sandbox failed",
			zap.String("sandbox_id", sandboxID),
			zap.Error(err))
		return status.Error(codes.Internal, "failed to "+op+" sandbox")
	}
}
