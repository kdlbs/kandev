package shared

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the shared transport tests under goleak so the ACP debug-log
// janitor's background goroutine is verified leak-free (Start/Stop drains it).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
