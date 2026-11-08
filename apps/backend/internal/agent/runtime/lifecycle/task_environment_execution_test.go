package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type primarySessionWorkspaceProvider struct {
	*mockWorkspaceInfoProvider
	primary *models.TaskSession
}

func (p *primarySessionWorkspaceProvider) GetPrimarySession(context.Context, string) (*models.TaskSession, error) {
	return p.primary, nil
}

func newSharedEnvironmentOwnerManager(t *testing.T, child *models.TaskSession) (*Manager, *createInstanceExecutor) {
	t.Helper()
	provider := &primarySessionWorkspaceProvider{
		mockWorkspaceInfoProvider: &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			child.ID: {
				TaskID: child.TaskID, SessionID: child.ID, TaskEnvironmentID: "env-shared",
				WorkspacePath: "/workspace/shared", AgentID: "auggie",
			},
		}},
		primary: child,
	}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)
	mgr.executorProfileReader = &fakeExecutorProfileReader{
		session: child,
		env: &models.TaskEnvironment{
			ID: "env-shared", TaskID: "task-parent", Status: models.TaskEnvironmentStatusReady,
		},
	}
	parent := &AgentExecution{
		ID: "exec-parent", TaskID: "task-parent", SessionID: "session-parent",
		TaskEnvironmentID: "env-shared", Status: v1.AgentStatusRunning,
		agentctl: newReadyAgentctlClient(t, newTestLogger()),
	}
	if err := mgr.executionStore.Add(parent); err != nil {
		t.Fatalf("add parent execution: %v", err)
	}
	return mgr, backend
}

func TestGetOrEnsureExecutionForTaskEnvironmentDoesNotBorrowSiblingExecution(t *testing.T) {
	child := &models.TaskSession{ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-shared"}
	mgr, backend := newSharedEnvironmentOwnerManager(t, child)

	got, err := mgr.GetOrEnsureExecutionForTaskEnvironment(context.Background(), "task-child", "env-shared")
	if err != nil {
		t.Fatalf("GetOrEnsureExecutionForTaskEnvironment: %v", err)
	}
	if got.TaskID != "task-child" || got.SessionID != "session-child" || got.TaskEnvironmentID != "env-shared" {
		t.Fatalf("execution owner = %s/%s/%s, want task-child/session-child/env-shared",
			got.TaskID, got.SessionID, got.TaskEnvironmentID)
	}
	if backend.createCount.Load() != 1 {
		t.Fatalf("CreateInstance calls = %d, want 1 for the owner session", backend.createCount.Load())
	}

	again, err := mgr.GetOrEnsureExecutionForTaskEnvironment(context.Background(), "task-child", "env-shared")
	if err != nil {
		t.Fatalf("second GetOrEnsureExecutionForTaskEnvironment: %v", err)
	}
	if again.ID != got.ID || backend.createCount.Load() != 1 {
		t.Fatalf("reopen execution = %s (creates %d), want cached %s", again.ID, backend.createCount.Load(), got.ID)
	}
}
