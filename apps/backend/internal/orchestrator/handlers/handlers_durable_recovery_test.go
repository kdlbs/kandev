package handlers

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
func TestRestoreRequiredRecoveryResponseExposesUnresolvedDelivery(t *testing.T) {
	msg := createTestMessage(t, ws.ActionSessionLaunch, map[string]interface{}{})
	err := fmt.Errorf("launch failed: %w", restoreRequiredTestError{reason: "unresolved_durable_work"})
	response, responseErr := restoreRequiredRecoveryResponse(msg, err, "session-interrupted")
	require.NoError(t, responseErr)
	require.NotNil(t, response, "an interrupted durable prompt needs an explicit recovery action")
	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "session_restore_required", payload.Details["kind"])
	require.Equal(t, "continue_from_history", payload.Details["recovery_action"])
	require.Equal(t, "unresolved_durable_work", payload.Details["reason"])
	require.Equal(t, "session-interrupted", payload.Details["session_id"])
}

// @covers AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
func TestWSLaunchSession_DurableRecovery(t *testing.T) {
	ctx := context.Background()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace-interrupted", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "workflow-interrupted", WorkspaceID: "workspace-interrupted", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-interrupted", WorkspaceID: "workspace-interrupted", WorkflowID: "workflow-interrupted",
		Title: "Interrupted task", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "session-interrupted", TaskID: "task-interrupted", State: taskmodels.TaskSessionStateWaitingForInput,
		AgentProfileID: "profile-1", StartedAt: now, UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, "session-interrupted")
	require.NoError(t, err)
	continuity := repo
	require.NoError(t, continuity.CreateHarnessSessionGeneration(ctx, &taskmodels.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "native-interrupted", AgentType: "codex", CreationReason: "native_started",
		CreatedAt: now, CommittedAt: now,
	}))
	require.NoError(t, continuity.UpsertSessionRecoveryBlock(ctx, &taskmodels.SessionRecoveryBlock{
		ID: "block-interrupted", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unresolved_durable_work", State: taskmodels.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", CreatedAt: now, UpdatedAt: now,
	}))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	service := orchestrator.NewService(orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log),
		&archivedLaunchAgentManager{}, nil, repo, nil, nil, nil, log)
	handlers := NewHandlers(service, log)
	response, err := handlers.wsLaunchSession(ctx, createTestMessage(t, ws.ActionSessionLaunch, map[string]interface{}{
		"task_id": "task-interrupted", "session_id": session.ID, "intent": string(orchestrator.IntentResume),
	}))
	require.NoError(t, err)
	payload := parseError(t, response)
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "continue_from_history", payload.Details["recovery_action"])
	require.Equal(t, "unresolved_durable_work", payload.Details["reason"])
	block, err := continuity.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Equal(t, taskmodels.RecoveryBlockOpen, block.State, "showing recovery must not authorize it")
}
