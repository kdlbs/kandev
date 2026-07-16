package plugins

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. HealthMonitor owns a single ticker loop guarded by Start/Stop —
// goleak catches regressions where a Start path forgets to register on the
// WaitGroup, or a Stop path returns before the loop drains.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
