# pod-lifecycle-go

Kubernetes-native pod lifecycle management for Go microservices. Coordinates with kubelet signals and probes so your pod behaves as a good citizen: graceful shutdown, correct readiness, liveness, and startup semantics.

## Features

- **Graceful shutdown** — On `SIGTERM`, stops accepting new work, drains in-flight work, then exits within a configurable timeout. The process must exit within the pod's `terminationGracePeriodSeconds` or Kubernetes sends `SIGKILL`. Maps to `terminationGracePeriodSeconds` and kubelet's shutdown sequence.
- **Readiness** — Mark the pod ready only when your app has finished startup (e.g. DB connected, caches warm). The library exposes an endpoint that returns success only when ready; the pod is removed from Service endpoints while draining. Maps to `readinessProbe` and Service Endpoints.
- **Liveness** — Endpoint that reports the process is alive and making progress. Repeated failure causes the kubelet to restart the container. Never depends on external deps (e.g. DB) to avoid restart loops. Maps to `livenessProbe`.
- **Startup** — Separate probe for "container has started." Until it succeeds, liveness is not counted as failed, so slow starters are not killed during initialisation. Maps to `startupProbe`.
- **Dependency health checks** — Register named `Checker` implementations (e.g. a DB ping) via `WithChecker`. They run in parallel on every `/ready` request with a configurable timeout. Any failure → 503 + JSON body with per-checker status. Only wired to `/ready` — never to `/live` — so a DB outage drains traffic without triggering pod restarts.
- **HTTP or gRPC probes** — Choose the check mechanism when creating the manager: `httpGet` (paths `/ready`, `/live`, `/startup`) or `grpc` (service names `ready`, `live`, `startup`).
- **Server hardening** — HTTP server is configured with `ReadTimeout: 2s`, `WriteTimeout: 2s`, `IdleTimeout: 60s`. Non-GET requests to probe endpoints return 405.
- **PreStop / grace period** — Works with `lifecycle.preStop` (e.g. `sleep 5` for ingress drain): the app still receives `SIGTERM` after the container is asked to stop; graceful drain runs in that window. Set `terminationGracePeriodSeconds` to at least (preStop delay + your drain time).

## How it maps to Kubernetes

| Library behavior | Kubernetes concept | Effect |
| -----------------|--------------------|--------|
| Handle `SIGTERM`, drain, exit | `terminationGracePeriodSeconds` | Exit within grace period or receive `SIGKILL` |
| Readiness endpoint | `readinessProbe` | Probe success → pod in Service Endpoints; failure → pod removed (no restart) |
| Liveness endpoint | `livenessProbe` | Repeated failure → container restart |
| Startup endpoint | `startupProbe` | Until success, liveness failures are ignored; protects slow starters |
| Graceful shutdown | `lifecycle.preStop` + grace period | PreStop runs first (e.g. drain connections); then `SIGTERM`; app must exit within grace period |
| Dependency checkers on `/ready` only | Readiness semantics | DB down → 503 on `/ready` → pod removed from endpoints (no restart loop) |

## Probe paths and mechanism

- **HTTP** (default): paths `/ready`, `/live`, `/startup` on the configured port (default 8080). Use `httpGet` in your probe definitions.
- **gRPC**: gRPC health protocol with service names `ready`, `live`, `startup` on the configured port (default 50051). Use `grpc` in your probe definitions.

Ports are configurable via `WithHTTPPort(port)` and `WithGRPCPort(port)` and are validated to be in `[1, 65535]`.

## Installation

```bash
go get github.com/kroderdev/pod-lifecycle-go@latest
```

## Usage

`NewPodManager` returns `(*PodManager, error)` — check the error for invalid configuration (e.g. port out of range).

**HTTP probes (default, port 8080):**

