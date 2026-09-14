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
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type archivedLaunchAgentManager struct {
	executor.AgentManagerClient
}

func TestWSLaunchSession_ArchivedConflict(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace-1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "workflow-1", WorkspaceID: "workspace-1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-archive", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		Title: "Archived task", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "session-archive", TaskID: "task-archive", State: taskmodels.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-1", StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.ArchiveTask(ctx, "task-archive"))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &taskmodels.ExecutorRunning{
		ID: "running-archive", SessionID: "session-archive", TaskID: "task-archive",
		Status: "ready", Resumable: true, ResumeToken: "acp-session",
		CreatedAt: now, UpdatedAt: now,
	}))

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := orchestrator.NewService(
		orchestrator.ServiceConfig{},
		bus.NewMemoryEventBus(log),
		&archivedLaunchAgentManager{},
		nil,
		repo,
		nil,
		nil,
		nil,
		log,
	)
	handlers := NewHandlers(svc, log)

	response, err := handlers.wsLaunchSession(ctx, createTestMessage(t, ws.ActionSessionLaunch, map[string]interface{}{
		"task_id":    "task-archive",
		"session_id": "session-archive",
		"intent":     string(orchestrator.IntentResume),
	}))
	require.NoError(t, err)

	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "task_archived", payload.Details["kind"])
}
