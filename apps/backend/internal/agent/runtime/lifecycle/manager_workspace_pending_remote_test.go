package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceAccessDoesNotReplacePendingRemoteRecovery(t *testing.T) {
	paths := map[string]func(*Manager) (*AgentExecution, error){
		"session": func(m *Manager) (*AgentExecution, error) {
			return m.GetOrEnsureExecution(context.Background(), "session-pending")
		},
		"environment": func(m *Manager) (*AgentExecution, error) {
			return m.GetOrEnsureExecutionForEnvironment(context.Background(), "env-pending")
		},
		"explicit session": func(m *Manager) (*AgentExecution, error) {
			return m.EnsureWorkspaceExecutionForSession(context.Background(), "task-pending", "session-pending")
		},
	}
	for name, access := range paths {
		t.Run(name, func(t *testing.T) {
			info := &WorkspaceInfo{
				TaskID: "task-pending", SessionID: "session-pending", TaskEnvironmentID: "env-pending",
				WorkspacePath: "/workspace/task", AgentID: "auggie",
			}
			manager, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				infos:    map[string]*WorkspaceInfo{info.SessionID: info},
				envInfos: map[string]*WorkspaceInfo{info.TaskEnvironmentID: info},
			})
			entry := manager.queueRemoteRecovery(&models.ExecutorRunning{
				TaskID: info.TaskID, SessionID: info.SessionID,
				AgentExecutionID: "retained-owner", Runtime: agentruntime.RuntimeSSH,
			}, nil)
			if entry == nil {
				t.Fatal("remote recovery was not queued")
			}
			execution, err := access(manager)
			if !errors.Is(err, ErrSessionRecoveryGuarded) || execution != nil {
				t.Fatalf("workspace access returned execution=%v err=%v, want retryable recovery refusal", execution != nil, err)
			}
			if backend.createCount.Load() != 0 {
				t.Fatal("workspace access created an execution beside the retained remote owner")
			}
			manager.removeRemoteRecovery(entry, true)
			execution, err = access(manager)
			if err != nil || execution == nil || backend.createCount.Load() != 1 {
				t.Fatalf("unguarded workspace access returned execution=%v err=%v creates=%d", execution != nil, err, backend.createCount.Load())
			}
		})
	}
}
