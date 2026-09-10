package retention

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the retention package.
// Scheduler.Start spawns a single lifecycle-managed loop goroutine; Stop
// cancels its context and waits on the WaitGroup. Regressions where Stop
// forgets to cancel or a test leaves a scheduler running surface here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
