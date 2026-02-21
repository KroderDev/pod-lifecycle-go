package podlifecycle

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
)

func TestLoggingUnaryInterceptor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	interceptor := LoggingUnaryInterceptor(logger)

	nullHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "resp", nil
	}

	errHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("boom")
	}

	t.Run("logs normal request", func(t *testing.T) {
		buf.Reset()
		_, _ = interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/service/Method"}, nullHandler)

		output := buf.String()
		if !strings.Contains(output, "level=INFO") {
			t.Errorf("expected INFO level, got %s", output)
		}
		if !strings.Contains(output, "method=/service/Method") {
			t.Errorf("expected method in log, got %s", output)
		}
		if !strings.Contains(output, "code=OK") {
			t.Errorf("expected code=OK, got %s", output)
		}
	})

	t.Run("skips health check", func(t *testing.T) {
		buf.Reset()
		_, _ = interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, nullHandler)

		if buf.Len() > 0 {
			t.Errorf("expected no logs for health check, got %s", buf.String())
		}
	})

	t.Run("logs error as warn", func(t *testing.T) {
		buf.Reset()
		_, _ = interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/service/Fail"}, errHandler)

		output := buf.String()
		if !strings.Contains(output, "level=WARN") {
			t.Errorf("expected WARN level, got %s", output)
		}
		if !strings.Contains(output, "msg=\"grpc request error\"") {
			t.Errorf("expected error message, got %s", output)
		}
		if !strings.Contains(output, "err=boom") {
			t.Errorf("expected error detail, got %s", output)
		}
	})
}
