package lifecycle

import (
	"context"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestEnsureWorkspaceExecutionReplacesTerminalCachedExecution proves a failed
// agent execution is retired before workspace-only restoration creates a fresh
// agentctl execution. Reusing the terminal execution times out readiness.
func TestEnsureWorkspaceExecutionReplacesTerminalCachedExecution(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
				WorkspacePath: t.TempDir(), AgentID: "auggie",
			},
		},
	})
	failed := &AgentExecution{
		ID:                "execution-failed",
		SessionID:         "session-1",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		Status:            v1.AgentStatusFailed,
		agentctl:          newReadyAgentctlClient(t, newTestLogger()),
	}
	if err := mgr.executionStore.Add(failed); err != nil {
		t.Fatalf("add failed execution: %v", err)
	}

	restored, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "task-1", "session-1")
	if err != nil {
		t.Fatalf("EnsureWorkspaceExecutionForSession error = %v", err)
	}
	if restored == nil || restored.ID == failed.ID {
		t.Fatalf("restored execution = %+v, want a replacement for %q", restored, failed.ID)
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("runtime creations = %d, want 1", got)
	}
	if current, exists := mgr.executionStore.GetBySessionID("session-1"); !exists || current.ID != restored.ID {
		t.Fatalf("stored execution = %+v, exists=%v; want %q", current, exists, restored.ID)
	}
}
