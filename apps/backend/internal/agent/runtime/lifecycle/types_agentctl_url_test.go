package lifecycle

import (
	"fmt"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

func newNopLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestAgentExecution_AgentctlURL_NilClient(t *testing.T) {
	t.Parallel()
	exec := &AgentExecution{ID: "exec-1"}
	if got := exec.AgentctlURL(); got != "" {
		t.Errorf("expected empty string when no client set, got %q", got)
	}
}

func TestAgentExecution_AgentctlURL_WithClient(t *testing.T) {
	t.Parallel()
	log := newNopLogger(t)
	client := agentctl.NewClient("127.0.0.1", 12345, log)
	exec := &AgentExecution{
		ID:       "exec-2",
		agentctl: client,
	}
	want := fmt.Sprintf("http://%s:%d", "127.0.0.1", 12345)
	if got := exec.AgentctlURL(); got != want {
		t.Errorf("AgentctlURL() = %q, want %q", got, want)
	}
}
