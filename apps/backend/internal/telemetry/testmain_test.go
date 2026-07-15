package telemetry

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts no goroutines from this package outlive the tests.
// The Service owns exactly two loops (flush, heartbeat) guarded by
// Start/Stop; goleak catches any path that forgets the WaitGroup.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
