package validation

import (
	"context"

	"buf.build/go/protovalidate"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// UnaryServerInterceptor validates incoming requests by evaluating the
// constraints declared in proto messages.
func UnaryServerInterceptor(ctx context.Context) grpc.UnaryServerInterceptor {
	validator, err := protovalidate.New()
	if err != nil {
		logger := ctxzap.Extract(ctx)
		logger.Fatal("failed to instantiate the validator", zap.Error(err))
	}
	return func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		protoMessage, ok := request.(proto.Message)
		if ok {
			if err := validator.Validate(protoMessage); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		} else {
			logger := ctxzap.Extract(ctx).Sugar()
			logger.Warnf("unable to validate %T", request)
		}
		return handler(ctx, request)
	}
}
