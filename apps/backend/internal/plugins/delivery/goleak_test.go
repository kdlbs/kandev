package delivery

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. Every pluginWorker's run loop is started by Deliverer.Refresh
// and must be stopped via Deliverer.Stop (or Refresh dropping the plugin) —
// goleak catches a regression where a worker is created without a matching
// stop path.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
