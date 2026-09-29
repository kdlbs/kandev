package coordinator

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. The approval sweep owns a single ticker loop guarded by
// startApprovalSweep/ctx cancellation — goleak catches a regression where a
// start path forgets to register on sweepWG, or a stop path returns before
// the loop drains.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
