package check

import (
	"fmt"
	"net"
	"sync"

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
	port   int
	server *grpc.Server
	health *health.Server
	mu     sync.Mutex
}

// NewGRPCProbe returns a Server that implements the gRPC health protocol for services "ready", "live", "startup".
func NewGRPCProbe(port int) Server {
	return &grpcProbe{port: port}
}

func (g *grpcProbe) Start(state StateReader, onStarted func()) error {
	g.mu.Lock()
	g.health = health.NewServer()
	g.server = grpc.NewServer()
	healthpb.RegisterHealthServer(g.server, g.health)
	g.mu.Unlock()

	addr := net.JoinHostPort("", fmt.Sprintf("%d", g.port))
	if g.port <= 0 {
		addr = ":9090"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	onStarted()
	g.health.SetServingStatus(serviceStartup, healthpb.HealthCheckResponse_SERVING)
	g.syncState(state)
	go func() { _ = g.server.Serve(ln) }()
	return nil
}

func (g *grpcProbe) Shutdown() {
	g.mu.Lock()
	srv := g.server
	g.mu.Unlock()
	if srv != nil {
		srv.GracefulStop()
	}
}

func (g *grpcProbe) SetState(ready, shuttingDown bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.health == nil {
		return
	}
	if shuttingDown {
		g.health.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_NOT_SERVING)
		g.health.SetServingStatus(serviceLive, healthpb.HealthCheckResponse_NOT_SERVING)
		return
	}
	if ready {
		g.health.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_SERVING)
	} else {
		g.health.SetServingStatus(serviceReady, healthpb.HealthCheckResponse_NOT_SERVING)
	}
	g.health.SetServingStatus(serviceLive, healthpb.HealthCheckResponse_SERVING)
}

func (g *grpcProbe) syncState(state StateReader) {
	g.SetState(state.Ready(), state.ShuttingDown())
}
