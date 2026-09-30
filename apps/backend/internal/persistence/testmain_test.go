package persistence

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wraps the persistence package's test suite in goleak so the
// backup-progress ticker goroutine (trackBackupProgress) is verified to
// join on stop. Matches the goleak convention already used by
// internal/gateway/websocket, agentctl/server/process, and the runtime
// lifecycle package.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
