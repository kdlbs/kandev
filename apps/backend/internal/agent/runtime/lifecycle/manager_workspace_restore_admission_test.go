package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkspaceRestoreTerminalAdmission(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateFailed,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			const sessionID = "session-workspace-restore"
			mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				infos: map[string]*WorkspaceInfo{
					sessionID: {
						TaskID:        "task-workspace-restore",
						SessionID:     sessionID,
						WorkspacePath: "/workspace/task",
						AgentID:       "auggie",
					},
				},
			})
			mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
				ID: sessionID, TaskID: "task-workspace-restore", State: state,
			}})

			execution, err := mgr.GetOrEnsureExecution(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("GetOrEnsureExecution returned error: %v", err)
			}
			if execution == nil {
				t.Fatal("GetOrEnsureExecution returned nil execution")
			}
			if got := backend.createCount.Load(); got != 1 {
				t.Fatalf("runtime creation count = %d, want 1", got)
			}
			if execution.AgentCommand != "" {
				t.Fatalf("workspace-only execution has agent command %q", execution.AgentCommand)
			}
		})
	}
}

func TestWorkspaceRestoreAdmissionRejectsInvalidOwners(t *testing.T) {
	archivedAt := time.Now()
	tests := []struct {
		name       string
		reader     *fakeExecutorProfileReader
		configure  func(*Manager)
		want       error
		wantCreate int32
	}{
		{
			name:       "archived task",
			reader:     &fakeExecutorProfileReader{task: &models.Task{ID: "task-workspace-restore", ArchivedAt: &archivedAt}},
			want:       ErrSessionTerminal,
			wantCreate: 0,
		},
		{
			name:       "missing task",
			reader:     &fakeExecutorProfileReader{task: &models.Task{ID: "different-task"}},
			wantCreate: 0,
		},
		{
			name:       "cleanup active",
			reader:     &fakeExecutorProfileReader{cleanupActive: true},
			want:       errTaskCleanupActive,
			wantCreate: 0,
		},
		{
			name:   "foreign session",
			reader: &fakeExecutorProfileReader{},
			configure: func(mgr *Manager) {
				mgr.SetSessionExecAccessChecker(func(context.Context, string) error {
					return errors.New("session access denied")
				})
			},
			wantCreate: 0,
		},
		{
			name: "ambiguous task ownership",
			reader: &fakeExecutorProfileReader{session: &models.TaskSession{
				ID: "session-workspace-restore", State: models.TaskSessionStateFailed,
			}},
			wantCreate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				infos: map[string]*WorkspaceInfo{
					"session-workspace-restore": {
						TaskID:        "task-workspace-restore",
						SessionID:     "session-workspace-restore",
						WorkspacePath: "/workspace/task",
						AgentID:       "auggie",
					},
				},
			})
			if tt.reader.session == nil {
				tt.reader.session = &models.TaskSession{
					ID: "session-workspace-restore", TaskID: "task-workspace-restore", State: models.TaskSessionStateFailed,
				}
			}
			mgr.SetExecutorProfileReader(tt.reader)
			if tt.configure != nil {
				tt.configure(mgr)
			}

			_, err := mgr.EnsureWorkspaceExecutionForSession(
				context.Background(), "task-workspace-restore", "session-workspace-restore",
			)
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if err == nil {
				t.Fatal("workspace restore unexpectedly succeeded")
			}
			if got := backend.createCount.Load(); got != tt.wantCreate {
				t.Fatalf("runtime creation count = %d, want %d", got, tt.wantCreate)
			}
			if _, exists := mgr.executionStore.GetBySessionID("session-workspace-restore"); exists {
				t.Fatal("rejected workspace restore left an execution registered")
			}
		})
	}
}

func TestWorkspaceRestoreReusesRetainedTerminalRuntime(t *testing.T) {
	mgr := newTestManager(t)
	mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-workspace-restore": {
			TaskID: "task-workspace-restore", SessionID: "session-workspace-restore",
		},
	}}
	mgr.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{
		ID: "session-workspace-restore", TaskID: "task-workspace-restore", State: models.TaskSessionStateFailed,
	}})
	retained := &AgentExecution{
		ID: "retained-workspace", TaskID: "task-workspace-restore", SessionID: "session-workspace-restore",
		Status: v1.AgentStatusStopped,
	}
	if err := mgr.executionStore.Add(retained); err != nil {
		t.Fatalf("add retained execution: %v", err)
	}

	got, err := mgr.GetOrEnsureExecution(context.Background(), "session-workspace-restore")
	if err != nil {
		t.Fatalf("GetOrEnsureExecution returned error: %v", err)
	}
	if got != retained {
		t.Fatalf("execution = %p, want retained %p", got, retained)
	}
}
