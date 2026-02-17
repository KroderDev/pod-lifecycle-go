package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/kroderdev/pod-lifecycle-go/internal/config"
)

// stubChecker satisfies check.Checker without importing the internal package directly.
type stubChecker struct{}

func (stubChecker) Check(_ context.Context) error { return nil }

func TestDefaultConfig(t *testing.T) {
	cfg, err := config.ApplyOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort: got %d, want 8080", cfg.HTTPPort)
	}
	if cfg.GRPCPort != 50051 {
		t.Errorf("GRPCPort: got %d, want 50051", cfg.GRPCPort)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout: got %v, want 5s", cfg.ShutdownTimeout)
	}
	if cfg.CheckerTimeout != 2*time.Second {
		t.Errorf("CheckerTimeout: got %v, want 2s", cfg.CheckerTimeout)
	}
	if cfg.CheckMechanism != config.CheckHTTP {
		t.Errorf("CheckMechanism: got %v, want CheckHTTP", cfg.CheckMechanism)
	}
}

func TestPortValidation(t *testing.T) {
	invalid := []int{0, -1, 65536, -100}
	for _, p := range invalid {
		_, err := config.ApplyOptions([]config.Option{config.WithHTTPPort(p)})
		if err == nil {
			t.Errorf("HTTPPort %d: expected error, got nil", p)
		}
		_, err = config.ApplyOptions([]config.Option{config.WithGRPCPort(p)})
		if err == nil {
			t.Errorf("GRPCPort %d: expected error, got nil", p)
		}
	}
	valid := []int{1, 80, 8080, 65535}
	for _, p := range valid {
		_, err := config.ApplyOptions([]config.Option{config.WithHTTPPort(p)})
		if err != nil {
			t.Errorf("HTTPPort %d: unexpected error: %v", p, err)
		}
		_, err = config.ApplyOptions([]config.Option{config.WithGRPCPort(p)})
		if err != nil {
			t.Errorf("GRPCPort %d: unexpected error: %v", p, err)
		}
	}
}

func TestWithShutdownTimeout(t *testing.T) {
	cfg, err := config.ApplyOptions([]config.Option{config.WithShutdownTimeout(10 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("got %v, want 10s", cfg.ShutdownTimeout)
	}
}

func TestWithCheckerTimeout(t *testing.T) {
	cfg, err := config.ApplyOptions([]config.Option{config.WithCheckerTimeout(500 * time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CheckerTimeout != 500*time.Millisecond {
		t.Errorf("got %v, want 500ms", cfg.CheckerTimeout)
	}
}

func TestWithCheckerRegistration(t *testing.T) {
	c1, c2 := stubChecker{}, stubChecker{}
	cfg, err := config.ApplyOptions([]config.Option{
		config.WithChecker("db", c1),
		config.WithChecker("cache", c2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checkers) != 2 {
		t.Errorf("want 2 checkers, got %d", len(cfg.Checkers))
	}
}

func TestWithCheckerDuplicateOverwrites(t *testing.T) {
	c1, c2 := stubChecker{}, stubChecker{}
	cfg, err := config.ApplyOptions([]config.Option{
		config.WithChecker("db", c1),
		config.WithChecker("db", c2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checkers) != 1 {
		t.Errorf("want 1 checker, got %d", len(cfg.Checkers))
	}
}

func TestNewProbeHTTPNonNil(t *testing.T) {
	cfg, _ := config.ApplyOptions(nil)
	p := config.NewProbe(cfg)
	if p == nil {
		t.Error("NewProbe(CheckHTTP) returned nil")
	}
}

func TestNewProbeGRPCNonNil(t *testing.T) {
	cfg, _ := config.ApplyOptions([]config.Option{config.WithCheckMechanism(config.CheckGRPC)})
	p := config.NewProbe(cfg)
	if p == nil {
		t.Error("NewProbe(CheckGRPC) returned nil")
	}
}
