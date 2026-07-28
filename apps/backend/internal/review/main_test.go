package review

import (
	"testing"

	"go.uber.org/goleak"
)

// Runner owns background review passes, so the package is goroutine-leak
// instrumented per apps/backend/AGENTS.md. A pass that outlived Stop would keep
// calling a provider after shutdown.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
