package check

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type httpProbe struct {
	port            int
	shutdownTimeout time.Duration
	checkerTimeout  time.Duration
	checkers        map[string]Checker
	errHandler      func(error)
	server          *http.Server
	mu              sync.Mutex
}

// NewHTTPProbe returns a Server that serves /ready, /live, /startup over HTTP.
func NewHTTPProbe(port int, shutdownTimeout, checkerTimeout time.Duration, checkers map[string]Checker, errHandler func(error)) Server {
	return &httpProbe{
		port:            port,
		shutdownTimeout: shutdownTimeout,
		checkerTimeout:  checkerTimeout,
		checkers:        checkers,
		errHandler:      errHandler,
	}
}

func (h *httpProbe) Start(state StateReader, onStarted func()) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", onlyGET(h.readyHandler(state)))
	mux.HandleFunc("/live", onlyGET(func(w http.ResponseWriter, _ *http.Request) {
		if state.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/startup", onlyGET(func(w http.ResponseWriter, _ *http.Request) {
		if state.Started() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	srv := &http.Server{
		Addr:         net.JoinHostPort("", fmt.Sprintf("%d", h.port)),
		Handler:      mux,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	h.mu.Lock()
	h.server = srv
	h.mu.Unlock()

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	onStarted()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			if h.errHandler != nil {
				h.errHandler(err)
			}
		}
	}()
	return nil
}

func (h *httpProbe) readyHandler(state StateReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !state.Ready() || state.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if len(h.checkers) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		results := h.runCheckers(r.Context())
		allOK := true
		for _, v := range results {
			if v != "ok" {
				allOK = false
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if allOK {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(results)
	}
}

func (h *httpProbe) runCheckers(reqCtx context.Context) map[string]string {
	type result struct {
		name string
		val  string
	}
	ch := make(chan result, len(h.checkers))
	for name, c := range h.checkers {
		name, c := name, c
		go func() {
			ctx, cancel := context.WithTimeout(reqCtx, h.checkerTimeout)
			defer cancel()
			if err := c.Check(ctx); err != nil {
				ch <- result{name, "error: " + err.Error()}
			} else {
				ch <- result{name, "ok"}
			}
		}()
	}
	out := make(map[string]string, len(h.checkers))
	for range h.checkers {
		r := <-ch
		out[r.name] = r.val
	}
	return out
}

// onlyGET wraps a handler to return 405 for non-GET methods.
func onlyGET(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func (h *httpProbe) Shutdown(ctx context.Context) {
	h.mu.Lock()
	srv := h.server
	h.mu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
}

func (h *httpProbe) SetState(_, _ bool) {
	// HTTP reads state from StateReader on each request; no-op.
}
