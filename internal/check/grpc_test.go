package check_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/kroderdev/pod-lifecycle-go/internal/check"
)

// startGRPCProbe starts a gRPC probe on port and returns the address and a cleanup func.
func startGRPCProbe(t *testing.T, port int, state check.StateReader) (addr string, cleanup func()) {
	t.Helper()
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if err := probe.Start(state, func() { close(started) }); err != nil {
			errCh <- err
		}
	}()
	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("grpcProbe.Start: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("gRPC probe did not start in time")
	}
	a := fmt.Sprintf("127.0.0.1:%d", port)
	return a, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}
}

func grpcHealthClient(t *testing.T, addr string) (healthpb.HealthClient, *grpc.ClientConn) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	return healthpb.NewHealthClient(conn), conn
}

func checkStatus(t *testing.T, client healthpb.HealthClient, service string) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		t.Fatalf("health.Check(%q): %v", service, err)
	}
	return resp.Status
}

// ---- state matrix tests ----

func TestGRPCReadyBeforeSetState(t *testing.T) {
	port := freePort(t)
	addr, cleanup := startGRPCProbe(t, port, fakeState{ready: false})
	defer cleanup()

	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "ready"); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("ready before SetState: want NOT_SERVING, got %v", got)
	}
}

func TestGRPCReadyAfterSetStateTrue(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{ready: false}, func() { close(started) }) }() //nolint:errcheck
	<-started
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}()

	probe.SetState(true, false)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "ready"); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("after SetState(true,false): want SERVING, got %v", got)
	}
}

func TestGRPCReadyAfterSetStateFalse(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{ready: true}, func() { close(started) }) }() //nolint:errcheck
	<-started
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}()

	probe.SetState(false, false)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "ready"); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("after SetState(false,false): want NOT_SERVING, got %v", got)
	}
}

func TestGRPCLiveNormally(t *testing.T) {
	port := freePort(t)
	addr, cleanup := startGRPCProbe(t, port, fakeState{})
	defer cleanup()

	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "live"); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("live normally: want SERVING, got %v", got)
	}
}

func TestGRPCLiveShuttingDown(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{}, func() { close(started) }) }() //nolint:errcheck
	<-started
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}()

	probe.SetState(false, true)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "live"); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("live (shuttingDown): want NOT_SERVING, got %v", got)
	}
}

func TestGRPCStartupServing(t *testing.T) {
	port := freePort(t)
	addr, cleanup := startGRPCProbe(t, port, fakeState{})
	defer cleanup()

	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "startup"); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("startup after Start: want SERVING, got %v", got)
	}
}

func TestGRPCStartupNotServingAfterShutdownState(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{}, func() { close(started) }) }() //nolint:errcheck
	<-started
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}()

	probe.SetState(false, true)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client, conn := grpcHealthClient(t, addr)
	defer func() { _ = conn.Close() }()

	if got := checkStatus(t, client, "startup"); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("startup (shuttingDown): want NOT_SERVING, got %v", got)
	}
}

func TestGRPCSetStateBeforeStartNoPanic(t *testing.T) {
	probe := check.NewGRPCProbe(freePort(t), 5*time.Second)
	// Should not panic when called before Start.
	probe.SetState(true, false)
	probe.SetState(false, true)
}

func TestGRPCShutdownClosesListener(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{}, func() { close(started) }) }() //nolint:errcheck
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	probe.Shutdown(ctx)

	// After shutdown, dialing should fail or health check should error.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return // expected
	}
	defer func() { _ = conn.Close() }()
	client := healthpb.NewHealthClient(conn)
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer rpcCancel()
	_, err = client.Check(rpcCtx, &healthpb.HealthCheckRequest{Service: "live"})
	if err == nil {
		t.Error("expected error after Shutdown, got nil")
	}
}

func TestGRPCShutdownWithExpiredContextForcesStop(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{}, func() { close(started) }) }() //nolint:errcheck
	<-started

	// Hold a streaming connection to block GracefulStop.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		client := healthpb.NewHealthClient(conn)
		streamCtx, streamCancel := context.WithCancel(context.Background())
		defer streamCancel()
		_, _ = client.Watch(streamCtx, &healthpb.HealthCheckRequest{Service: "live"})
		defer func() { _ = conn.Close() }()
	}

	// Shutdown with an already-expired context → should call Stop().
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond) // ensure expired

	start := time.Now()
	probe.Shutdown(ctx)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown with expired ctx took too long: %v", elapsed)
	}
}

func TestGRPCConcurrentSetState(t *testing.T) {
	port := freePort(t)
	probe := check.NewGRPCProbe(port, 5*time.Second)
	started := make(chan struct{})
	go func() { probe.Start(fakeState{}, func() { close(started) }) }() //nolint:errcheck
	<-started
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		probe.Shutdown(ctx)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			probe.SetState(i%2 == 0, false)
		}()
	}
	wg.Wait()
}

// Verify the gRPC probe uses the configured port (not a hardcoded fallback).
func TestGRPCUsesConfiguredPort(t *testing.T) {
	port := freePort(t)
	addr, cleanup := startGRPCProbe(t, port, fakeState{})
	defer cleanup()

	// Try binding the same wildcard address the probe uses; it must fail.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		_ = ln.Close()
		t.Errorf("expected port %d to be in use, but Listen succeeded", port)
	}
	_ = addr
}
