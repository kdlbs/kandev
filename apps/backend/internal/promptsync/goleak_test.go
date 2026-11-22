package promptsync

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces that the Poller's loop is drained by Stop.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
