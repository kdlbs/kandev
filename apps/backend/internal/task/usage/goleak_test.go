package usage

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. Writer owns a single worker goroutine guarded by Start/Stop;
// goleak catches regressions where Start forgets to register on the
// WaitGroup, or Stop returns before the worker actually exits.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
