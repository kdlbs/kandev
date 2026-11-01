package acp

import (
	"fmt"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

// TestMain keeps adapter frames out of the developer's real ACP log directory.
// Debug logging is enabled from the environment that runs the tests, so without
// this redirect a test run with KANDEV_DEBUG_AGENT_MESSAGES=true would rotate
// the host's session evidence away.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kandev-acp-test-logs-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated ACP test log directory: %v\n", err)
		os.Exit(1)
	}
	shared.SetDebugLogDirForTest(dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
