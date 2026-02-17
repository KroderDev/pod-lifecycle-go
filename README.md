# pod-lifecycle-go

Kubernetes-native pod lifecycle management for Go microservices. Coordinates with kubelet signals and probes so your pod behaves as a good citizen: graceful shutdown, correct readiness, liveness, and startup semantics.

## Features

- **Graceful shutdown** — On `SIGTERM`, stops accepting new work, drains in-flight work, then exits. The process must exit within the pod’s `terminationGracePeriodSeconds` or Kubernetes sends `SIGKILL`. Maps to `terminationGracePeriodSeconds` and kubelet’s shutdown sequence.
- **Readiness** — Mark the pod ready only when your app has finished startup (e.g. DB connected, caches warm). The library exposes an endpoint that returns success only when ready; the pod is removed from Service endpoints while draining. Maps to `readinessProbe` and Service Endpoints.
- **Liveness** — Endpoint that reports the process is alive and making progress. Repeated failure causes the kubelet to restart the container. Should not depend on external deps (e.g. DB) to avoid restart loops. Maps to `livenessProbe`.
- **Startup** — Separate probe for “container has started.” Until it succeeds, liveness is not counted as failed, so slow starters are not killed during initialisation. Maps to `startupProbe`.
- **PreStop / grace period** — Works with `lifecycle.preStop` (e.g. `sleep 5` for ingress drain): the app still receives `SIGTERM` after the container is asked to stop; graceful drain runs in that window. Set `terminationGracePeriodSeconds` to at least (preStop delay + your drain time).
- **HTTP or gRPC probes** — Choose the check mechanism when creating the manager: `httpGet` (paths `/ready`, `/live`, `/startup`) or `grpc` (service names `ready`, `live`, `startup`). Use `WithCheckMechanism(podlifecycle.CheckHTTP)` or `WithCheckMechanism(podlifecycle.CheckGRPC)`.

## How it maps to Kubernetes

| Library behavior | Kubernetes concept | Effect |
| -----------------|--------------------|--------|
| Handle `SIGTERM`, drain, exit | `terminationGracePeriodSeconds` | Exit within grace period or receive `SIGKILL` |
| Readiness endpoint | `readinessProbe` | Probe success → pod in Service Endpoints; failure → pod removed (no restart) |
| Liveness endpoint | `livenessProbe` | Repeated failure → container restart |
| Startup endpoint | `startupProbe` | Until success, liveness failures are ignored; protects slow starters |
| Graceful shutdown | `lifecycle.preStop` + grace period | PreStop runs first (e.g. drain connections); then `SIGTERM`; app must exit within grace period |

## Probe paths and mechanism

- **HTTP** (default): paths `/ready`, `/live`, `/startup` on the configured port (default 8080). Use `httpGet` in your probe definitions.
- **gRPC**: gRPC health protocol with service names `ready`, `live`, `startup` on the configured port (default 50051). Use `grpc` in your probe definitions.

Ports are configurable via `WithHTTPPort(port)` and `WithGRPCPort(port)`.

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

## Installation

```bash
go get github.com/kroderdev/pod-lifecycle-go@latest
```

## Usage

Create a `PodManager` with the desired check mechanism (HTTP or gRPC), then call `Start()` to run the probe server and block until SIGTERM/SIGINT. Call `SetReady()` once your app is ready (e.g. DB connected, caches warm).

**HTTP probes (default, port 8080):**

```go
package main

import (
	"github.com/kroderdev/pod-lifecycle-go"
)

func main() {
	pm := podlifecycle.NewPodManager()
	// Optional: podlifecycle.WithHTTPPort(8080) is the default
	go func() {
		// When DB connected, caches warm, etc.:
		// pm.SetReady()
	}()
	_ = pm.Start()
}
```

**gRPC probes (port 50051):**

```go
pm := podlifecycle.NewPodManager(
	podlifecycle.WithCheckMechanism(podlifecycle.CheckGRPC),
	podlifecycle.WithGRPCPort(50051), // optional, 50051 is default
)
go func() { /* ... */ pm.SetReady() }()
_ = pm.Start()
```

**Minimal (HTTP, no SetReady):** `podlifecycle.Start()` runs a default HTTP manager and blocks until signal.
