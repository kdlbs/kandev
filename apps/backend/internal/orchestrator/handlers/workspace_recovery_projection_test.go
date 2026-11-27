package handlers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type restoreProjectionAgentManager struct {
	executor.AgentManagerClient
	err error
}

func (m restoreProjectionAgentManager) EnsureWorkspaceExecutionForSession(context.Context, string, string) error {
	return m.err
}

func TestRestoreWorkspaceReturnsManagedCloneRelocationDetails(t *testing.T) {
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace-restore", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "workflow-restore", WorkspaceID: "workspace-restore", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-restore", WorkspaceID: "workspace-restore", WorkflowID: "workflow-restore",
		Title: "Restore workspace", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "session-restore", TaskID: "task-restore", State: taskmodels.TaskSessionStateCancelled,
		AgentProfileID: "profile-restore", StartedAt: now, UpdatedAt: now,
	}))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	service := orchestrator.NewService(
		orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log),
		restoreProjectionAgentManager{err: &lifecycle.WorkspaceRecoveryProjectionError{Stamp: "relocation-stamp"}},
		nil, repo, nil, nil, nil, log,
	)
	response, err := NewHandlers(service, log).wsLaunchSession(ctx, createTestMessage(t, ws.ActionSessionLaunch, map[string]interface{}{
		"task_id": "task-restore", "session_id": "session-restore", "intent": string(orchestrator.IntentRestoreWorkspace),
	}))
	require.NoError(t, err)

	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "managed_clone_relocation_required", payload.Details["kind"])
	require.Equal(t, "relocation-stamp", payload.Details["error_stamp"])
	require.Equal(t, "relocate_and_resume", payload.Details["recovery_action"])
}

func TestSessionLaunchMapsRecoveryInspectionContentionToConflict(t *testing.T) {
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace-contention", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "workflow-contention", WorkspaceID: "workspace-contention", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-contention", WorkspaceID: "workspace-contention", WorkflowID: "workflow-contention",
		Title: "Session launch contention", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "session-contention", TaskID: "task-contention", State: taskmodels.TaskSessionStateCancelled,
		AgentProfileID: "profile-contention", StartedAt: now, UpdatedAt: now,
	}))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	service := orchestrator.NewService(
		orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log),
		restoreProjectionAgentManager{err: &worktree.RecoveryInspectionContentionError{}},
		nil, repo, nil, nil, nil, log,
	)
	response, err := NewHandlers(service, log).wsLaunchSession(ctx, createTestMessage(t, ws.ActionSessionLaunch, map[string]interface{}{
		"task_id": "task-contention", "session_id": "session-contention", "intent": string(orchestrator.IntentRestoreWorkspace),
	}))
	require.NoError(t, err)

	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "recovery_inspection_busy", payload.Details["kind"])
}
