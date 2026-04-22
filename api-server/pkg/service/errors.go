package service

import (
	"context"
	"errors"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	sandboxdb "golang.nuinfra.api-server/pkg/db/sandbox"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dbErrToStatus maps a sandboxdb sentinel error to a gRPC status. Unknown
// errors are logged with the supplied op and sandbox id, then surfaced as
// codes.Internal so server internals do not leak to clients.
func dbErrToStatus(ctx context.Context, op, sandboxID string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sandboxdb.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, sandboxdb.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, sandboxdb.ErrVersionConflict):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, sandboxdb.ErrInvalidPhaseTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		ctxzap.Extract(ctx).Error(op+" sandbox failed",
			zap.String("sandbox_id", sandboxID),
			zap.Error(err))
		return status.Error(codes.Internal, "failed to "+op+" sandbox")
	}
}
