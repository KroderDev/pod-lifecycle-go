package podlifecycle_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	podlifecycle "github.com/kroderdev/pod-lifecycle-go"
)

// ---- checker spy ----

type spyChecker struct {
	mu    sync.Mutex
	calls int
}

func (s *spyChecker) Check(_ context.Context) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

func (s *spyChecker) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ---- helpers ----

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func doGET(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// ---- tests ----

func TestNewPodManagerInvalidPort(t *testing.T) {
	_, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(0))
	if err == nil {
		t.Error("expected error for port 0, got nil")
	}
}

func TestNewPodManagerValidReturnsNonNil(t *testing.T) {
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(freePort(t)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm == nil {
		t.Fatal("expected non-nil PodManager")
	}
}

func TestInitialStateAllFalse(t *testing.T) {
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(freePort(t)))
	if err != nil {
		t.Fatal(err)
	}
	if pm.Ready() {
		t.Error("Ready() should be false initially")
	}
	if pm.IsShuttingDown() {
		t.Error("IsShuttingDown() should be false initially")
	}
	if pm.Started() {
		t.Error("Started() should be false initially")
	}
}

func TestSetReadyUpdatesState(t *testing.T) {
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(freePort(t)))
	if err != nil {
		t.Fatal(err)
	}
	pm.SetReady()
	if !pm.Ready() {
		t.Error("Ready() should be true after SetReady()")
	}
}

func TestStartContextStartsProbe(t *testing.T) {
	port := freePort(t)
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(port))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- pm.StartContext(ctx)
	}()

	// Wait for probe to start.
	time.Sleep(100 * time.Millisecond)

	// /live should be reachable.
	if got := doGET(t, fmt.Sprintf("http://127.0.0.1:%d/live", port)); got != http.StatusOK {
		t.Errorf("/live want 200, got %d", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartContext did not return after cancel")
	}
	if !pm.IsShuttingDown() {
		t.Error("IsShuttingDown() should be true after cancel")
	}
}

func TestReadyEndpointAfterSetReady(t *testing.T) {
	port := freePort(t)
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(port))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pm.StartContext(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	if got := doGET(t, fmt.Sprintf("http://127.0.0.1:%d/ready", port)); got != http.StatusServiceUnavailable {
		t.Errorf("/ready before SetReady: want 503, got %d", got)
	}

	pm.SetReady()

	if got := doGET(t, fmt.Sprintf("http://127.0.0.1:%d/ready", port)); got != http.StatusOK {
		t.Errorf("/ready after SetReady: want 200, got %d", got)
	}
}

func TestWithCheckerSpyCalled(t *testing.T) {
	port := freePort(t)
	spy := &spyChecker{}
	pm, err := podlifecycle.NewPodManager(
		podlifecycle.WithHTTPPort(port),
		podlifecycle.WithChecker("db", spy),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pm.StartContext(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)
	pm.SetReady()

	doGET(t, fmt.Sprintf("http://127.0.0.1:%d/ready", port))

	if spy.Calls() == 0 {
		t.Error("expected checker to be called at least once")
	}
}

func TestPortAlreadyInUseReturnsError(t *testing.T) {
	port := freePort(t)
	// Hold the port on all interfaces (same bind as the HTTP probe uses).
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("failed to hold port: %v", err)
	}
	defer ln.Close()

	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(port))
	if err != nil {
		t.Fatal(err)
	}
	if err := pm.Start(); err == nil {
		t.Error("expected error when port is in use, got nil")
	}
}

func TestConcurrentSetReadyAndIsShuttingDown(t *testing.T) {
	port := freePort(t)
	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(port))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			pm.SetReady()
		}()
		go func() {
			defer wg.Done()
			_ = pm.IsShuttingDown()
		}()
	}
	wg.Wait()
}

func TestShutdownTimeoutCompletes(t *testing.T) {
	port := freePort(t)
	pm, err := podlifecycle.NewPodManager(
		podlifecycle.WithHTTPPort(port),
		podlifecycle.WithShutdownTimeout(10*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- pm.StartContext(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	start := time.Now()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("shutdown did not complete within 100ms bound")
	}
	_ = start
}

func TestStartContextPortInUseReturnsError(t *testing.T) {
	port := freePort(t)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("failed to hold port: %v", err)
	}
	defer ln.Close()

	pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(port))
	if err != nil {
		t.Fatal(err)
	}
	if err := pm.StartContext(context.Background()); err == nil {
		t.Error("expected error when port is in use, got nil")
	}
}

