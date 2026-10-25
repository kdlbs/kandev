package handlers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type inheritedWorkspaceTasks struct {
	scheduler.TaskRepository
	task *models.Task
}

type inheritedWorkspaceAgentManager struct{ archivedLaunchAgentManager }

func (*inheritedWorkspaceAgentManager) ResolveAgentProfile(context.Context, string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{SupportsMCP: true}, nil
}

func (r *inheritedWorkspaceTasks) GetTask(context.Context, string) (*v1.Task, error) {
	return r.task.ToAPI(), nil
}

func TestKubernetesInheritedWorkspaceWSConflict(t *testing.T) {
	for _, action := range []string{ws.ActionSessionEnsure, ws.ActionSessionLaunch} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
			require.NoError(t, err)
			sqlDB := sqlx.NewDb(conn, "sqlite3")
			t.Cleanup(func() { _ = sqlDB.Close() })
			repo, cleanup, err := repository.Provide(sqlDB, sqlDB, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
			now := time.Now().UTC()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "Test", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow", WorkspaceID: "ws", Name: "Test", CreatedAt: now, UpdatedAt: now}))
			parent := &models.Task{ID: "parent", WorkspaceID: "ws", WorkflowID: "workflow", Title: "Parent", CreatedAt: now, UpdatedAt: now}
			child := &models.Task{ID: "child", ParentID: parent.ID, WorkspaceID: "ws", WorkflowID: "workflow", Title: "Child", CreatedAt: now, UpdatedAt: now,
				Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}, "agent_profile_id": "profile", "executor_profile_id": "kube-profile"}}
			require.NoError(t, repo.CreateTask(ctx, parent))
			require.NoError(t, repo.CreateTask(ctx, child))
			require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: "kube", Type: models.ExecutorTypeKubernetes, Name: "Kubernetes"}))
			require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "kube-profile", ExecutorID: "kube", Name: "Kubernetes"}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "parent-env", TaskID: parent.ID, ExecutorType: "k8s", Status: models.TaskEnvironmentStatusReady}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID, TaskEnvironmentID: "parent-env", AgentProfileID: "profile", ExecutorProfileID: "kube-profile", IsPrimary: true, State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}))
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
			require.NoError(t, err)
			svc := orchestrator.NewService(orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log), &inheritedWorkspaceAgentManager{}, &inheritedWorkspaceTasks{task: child}, repo, nil, nil, nil, log)
			h := NewHandlers(svc, log)
			msg := createTestMessage(t, action, map[string]interface{}{"task_id": child.ID, "intent": "prepare", "auto_start": false, "agent_profile_id": "profile"})
			var response *ws.Message
			if action == ws.ActionSessionEnsure {
				response, err = h.wsEnsureSession(ctx, msg)
			} else {
				response, err = h.wsLaunchSession(ctx, msg)
			}
			require.NoError(t, err)
			payload := parseError(t, response)
			require.Equal(t, ws.ErrorCodeConflict, payload.Code)
			require.Equal(t, "workspace_reuse_unsupported", payload.Details["reason"])
			require.Equal(t, true, payload.Details["recoverable"])
			require.Equal(t, "create a task with workspace_mode=\"new_workspace\"", payload.Details["retry"])
			require.Contains(t, payload.Message, "new_workspace")
			sessions, err := repo.ListTaskSessions(ctx, child.ID)
			require.NoError(t, err)
			require.Empty(t, sessions)
		})
	}
}
