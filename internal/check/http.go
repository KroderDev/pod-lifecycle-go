package check

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

type httpProbe struct {
	port   int
	server *http.Server
	mu     sync.Mutex
}

// NewHTTPProbe returns a Server that serves /ready, /live, /startup over HTTP.
func NewHTTPProbe(port int) Server {
	return &httpProbe{port: port}
}

func (h *httpProbe) Start(state StateReader, onStarted func()) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if state.Ready() && !state.ShuttingDown() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/live", func(w http.ResponseWriter, _ *http.Request) {
		if state.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/startup", func(w http.ResponseWriter, _ *http.Request) {
		if state.Started() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	h.mu.Lock()
	h.server = &http.Server{Addr: net.JoinHostPort("", portString(h.port)), Handler: mux}
	h.mu.Unlock()
	ln, err := net.Listen("tcp", h.server.Addr)
	if err != nil {
		return err
	}
	onStarted()
	go func() { _ = h.server.Serve(ln) }()
	return nil
}

func portString(port int) string {
	if port <= 0 {
		return "8080"
	}
	return fmt.Sprintf("%d", port)
}

func (h *httpProbe) Shutdown() {
	h.mu.Lock()
	srv := h.server
	h.mu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(context.Background())
	}
}

func (h *httpProbe) SetState(_, _ bool) {
	// HTTP reads state from StateReader on each request; no-op.
}
