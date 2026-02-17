package config

import (
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
	CheckMechanism CheckMechanism
	HTTPPort       int
	GRPCPort       int
}

func defaultConfig() Config {
	return Config{
		CheckMechanism: CheckHTTP,
		HTTPPort:       8080,
		GRPCPort:       50051,
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

// ApplyOptions returns a Config with all opts applied.
func ApplyOptions(opts []Option) Config {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// NewProbe returns a check.Server for the given config.
func NewProbe(cfg Config) check.Server {
	switch cfg.CheckMechanism {
	case CheckGRPC:
		return check.NewGRPCProbe(cfg.GRPCPort)
	default:
		return check.NewHTTPProbe(cfg.HTTPPort)
	}
}
