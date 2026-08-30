package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A terminal move is the one active-session exception to deferred routing:
// success must mean the card and terminal task state are already committed.
func TestHandleMoveTask_ActiveSessionTerminalMoveCommitsBeforeResponse(t *testing.T) {
	svc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-terminal", Name: "Terminal", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-terminal", WorkspaceID: "ws-terminal", Name: "Board", CreatedAt: now, UpdatedAt: now,
	}))
	for _, step := range []*workflowmodels.WorkflowStep{
		{ID: "step-pr", WorkflowID: "wf-terminal", Name: "PR", Position: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "step-done", WorkflowID: "wf-terminal", Name: "Done", Position: 1, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, workflowRepo.CreateStep(ctx, step))
	}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-terminal", WorkspaceID: "ws-terminal", WorkflowID: "wf-terminal",
		WorkflowStepID: "step-pr", Title: "Finish", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-terminal", TaskID: "task-terminal", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
	turn := &models.Turn{
		ID: "turn-terminal", TaskSessionID: "sess-terminal", TaskID: "task-terminal",
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	stamped, err := repo.CreateTurnWithStepStamp(ctx, turn)
	require.NoError(t, err)
	require.True(t, stamped)

	queueDB := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(queueDB, queueDB)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger(t))
	require.NoError(t, queue.SetPendingMove(ctx, "sess-terminal", &messagequeue.PendingMove{
		ID: "pending-terminal", MoveID: "move-terminal", TaskID: "task-terminal",
		WorkflowID: "wf-terminal", WorkflowStepID: "step-done",
		ExpectedWorkflowStepID: "step-pr",
	}))
	h := &Handlers{
		taskSvc: svc, workflowCtrl: workflowCtrl, messageQueue: queue,
		logger: testLogger(t).WithFields(),
	}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-terminal", "workflow_id": "wf-terminal",
		"workflow_step_id": "step-done", "sender_session_id": "sess-terminal",
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	assert.NotEqual(t, ws.MessageTypeError, resp.Type)
	pending, present, err := queue.GetPendingMoveWithError(ctx, "sess-terminal")
	require.NoError(t, err)
	assert.False(t, present, "terminal success must settle its deferred route in the task transaction")
	assert.Nil(t, pending)

	stored, err := svc.GetTask(ctx, "task-terminal")
	require.NoError(t, err)
	assert.Equal(t, "step-done", stored.WorkflowStepID)
	assert.Equal(t, v1.TaskStateCompleted, stored.State)

	var transitionCount int
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_step_transitions
		WHERE task_id = ? AND from_workflow_step_id = ? AND to_workflow_step_id = ?
	`, "task-terminal", "step-pr", "step-done").Scan(&transitionCount))
	assert.Equal(t, 1, transitionCount)

	// The agent turn was launched in PR. Its immediate completion signal is
	// stale after the manual terminal route and must fail without recreating a
	// deferred row or attributing a signal to Done.
	bus := &mcpRecordingEventBus{}
	h.sessionRepo = repo
	h.eventBus = bus
	signalResp, err := h.handleStepComplete(ctx, makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id": "task-terminal", "session_id": "sess-terminal", "summary": "merged",
	}))
	require.NoError(t, err)
	assertWSError(t, signalResp, ws.ErrorCodeValidation)
	assert.Empty(t, bus.events)
	session, err := repo.GetTaskSession(ctx, "sess-terminal")
	require.NoError(t, err)
	_, hasSignal := models.LoadPendingStepSignal(session.Metadata)
	assert.False(t, hasSignal)
	_, present, err = queue.GetPendingMoveWithError(ctx, "sess-terminal")
	require.NoError(t, err)
	assert.False(t, present)

	// A bound merged-PR lifecycle observation routes to the same terminal
	// target. Its persisted prompt metadata is the trusted external cause; the
	// agent cannot supply or alter that tuple in move_task_kandev.
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		TaskSessionID: "sess-terminal", TaskID: "task-terminal", TurnID: "turn-terminal",
		AuthorType: models.MessageAuthorUser, Content: "PR merged",
		Metadata: map[string]interface{}{
			"origin": "github_pr_automation", "automation_kind": "merged",
			"repository_id": "repo-terminal", "pr_number": 14,
		},
	}))
	retryMsg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-terminal", "workflow_id": "wf-terminal",
		"workflow_step_id": "step-done", "sender_session_id": "sess-terminal",
	})
	retryMsg.ID = "merged-observation"
	// A duplicate merged-PR observation routes to the same terminal target.
	// It converges on the committed destination without a second ledger/effect
	// identity and without re-arming the stale source generation.
	retryResp, err := h.handleMoveTask(ctx, retryMsg)
	require.NoError(t, err)
	assert.NotEqual(t, ws.MessageTypeError, retryResp.Type)
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_step_transitions
		WHERE task_id = ? AND from_workflow_step_id = ? AND to_workflow_step_id = ?
	`, "task-terminal", "step-pr", "step-done").Scan(&transitionCount))
	assert.Equal(t, 1, transitionCount)
	_, present, err = queue.GetPendingMoveWithError(ctx, "sess-terminal")
	require.NoError(t, err)
	assert.False(t, present)

	var committedRoutes, staleSignals, mergedRoutes, effects int
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_route_operations
		WHERE task_id = ? AND producer = 'manual_move' AND outcome = 'committed'
		  AND expected_step_id = ? AND observed_step_id = ? AND target_step_id = ?
		  AND session_id = ? AND transition_id IS NOT NULL AND effect_id IS NOT NULL
	`, "task-terminal", "step-pr", "step-pr", "step-done", "sess-terminal").Scan(&committedRoutes))
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_route_operations
		WHERE task_id = ? AND producer = 'step_complete' AND outcome = 'stale_source'
		  AND expected_step_id = ? AND observed_step_id = ? AND turn_id = ?
	`, "task-terminal", "step-pr", "step-done", "turn-terminal").Scan(&staleSignals))
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_route_effects WHERE task_id = ? AND target_step_id = ?
	`, "task-terminal", "step-done").Scan(&effects))
	require.NoError(t, repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_route_operations
		WHERE task_id = ? AND producer = 'merged_pr' AND outcome = 'already_satisfied'
		  AND external_cause = 'github_pr_merged' AND external_cause_id = 'repo-terminal:14'
		  AND transition_id IS NULL AND effect_id IS NULL
	`, "task-terminal").Scan(&mergedRoutes))
	assert.Equal(t, 1, committedRoutes)
	assert.Equal(t, 1, staleSignals)
	assert.Equal(t, 1, mergedRoutes)
	assert.Equal(t, 1, effects)
}
