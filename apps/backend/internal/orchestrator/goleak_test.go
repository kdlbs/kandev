package orchestrator

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the orchestrator package.
// Event handlers in event_handlers_*.go spawn detached goroutines for
// queued message execution, cleanup, watchdogs, and step-exit/enter
// processing — each must terminate on its own (context cancellation or
// natural completion). Regressions where a handler forgets a return on
// ctx.Done() or hangs on a buffered channel surface here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
