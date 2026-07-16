package workflowsync

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the workflowsync package — the
// Poller owns a sync loop, lifecycle-managed via Start/Stop. Regressions
// where Stop forgets to cancel the context or drain the WaitGroup surface
// here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
