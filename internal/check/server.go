package check

import "context"

// Server is the interface for HTTP or gRPC probe implementations.
type Server interface {
	// Start starts the probe server. onStarted is called when the server is listening.
	Start(state StateReader, onStarted func()) error
	Shutdown(ctx context.Context)
	SetState(ready, shuttingDown bool)
}