```go
package main

import (
    "log"
    podlifecycle "github.com/kroderdev/pod-lifecycle-go"
)

func main() {
    pm, err := podlifecycle.NewPodManager()
    if err != nil {
        log.Fatal(err)
    }
    go func() {
        // When DB connected, caches warm, etc.:
        pm.SetReady()
    }()
    if err := pm.Start(); err != nil {
        log.Fatal(err)
    }
}
```

**gRPC probes (port 50051):**

```go
pm, err := podlifecycle.NewPodManager(
    podlifecycle.WithCheckMechanism(podlifecycle.CheckGRPC),
    podlifecycle.WithGRPCPort(50051), // optional, 50051 is default
)
if err != nil {
    log.Fatal(err)
}
go func() { /* ... */ pm.SetReady() }()
_ = pm.Start()
```

**Minimal (HTTP, no SetReady):** `podlifecycle.Start()` runs a default HTTP manager and blocks until signal.

**Context-based shutdown:**

```go
pm, err := podlifecycle.NewPodManager(podlifecycle.WithHTTPPort(8080))
if err != nil {
    log.Fatal(err)
}
// Returns when ctx is cancelled; IsShuttingDown() is true afterwards.
err = pm.StartContext(ctx)
```

**Dependency health checks on `/ready`:**

```go
type DBChecker struct{ db *sql.DB }

func (c *DBChecker) Check(ctx context.Context) error {
    return c.db.PingContext(ctx)
}

pm, err := podlifecycle.NewPodManager(
    podlifecycle.WithChecker("postgres", &DBChecker{db: db}),
    podlifecycle.WithChecker("redis",    &RedisChecker{rdb: rdb}),
    podlifecycle.WithCheckerTimeout(2 * time.Second),   // per-checker deadline (default 2s)
    podlifecycle.WithShutdownTimeout(5 * time.Second),  // probe drain timeout (default 5s)
)
```

`/ready` with checkers returns JSON:

```json
{"postgres": "ok", "redis": "error: connection refused"}
```

503 if any checker fails; 200 if all pass.

## Configuration options

| Option | Default | Description |
|--------|---------|-------------|
| `WithCheckMechanism(m)` | `CheckHTTP` | Probe mechanism: `CheckHTTP` or `CheckGRPC` |
| `WithHTTPPort(port)` | `8080` | HTTP probe port `[1, 65535]` |
| `WithGRPCPort(port)` | `50051` | gRPC probe port `[1, 65535]` |
| `WithShutdownTimeout(d)` | `5s` | Max time to drain probe servers on shutdown |
| `WithCheckerTimeout(d)` | `2s` | Per-checker deadline on each `/ready` request |
| `WithChecker(name, c)` | — | Register a named dependency checker |
| `WithErrorHandler(fn)` | — | Callback for non-fatal probe server errors |

## Example Deployment (HTTP probes)

```yaml
spec:
  terminationGracePeriodSeconds: 45
  containers:
    - name: app
      # ... image, ports, etc.
      lifecycle:
        preStop:
          exec:
            command: ["sleep", "5"]
      readinessProbe:
        httpGet:
          path: /ready
          port: 8080
        initialDelaySeconds: 2
        periodSeconds: 5
      livenessProbe:
        httpGet:
          path: /live
          port: 8080
        initialDelaySeconds: 5
        periodSeconds: 10
      startupProbe:
        httpGet:
          path: /startup
          port: 8080
        failureThreshold: 30
        periodSeconds: 2
```

## Example Deployment (gRPC probes)

```yaml
spec:
  terminationGracePeriodSeconds: 45
  containers:
    - name: app
      # ... image, ports, etc.
      readinessProbe:
        grpc:
          port: 50051
          service: ready
        initialDelaySeconds: 2
        periodSeconds: 5
      livenessProbe:
        grpc:
          port: 50051
          service: live
        initialDelaySeconds: 5
        periodSeconds: 10
      startupProbe:
        grpc:
          port: 50051
          service: startup
        failureThreshold: 30
        periodSeconds: 2
```
