package reachability

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. The Poller owns the scheduled loop plus any off-cycle ProbeNow
// call, all registered on one WaitGroup — goleak catches a Start path that
// forgets to register, or a Stop path that returns before every probe drains.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
