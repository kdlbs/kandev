package github

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the github package — the Poller
// owns three loops (prMonitor, reviewQueue, issueWatch) gated on a single
// context via Stop. Regressions where Stop forgets to cancel the context
// or drain the WaitGroup surface here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
