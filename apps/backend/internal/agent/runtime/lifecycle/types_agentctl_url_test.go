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

func TestAgentExecution_AcquireAgentCtlClientPinsReplacement(t *testing.T) {
	t.Parallel()
	client := agentctl.NewClient("127.0.0.1", 12345, newNopLogger(t))
	exec := &AgentExecution{ID: "exec-lease", agentctl: client}

	acquired, release := exec.AcquireAgentCtlClient()
	if acquired != client {
		t.Fatalf("AcquireAgentCtlClient() = %p, want %p", acquired, client)
	}
	if exec.agentctlLifecycleMu.TryLock() {
		exec.agentctlLifecycleMu.Unlock()
		release()
		t.Fatal("replacement lock acquired while client lease was active")
	}

	release()
	if !exec.agentctlLifecycleMu.TryLock() {
		t.Fatal("replacement lock remained blocked after client lease release")
	}
	exec.agentctlLifecycleMu.Unlock()
}
