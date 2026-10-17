package lsp

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that controller workers, task-host watches, recovery
// timers, and capacity promotion goroutines are joined by their owners.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
