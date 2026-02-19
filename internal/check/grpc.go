package check

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	serviceReady   = "ready"
	serviceLive    = "live"
	serviceStartup = "startup"
)

type grpcProbe struct {
	port            int
	shutdownTimeout time.Duration
	server          *grpc.Server
	health          *health.Server
	mu              sync.Mutex
}

// NewGRPCProbe returns a Server that implements the gRPC health protocol for services "ready", "live", "startup".
func NewGRPCProbe(port int, shutdownTimeout time.Duration) Server {
	return &grpcProbe{port: port, shutdownTimeout: shutdownTimeout}
}

func (g *grpcProbe) Start(state StateReader, onStarted func()) error {
	g.mu.Lock()
	g.health = health.NewServer()
	g.server = grpc.NewServer()
	healthpb.RegisterHealthServer(g.server, g.health)
	g.mu.Unlock()

	addr := net.JoinHostPort("", fmt.Sprintf("%d", g.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	onStarted()
	g.health.SetServingStatus(serviceStartup, healthpb.HealthCheckResponse_SERVING)
	g.applyState(g.health, state.Ready(), state.ShuttingDown())
	go func() { _ = g.server.Serve(ln) }()
	return nil
}

// applyState sets gRPC health statuses without acquiring the lock.
// Must be called with g.mu held OR before Start returns (single-goroutine context).
func applyState(hs *health.Server, ready, shuttingDown bool) {
	if shuttingDown {
		hs.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_NOT_SERVING)
		hs.SetServingStatus(serviceLive, healthpb.HealthCheckResponse_NOT_SERVING)
		hs.SetServingStatus(serviceStartup, healthpb.HealthCheckResponse_NOT_SERVING)
		return
	}
	if ready {
		hs.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_SERVING)
	} else {
		hs.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_NOT_SERVING)
	}
	hs.SetServingStatus(serviceLive, healthpb.HealthCheckResponse_SERVING)
}

func (g *grpcProbe) applyState(hs *health.Server, ready, shuttingDown bool) {
	applyState(hs, ready, shuttingDown)
}

func (g *grpcProbe) SetState(ready, shuttingDown bool) {
	g.mu.Lock()
	hs := g.health
	g.mu.Unlock()
	if hs == nil {
		return
	}
	applyState(hs, ready, shuttingDown)
}

func (g *grpcProbe) Shutdown(ctx context.Context) {
	g.mu.Lock()
	srv := g.server
	g.mu.Unlock()
	if srv == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		srv.Stop()
	}
}

// ---------------------------------------------------------------------------
// existingGRPCProbe — shared-port strategy
// ---------------------------------------------------------------------------

// existingGRPCProbe implements Server by registering the gRPC health service
// on a caller-owned *grpc.Server instead of starting a new one. Registration
// happens at construction time so callers can safely call s.Serve afterwards
// without a registration race.
//
// On Shutdown the health statuses are set to NOT_SERVING, but the underlying
// server is NOT stopped — the caller owns the server and is responsible for
// calling GracefulStop.
type existingGRPCProbe struct {
	health *health.Server
	mu     sync.Mutex
}

// NewExistingGRPCProbe creates a Server that registers gRPC health on s.
// s must not yet be serving when NewExistingGRPCProbe is called.
func NewExistingGRPCProbe(s *grpc.Server) Server {
	hs := health.NewServer()
	healthpb.RegisterHealthServer(s, hs)
	return &existingGRPCProbe{health: hs}
}

func (e *existingGRPCProbe) Start(state StateReader, onStarted func()) error {
	// No new server to start — health is pre-registered on the caller's server.
	onStarted()
	e.mu.Lock()
	hs := e.health
	e.mu.Unlock()
	hs.SetServingStatus(serviceStartup, healthpb.HealthCheckResponse_SERVING)
	applyState(hs, state.Ready(), state.ShuttingDown())
	return nil
}

func (e *existingGRPCProbe) SetState(ready, shuttingDown bool) {
	e.mu.Lock()
	hs := e.health
	e.mu.Unlock()
	applyState(hs, ready, shuttingDown)
}

func (e *existingGRPCProbe) Shutdown(_ context.Context) {
	e.mu.Lock()
	hs := e.health
	e.mu.Unlock()
	// Mark all health services NOT_SERVING so load-balancers stop routing.
	// The caller is responsible for stopping the gRPC server itself.
	hs.Shutdown()
}
