package delivery

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wraps internal/delivery's test suite in goleak so Sweep.Start's
// background loop goroutine is verified to drain on Stop, following the
// same convention as internal/gateway/websocket, internal/agentctl/server/
// process, and the runtime lifecycle package (Review round 2, finding #4;
// docs/plans/task-delivery-ledger/task-06-sweep-and-health.md commits to
// this coverage).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
