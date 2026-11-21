package handlers

import (
	"context"
	"errors"
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

type workspaceOnlyAgentManager struct {
	executor.AgentManagerClient
	launches int
}

func (m *workspaceOnlyAgentManager) LaunchAgent(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	m.launches++
	return nil, errors.New("workspace_only recovery must not launch an agent")
}

// workspace_only is honored only for repair_workspace_inventory; any other
// recovery action is a validation error before an agent can start.
func TestWsRecoverSessionWorkspaceOnlyRejectsNonRepairAction(t *testing.T) {
	for _, action := range []string{"resume", "fresh_start", "runtime_retry"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
			require.NoError(t, err)
			sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
			t.Cleanup(func() { _ = sqlxDB.Close() })
			repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })

			now := time.Now().UTC()
			require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "workspace-1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
				ID: "task-1", WorkspaceID: "workspace-1", Title: "Task", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
			}))
			require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
				ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateFailed,
				AgentProfileID: "profile-1", StartedAt: now, UpdatedAt: now,
			}))

			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
			require.NoError(t, err)
			agentMgr := &workspaceOnlyAgentManager{}
			svc := orchestrator.NewService(orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log), agentMgr, nil, repo, nil, nil, nil, log)
			handlers := NewHandlers(svc, log)

			response, err := handlers.wsRecoverSession(ctx, createTestMessage(t, ws.ActionSessionRecover, map[string]interface{}{
				"task_id": "task-1", "session_id": "session-1", "action": action,
				"workspace_only": true, "idempotency_key": "metadata-only",
			}))
			require.NoError(t, err)
			payload := parseError(t, response)
			require.Equal(t, ws.ErrorCodeValidation, payload.Code)
			require.Equal(t, "Workspace inventory repair request is invalid", payload.Message)
			require.Zero(t, agentMgr.launches)
			session, err := repo.GetTaskSession(ctx, "session-1")
			require.NoError(t, err)
			require.Equal(t, taskmodels.TaskSessionStateFailed, session.State)
		})
	}
}
