package check

import "context"

// Checker reports the health of an external dependency.
// Implementations must be safe for concurrent use.
type Checker interface {
	Check(ctx context.Context) error
}
