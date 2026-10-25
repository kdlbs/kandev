package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAgentStartsStillRejectTerminalSessions(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: terminalSessionID, TaskID: terminalTaskID, State: models.TaskSessionStateCompleted,
	}})
	execution := &AgentExecution{
		ID:           "execution-terminal-agent",
		SessionID:    terminalSessionID,
		TaskID:       terminalTaskID,
		AgentCommand: "agent-command",
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.StartAgentProcess(context.Background(), execution.ID); !errors.Is(err, ErrSessionTerminal) {
		t.Fatalf("StartAgentProcess() error = %v, want ErrSessionTerminal", err)
	}
}

func TestWorkspacePromotionStillRejectsTerminalSessions(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: terminalSessionID, TaskID: terminalTaskID, State: models.TaskSessionStateCompleted,
	}})
	execution := &AgentExecution{ID: "execution-terminal-promotion", SessionID: terminalSessionID}
	req := &LaunchRequest{SessionID: terminalSessionID, AgentProfileID: "profile-1"}

	if err := mgr.promoteWorkspaceExecution(context.Background(), execution, req); !errors.Is(err, ErrSessionTerminal) {
		t.Fatalf("promoteWorkspaceExecution() error = %v, want ErrSessionTerminal", err)
	}
	if execution.AgentCommand != "" {
		t.Fatalf("terminal promotion configured agent command %q", execution.AgentCommand)
	}
}

func TestPassthroughReconnectStillRejectsTerminalSessions(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: terminalSessionID, TaskID: terminalTaskID, State: models.TaskSessionStateCompleted,
	}})

	if _, err := mgr.EnsurePassthroughExecution(context.Background(), terminalSessionID); !errors.Is(err, ErrSessionTerminal) {
		t.Fatalf("EnsurePassthroughExecution() error = %v, want ErrSessionTerminal", err)
	}
}

func TestWorkspaceRestoreRejectsActiveCleanupWithoutMutation(t *testing.T) {
	entryPoints := []struct {
		name string
		call func(*Manager) error
	}{
		{
			name: "environment",
			call: func(m *Manager) error {
				_, err := m.GetOrEnsureExecutionForEnvironment(context.Background(), terminalEnvironmentID)
				return err
			},
		},
		{
			name: "session",
			call: func(m *Manager) error {
				_, err := m.GetOrEnsureExecution(context.Background(), terminalSessionID)
				return err
			},
		},
		{
			name: "explicit workspace session",
			call: func(m *Manager) error {
				_, err := m.EnsureWorkspaceExecutionForSession(
					context.Background(), terminalTaskID, terminalSessionID,
				)
				return err
			},
		},
	}

	for _, entryPoint := range entryPoints {
		t.Run(entryPoint.name, func(t *testing.T) {
			mgr, backend := newTerminalSessionManager(t, models.TaskSessionStateCompleted)
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
				session: &models.TaskSession{
					ID: terminalSessionID, TaskID: terminalTaskID,
					State: models.TaskSessionStateCompleted,
				},
				env: &models.TaskEnvironment{
					ID: terminalEnvironmentID, TaskID: terminalTaskID,
					Status: models.TaskEnvironmentStatusStopped,
				},
				cleanupActive: true,
			})

			if err := entryPoint.call(mgr); !errors.Is(err, errTaskCleanupActive) {
				t.Fatalf("workspace restore error = %v, want errTaskCleanupActive", err)
			}
			if got := backend.createCount.Load(); got != 0 {
				t.Fatalf("CreateInstance calls = %d, want 0", got)
			}
			if got := backend.stopCount.Load(); got != 0 {
				t.Fatalf("StopInstance calls = %d, want 0", got)
			}
		})
	}
}

