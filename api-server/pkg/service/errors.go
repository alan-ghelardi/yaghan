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
// errors are logged with the supplied entity, op, and id, then surfaced
// as codes.Internal so server internals do not leak to clients.
//
// entity (e.g. "sandbox", "node") is the noun used in the log message
// and the codes.Internal payload — handlers pass the kind they're
// servicing so log lines and client-facing strings read naturally.
//
// Shared sentinels (existence, version conflict, list-option /
// continuation-token validation) live in [db]. Sandbox-specific
// sentinels (e.g. invalid phase transition) live in [sandbox]; for
// entities that don't share that concern the corresponding errors.Is
// branch is simply never matched.
func dbErrToStatus(ctx context.Context, entity, op, id string, err error) error {
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
		ctxzap.Extract(ctx).Error(op+" "+entity+" failed",
			zap.String(entity+"_id", id),
			zap.Error(err))
		return status.Error(codes.Internal, "failed to "+op+" "+entity)
	}
}
