package podlifecycle

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const healthServicePrefix = "/grpc.health.v1.Health/"

// LoggingUnaryInterceptor returns a gRPC unary server interceptor that logs
// every request with method name, duration, and status code.
// It automatically skips logging for health check requests.
func LoggingUnaryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip logging for health checks to avoid noise.
		if strings.HasPrefix(info.FullMethod, healthServicePrefix) {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := status.Code(err)
		log.Info("grpc request",
			"method", info.FullMethod,
			"code", code.String(),
			"duration", duration,
		)
		if err != nil {
			log.Warn("grpc request error",
				"method", info.FullMethod,
				"code", code.String(),
				"err", err,
			)
		}
		return resp, err
	}
}
