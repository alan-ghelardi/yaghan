package auth

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/zap"
	authv1 "golang.nuinfra.net/apis/gen/nuinfra/auth/v1"
	"golang.nuinfra.net/commons/pkg/config"
	"google.golang.org/grpc"
)

type ActorKey struct{}

// GetActor returns the actor who initiated the current request, if the request
// was authenticated successfully. If authentication is disabled on the server,
// the request lacks an OAuth token, or the authentication interceptors are not
// configured in the middleware chain, it returns nil.
func GetActor(ctx context.Context) *authv1.Actor {
	actor, ok := ctx.Value(ActorKey{}).(*authv1.Actor)
	if ok {
		return actor
	}
	return nil
}

func UnaryServerInterceptor(ctx context.Context, config *config.Auth) grpc.UnaryServerInterceptor {
	authorizer, err := NewAuthorizer(ctx, config)
	if err != nil {
		logger := ctxzap.Extract(ctx)
		logger.Fatal("unable to setup authorizer", zap.Error(err))
	}

	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		actor, err := authorizer.Authorize(ctx, info.FullMethod)
		if actor != nil {
			ctx = context.WithValue(ctx, ActorKey{}, actor)
			setActorTags(ctx, actor)
		}

		if err != nil {
			return nil, err
		}
		return handler(ctx, request)
	}
}

func setActorTags(ctx context.Context, actor *authv1.Actor) {
	tags := grpc_ctxtags.Extract(ctx)
	tags.Set("actor.id", actor.ActorId)
	if actor.Username != nil {
		tags.Set("actor.username", *actor.Username)
	}
}

func StreamServerInterceptor(ctx context.Context, config *config.Auth) grpc.StreamServerInterceptor {
	authorizer, err := NewAuthorizer(ctx, config)
	if err != nil {
		logger := ctxzap.Extract(ctx)
		logger.Fatal("unable to setup authorizer", zap.Error(err))
	}

	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		actor, err := authorizer.Authorize(stream.Context(), info.FullMethod)

		if actor != nil {
			ctx = context.WithValue(ctx, ActorKey{}, actor)
			setActorTags(ctx, actor)
		}

		if err != nil {
			return err
		}

		return handler(server, stream)
	}
}
