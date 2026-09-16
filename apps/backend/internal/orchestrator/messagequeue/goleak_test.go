package messagequeue

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks in this package. StartDeliveryRecovery
// spawns a long-running bounded-retry goroutine with its own owned lifecycle
// (StopDeliveryRecovery cancels and joins it); a test that starts the worker
// without stopping it, or a future shutdown regression, surfaces here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
