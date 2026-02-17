package podlifecycle

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/kroderdev/pod-lifecycle-go/internal/check"
	"github.com/kroderdev/pod-lifecycle-go/internal/config"
)

// Re-export config types and options for consumers.
type (
	CheckMechanism = config.CheckMechanism
	Option         = config.Option
)

const (
	CheckHTTP = config.CheckHTTP
	CheckGRPC = config.CheckGRPC
)

var (
	WithCheckMechanism = config.WithCheckMechanism
	WithHTTPPort       = config.WithHTTPPort
	WithGRPCPort       = config.WithGRPCPort
)

// PodManager coordinates pod lifecycle: signals, readiness, liveness, and startup probes.
type PodManager struct {
	ready        atomic.Bool
	shuttingDown atomic.Bool
	started      atomic.Bool
	probe        check.Server
}

func (pm *PodManager) Ready() bool        { return pm.ready.Load() }
func (pm *PodManager) ShuttingDown() bool { return pm.shuttingDown.Load() }
func (pm *PodManager) Started() bool      { return pm.started.Load() }

// NewPodManager creates a PodManager with the given options.
func NewPodManager(opts ...Option) *PodManager {
	cfg := config.ApplyOptions(opts)
	return &PodManager{probe: config.NewProbe(cfg)}
}

// SetReady marks the pod as ready. Call once your app has finished startup.
func (pm *PodManager) SetReady() {
	pm.ready.Store(true)
	pm.probe.SetState(true, pm.shuttingDown.Load())
}

// IsShuttingDown returns true after a termination signal has been received.
func (pm *PodManager) IsShuttingDown() bool {
	return pm.shuttingDown.Load()
}

// Start starts the probe server and blocks until SIGTERM or SIGINT.
func (pm *PodManager) Start() error {
	if err := pm.probe.Start(pm, func() { pm.started.Store(true) }); err != nil {
		return err
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	signal.Stop(sigCh)
	pm.shuttingDown.Store(true)
	pm.probe.SetState(pm.ready.Load(), true)
	pm.probe.Shutdown()
	return nil
}

// StartContext is like Start but returns when ctx is cancelled.
func (pm *PodManager) StartContext(ctx context.Context) error {
	if err := pm.probe.Start(pm, func() { pm.started.Store(true) }); err != nil {
		return err
	}
	<-ctx.Done()
	pm.shuttingDown.Store(true)
	pm.probe.SetState(pm.ready.Load(), true)
	pm.probe.Shutdown()
	return ctx.Err()
}

// Start runs a default HTTP PodManager and blocks until SIGTERM/SIGINT.
func Start() error {
	return NewPodManager().Start()
}
