package config

import (
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/kroderdev/pod-lifecycle-go/internal/check"
)

// CheckMechanism is the probe mechanism used for readiness, liveness, and startup.
type CheckMechanism int

const (
	// CheckHTTP uses HTTP GET to paths /ready, /live, /startup.
	CheckHTTP CheckMechanism = iota
	// CheckGRPC uses the gRPC health protocol with service names "ready", "live", "startup".
	CheckGRPC
)

// Config holds PodManager configuration.
type Config struct {
	CheckMechanism     CheckMechanism
	HTTPPort           int
	GRPCPort           int
	ShutdownTimeout    time.Duration
	CheckerTimeout     time.Duration
	Checkers           map[string]check.Checker
	ErrorHandler       func(error)
	ExistingGRPCServer *grpc.Server
	ExistingHTTPMux    *http.ServeMux
}

func defaultConfig() Config {
	return Config{
		CheckMechanism:  CheckHTTP,
		HTTPPort:        8080,
		GRPCPort:        50051,
		ShutdownTimeout: 5 * time.Second,
		CheckerTimeout:  2 * time.Second,
		Checkers:        make(map[string]check.Checker),
	}
}

// Option configures a PodManager.
type Option func(*Config)

// WithCheckMechanism sets the probe mechanism (HTTP or gRPC).
func WithCheckMechanism(m CheckMechanism) Option {
	return func(c *Config) {
		c.CheckMechanism = m
	}
}

// WithHTTPPort sets the port for HTTP probes.
func WithHTTPPort(port int) Option {
	return func(c *Config) {
		c.HTTPPort = port
	}
}

// WithGRPCPort sets the port for gRPC health probes.
func WithGRPCPort(port int) Option {
	return func(c *Config) {
		c.GRPCPort = port
	}
}

// WithShutdownTimeout sets the maximum time to wait for probe servers to drain.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.ShutdownTimeout = d
	}
}

// WithCheckerTimeout sets the per-checker deadline for /ready dependency checks.
func WithCheckerTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.CheckerTimeout = d
	}
}

// WithChecker registers a named dependency checker run on every /ready request.
// Registering the same name twice overwrites the previous checker.
func WithChecker(name string, ch check.Checker) Option {
	return func(c *Config) {
		if c.Checkers == nil {
			c.Checkers = make(map[string]check.Checker)
		}
		c.Checkers[name] = ch
	}
}

// WithErrorHandler sets a callback for non-fatal server errors (e.g. unexpected Serve errors).
func WithErrorHandler(h func(error)) Option {
	return func(c *Config) {
		c.ErrorHandler = h
	}
}

// WithExistingGRPCServer registers the gRPC health service on s instead of starting
// a separate probe server. s must not yet be serving when this option is applied.
func WithExistingGRPCServer(s *grpc.Server) Option {
	return func(c *Config) { c.ExistingGRPCServer = s }
}

// WithExistingHTTPMux registers the /live, /ready, and /startup HTTP handlers
// on m instead of starting a separate probe server.
func WithExistingHTTPMux(m *http.ServeMux) Option {
	return func(c *Config) { c.ExistingHTTPMux = m }
}

// ApplyOptions returns a Config with all opts applied, or an error if validation fails.
func ApplyOptions(opts []Option) (Config, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 {
		return Config{}, fmt.Errorf("invalid HTTPPort %d: must be in [1, 65535]", cfg.HTTPPort)
	}
	if cfg.GRPCPort < 1 || cfg.GRPCPort > 65535 {
		return Config{}, fmt.Errorf("invalid GRPCPort %d: must be in [1, 65535]", cfg.GRPCPort)
	}
	return cfg, nil
}

// NewProbe returns a check.Server for the given config.
func NewProbe(cfg Config) check.Server {
	if cfg.ExistingGRPCServer != nil {
		return check.NewExistingGRPCProbe(cfg.ExistingGRPCServer)
	}
	if cfg.ExistingHTTPMux != nil {
		return check.NewExistingHTTPProbe(cfg.ExistingHTTPMux, cfg.CheckerTimeout, cfg.Checkers)
	}
	switch cfg.CheckMechanism {
	case CheckGRPC:
		return check.NewGRPCProbe(cfg.GRPCPort, cfg.ShutdownTimeout)
	default:
		return check.NewHTTPProbe(cfg.HTTPPort, cfg.ShutdownTimeout, cfg.CheckerTimeout, cfg.Checkers, cfg.ErrorHandler)
	}
}
