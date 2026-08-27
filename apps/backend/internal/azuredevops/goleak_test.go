package azuredevops

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the azuredevops package. The
// Poller owns an auth-health loop and a watcher loop, both lifecycle-managed
// via Start/Stop; regressions where Stop forgets to cancel either context or
// drain the WaitGroup surface here, as do watcher "initial check" goroutines
// that outlive their service.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
