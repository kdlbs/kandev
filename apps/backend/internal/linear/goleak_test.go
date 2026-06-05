package linear

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the linear package — the Poller
// owns an auth-health loop and an issue-watch loop, both lifecycle-managed
// via Start/Stop. Regressions where Stop forgets to cancel either context
// or drain the WaitGroup surface here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
