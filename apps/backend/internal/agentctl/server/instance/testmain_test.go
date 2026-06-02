package instance

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wraps the instance package's test suite in goleak so the idle
// reaper goroutine (and any future long-lived helper) is verified to
// drain on Shutdown. Matches the goleak convention already used by
// internal/gateway/websocket, agentctl/server/process, and the runtime
// lifecycle package.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
