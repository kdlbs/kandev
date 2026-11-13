package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
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

// TestEnsureWorkspaceExecutionKeepsTerminalExecutionWhenStopFails proves a
// failed stop keeps the retained execution instead of creating a duplicate.
func TestEnsureWorkspaceExecutionKeepsTerminalExecutionWhenStopFails(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
				WorkspacePath: t.TempDir(), AgentID: "auggie",
			},
		},
	})
	backend.stopErr = errors.New("stop failed")
	failed := &AgentExecution{
		ID: "execution-failed", SessionID: "session-1", TaskID: "task-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeStandalone, Status: v1.AgentStatusFailed,
		agentctl: newReadyAgentctlClient(t, newTestLogger()),
	}
	if err := mgr.executionStore.Add(failed); err != nil {
		t.Fatalf("add failed execution: %v", err)
	}
	if _, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), "task-1", "session-1"); err == nil {
		t.Fatal("EnsureWorkspaceExecutionForSession error = nil, want stale-stop failure")
	}
	if got := backend.stopCount.Load(); got != 1 {
		t.Fatalf("StopInstance calls = %d, want 1", got)
	}
	if current, exists := mgr.executionStore.GetBySessionID("session-1"); !exists || current.ID != failed.ID {
		t.Fatalf("stored execution = %+v, exists=%v; want retained %q", current, exists, failed.ID)
	}
	if got := backend.createCount.Load(); got != 0 {
		t.Fatalf("runtime creations = %d, want 0", got)
	}
}

// TestEnsureWorkspaceExecutionReplacesTerminalExecutionOnceUnderConcurrency
// proves concurrent ensures coalesce around one replacement.
func TestEnsureWorkspaceExecutionReplacesTerminalExecutionOnceUnderConcurrency(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
		infos: map[string]*WorkspaceInfo{
			"session-1": {
				TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1",
				WorkspacePath: t.TempDir(), AgentID: "auggie",
			},
		},
	})
	failed := &AgentExecution{
		ID: "execution-failed", SessionID: "session-1", TaskID: "task-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeStandalone, Status: v1.AgentStatusFailed,
		agentctl: newReadyAgentctlClient(t, newTestLogger()),
	}
	if err := mgr.executionStore.Add(failed); err != nil {
		t.Fatalf("add failed execution: %v", err)
	}

	const callers = 2
	results := make([]*AgentExecution, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = mgr.EnsureWorkspaceExecutionForSession(
				context.Background(), "task-1", "session-1",
			)
		}(i)
	}
	wg.Wait()
	for index := range results {
		if errs[index] != nil {
			t.Fatalf("caller %d error = %v", index, errs[index])
		}
		if results[index] == nil || results[index].ID == failed.ID {
			t.Fatalf("caller %d execution = %+v, want replacement", index, results[index])
		}
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("replacement IDs = %q and %q, want one coalesced execution", results[0].ID, results[1].ID)
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("runtime creations = %d, want 1", got)
	}
}
