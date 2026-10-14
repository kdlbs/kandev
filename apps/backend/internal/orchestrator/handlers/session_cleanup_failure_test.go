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
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type cleanupResponseAgentManager struct{ executor.AgentManagerClient }

func (cleanupResponseAgentManager) CleanupStaleExecutionBySessionID(context.Context, string) error {
	return errors.New("proxy 192.0.2.10:10250 failed /private/path token=secret")
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.13
func TestSessionCleanupFailureResponse(t *testing.T) {
	for _, action := range []string{ws.ActionSessionLaunch, ws.ActionSessionRecover} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
			require.NoError(t, err)
			database := sqlx.NewDb(dbConn, "sqlite3")
			t.Cleanup(func() { _ = database.Close() })
			repo, cleanup, err := repository.Provide(database, database, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cleanup() })
			now := time.Now().UTC()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "w", Name: "Test", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf", WorkspaceID: "w", Name: "Test", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "t", WorkspaceID: "w", WorkflowID: "wf", Title: "Cleanup failure", State: v1.TaskStateFailed, CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "s", TaskID: "t", State: models.TaskSessionStateFailed, AgentProfileID: "p", StartedAt: now, UpdatedAt: now}))
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
			require.NoError(t, err)
			service := orchestrator.NewService(orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log), cleanupResponseAgentManager{}, nil, repo, nil, nil, nil, log)
			handler := NewHandlers(service, log)
			request := createTestMessage(t, action, map[string]interface{}{"task_id": "t", "session_id": "s", "action": "resume", "intent": "resume"})
			var response *ws.Message
			if action == ws.ActionSessionRecover {
				response, err = handler.wsRecoverSession(ctx, request)
			} else {
				response, err = handler.wsLaunchSession(ctx, request)
			}
			require.NoError(t, err)
			payload := parseError(t, response)
			require.Equal(t, "runtime_cleanup_failed", payload.Details["kind"])
			require.Equal(t, "s", payload.Details["session_id"])
			require.NotEmpty(t, payload.Details["attempt_id"])
			require.NotEmpty(t, payload.Details["error_stamp"])
			require.NotContains(t, string(response.Payload), "192.0.2.")
			require.NotContains(t, string(response.Payload), "secret")
		})
	}
}