func TestWorkspaceRestoreRejectsAdmissionChangesWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		info       func(*WorkspaceInfo)
		env        func(*models.TaskEnvironment)
		wantErr    error
		wantInText string
	}{
		{
			name:    "archived task",
			info:    func(info *WorkspaceInfo) { info.TaskArchived = true },
			wantErr: ErrSessionWorkspaceNotReady,
		},
		{
			name:    "archived workspace owner",
			info:    func(info *WorkspaceInfo) { info.WorkspaceOwnerArchived = true },
			wantErr: ErrSessionWorkspaceNotReady,
		},
		{
			name:    "environment ownership changed",
			info:    func(info *WorkspaceInfo) { info.ValidatedTaskEnvironmentGeneration = 3 },
			env:     func(env *models.TaskEnvironment) { env.OwnershipGeneration = 4 },
			wantErr: ErrSessionWorkspaceNotReady,
		},
		{
			name:    "environment is still materializing",
			env:     func(env *models.TaskEnvironment) { env.Status = models.TaskEnvironmentStatusCreating },
			wantErr: ErrSessionWorkspaceNotReady,
		},
		{
			name:    "session environment binding changed",
			info:    func(info *WorkspaceInfo) { info.TaskEnvironmentID = "env-other" },
			wantErr: ErrSessionWorkspaceNotReady,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := &WorkspaceInfo{
				TaskID: terminalTaskID, SessionID: terminalSessionID,
				TaskEnvironmentID: terminalEnvironmentID, WorkspacePath: "/workspace/task",
			}
			if tc.info != nil {
				tc.info(info)
			}
			env := &models.TaskEnvironment{
				ID: terminalEnvironmentID, TaskID: terminalTaskID,
				Status: models.TaskEnvironmentStatusStopped, OwnershipGeneration: 3,
			}
			if tc.env != nil {
				tc.env(env)
			}
			provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{terminalSessionID: info}}
			mgr, backend := newEnvironmentExecutionTestManager(t, provider)
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
				session: &models.TaskSession{
					ID: terminalSessionID, TaskID: terminalTaskID,
					TaskEnvironmentID: terminalEnvironmentID, State: models.TaskSessionStateCompleted,
				},
				env: env,
			})

			_, err := mgr.EnsureWorkspaceExecutionForSession(context.Background(), terminalTaskID, terminalSessionID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("workspace restore error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantInText != "" && !strings.Contains(err.Error(), tc.wantInText) {
				t.Fatalf("workspace restore error = %v, want text %q", err, tc.wantInText)
			}
			if got := backend.createCount.Load(); got != 0 {
				t.Fatalf("CreateInstance calls = %d, want 0", got)
			}
		})
	}
}

func TestCachedWorkspaceRestoreRechecksProviderAdmission(t *testing.T) {
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		terminalSessionID: {
			TaskID: terminalTaskID, SessionID: terminalSessionID,
			TaskEnvironmentID: terminalEnvironmentID, TaskArchived: true,
		},
	}}
	mgr, backend := newEnvironmentExecutionTestManager(t, provider)
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{
		session: &models.TaskSession{
			ID: terminalSessionID, TaskID: terminalTaskID,
			TaskEnvironmentID: terminalEnvironmentID, State: models.TaskSessionStateCompleted,
		},
		env: &models.TaskEnvironment{
			ID: terminalEnvironmentID, TaskID: terminalTaskID,
			Status: models.TaskEnvironmentStatusStopped,
		},
	})
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "execution-cached-archived", SessionID: terminalSessionID,
		TaskID: terminalTaskID, TaskEnvironmentID: terminalEnvironmentID,
	}); err != nil {
		t.Fatalf("add cached execution: %v", err)
	}

	_, err := mgr.GetOrEnsureExecution(context.Background(), terminalSessionID)
	if !errors.Is(err, ErrSessionWorkspaceNotReady) {
		t.Fatalf("cached workspace restore error = %v, want ErrSessionWorkspaceNotReady", err)
	}
	if got := backend.createCount.Load(); got != 0 {
		t.Fatalf("CreateInstance calls = %d, want 0", got)
	}
}
