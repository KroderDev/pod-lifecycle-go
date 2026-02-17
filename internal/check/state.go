package check

// StateReader provides read-only access to pod lifecycle state for probe handlers.
type StateReader interface {
	Ready() bool
	ShuttingDown() bool
	Started() bool
}
