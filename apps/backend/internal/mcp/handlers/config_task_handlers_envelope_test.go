package handlers

// Move-result envelope coverage for the MCP move_task_kandev handler
// (codex thread 3936395919): both the immediate and deferred paths must return
// a dto.MoveTaskResponse carrying move_id, normalized entry_options, and a
// disposition so an agent can distinguish deferred acceptance from an immediate
// move and correlate the retained one-shot options with the eventual entry.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleMoveTask_ImmediateReturnsMoveResultEnvelope asserts the idle path
// returns an "applied" envelope with the moved task, rather than a bare task DTO.
func TestHandleMoveTask_ImmediateReturnsMoveResultEnvelope(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-env", Name: "Env", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-env", WorkspaceID: "ws-env", Name: "Board", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-env", WorkspaceID: "ws-env", WorkflowID: "wf-env",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-env", "workflow_id": "wf-env", "workflow_step_id": "step-done", "position": 0,
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "payload: %s", string(resp.Payload))

	var env dto.MoveTaskResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &env))
	assert.Equal(t, moveDispositionApplied, env.Disposition)
	assert.Equal(t, "task-env", env.Task.ID)
	assert.Equal(t, "step-done", env.Task.WorkflowStepID)
	assert.Empty(t, env.MoveID, "an option-less move mints no move_id")
	assert.Nil(t, env.EntryOptions)
}

// TestHandleMoveTask_DeferredReturnsMoveResultEnvelope asserts the deferred
// (active-session) path returns a "deferred" envelope whose move_id matches the
// recorded PendingMove and whose entry_options echo the accepted one-shot
// override, so the agent can correlate the retained options with the eventual
// step entry.
func TestHandleMoveTask_DeferredReturnsMoveResultEnvelope(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-envd", Name: "Env", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-envd", WorkspaceID: "ws-envd", Name: "Board", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-envd", WorkspaceID: "ws-envd", WorkflowID: "wf-envd",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-envd", TaskID: "task-envd", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, messageQueue: queue, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-envd", "workflow_id": "wf-envd", "workflow_step_id": "step-done", "position": 0,
		"entry_options": map[string]interface{}{"reset_context": true},
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "payload: %s", string(resp.Payload))
	require.Len(t, queue.pendingMoves, 1)

	var env dto.MoveTaskResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &env))
	assert.Equal(t, moveDispositionDeferred, env.Disposition)
	assert.Equal(t, "task-envd", env.Task.ID)
	assert.Equal(t, "step-done", env.Task.WorkflowStepID)
	assert.NotEmpty(t, env.MoveID, "a deferred optioned move must expose its move_id")
	assert.Equal(t, queue.pendingMoves[0].MoveID, env.MoveID,
		"envelope move_id must correlate with the recorded PendingMove")
	require.NotNil(t, env.EntryOptions)
	assert.True(t, env.EntryOptions.ResetContext, "accepted one-shot options must be echoed back")
}
