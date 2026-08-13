package handlers

// Ledger attribution coverage for the MCP move_task_kandev handler:
// applyMoveTaskImmediate records mcp_move, with actor kind/id resolved from
// the source session when one exists (idle) and system when there is none.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type ledgerRow struct {
	trigger   string
	actorKind string
	actorID   *string
}

func ledgerRowsForTask(t *testing.T, repo *sqliterepo.Repository, taskID string) []ledgerRow {
	t.Helper()
	rows, err := repo.DB().QueryContext(context.Background(), `
		SELECT trigger, actor_kind, actor_id FROM task_step_transitions
		WHERE task_id = ? ORDER BY occurred_at ASC, id ASC
	`, taskID)
	if err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ledgerRow
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.trigger, &r.actorKind, &r.actorID); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no ledger rows for task %s", taskID)
	}
	return out
}

func TestHandleMoveTaskImmediateNoSessionRecordsMCPMoveSystem(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mcp-move", Name: "Move", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mcp-move", WorkspaceID: "ws-mcp-move", Name: "Board", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-mcp-move", WorkspaceID: "ws-mcp-move", WorkflowID: "wf-mcp-move",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-mcp-move", "workflow_id": "wf-mcp-move", "workflow_step_id": "step-done", "position": 0,
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type)

	rows := ledgerRowsForTask(t, repo, "task-mcp-move")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPMove) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerMCPMove)
	}
	if last.actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q (no session to attribute to)", last.actorKind, steptelemetry.ActorSystem)
	}
}

// TestHandleMoveTaskImmediateTargetSessionAloneDoesNotAttributeToIt pins the
// fix for a real misattribution bug found in review: the target task having
// an idle session of its own must NOT cause the ledger row to attribute to
// that session merely because it happened to be in scope for hand-off-prompt
// queuing. Before the fix, calling move_task_kandev with no sender_session_id
// incorrectly recorded actor_kind=agent, actor_id=<target's own session> —
// attributing the move to a session that took no action. Absent an explicit
// sender_session_id, the correct record is actor_kind=system: no initiating
// identity is known.
func TestHandleMoveTaskImmediateTargetSessionAloneDoesNotAttributeToIt(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mcp-idle", Name: "Move", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mcp-idle", WorkspaceID: "ws-mcp-idle", Name: "Board", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-mcp-idle", WorkspaceID: "ws-mcp-idle", WorkflowID: "wf-mcp-idle",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-mcp-idle", TaskID: "task-mcp-idle", State: models.TaskSessionStateWaitingForInput,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-mcp-idle", "workflow_id": "wf-mcp-idle", "workflow_step_id": "step-done", "position": 0,
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type)

	rows := ledgerRowsForTask(t, repo, "task-mcp-idle")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPMove) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerMCPMove)
	}
	if last.actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q (target's own idle session must not be treated as the actor)", last.actorKind, steptelemetry.ActorSystem)
	}
	if last.actorID != nil {
		t.Fatalf("actor_id = %v, want nil", *last.actorID)
	}
}

// TestHandleMoveTaskImmediateUsesSenderSessionNotTargetSession proves the fix
// end-to-end: when the caller supplies sender_session_id — as moveTaskHandler
// now always does, injected server-side from the MCP server's own bound
// session — the ledger row attributes to THAT session, even though the
// target task has a different idle session of its own that must not be
// confused for the actor.
func TestHandleMoveTaskImmediateUsesSenderSessionNotTargetSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mcp-sender", Name: "Move", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mcp-sender", WorkspaceID: "ws-mcp-sender", Name: "Board", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-mcp-target", WorkspaceID: "ws-mcp-sender", WorkflowID: "wf-mcp-sender",
		WorkflowStepID: "step-work", Title: "Move me", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-mcp-target-idle", TaskID: "task-mcp-target", State: models.TaskSessionStateWaitingForInput,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
	// The calling agent's own session, on a different task entirely — the
	// realistic shape of an agent orchestrating a card it doesn't run on.
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-mcp-caller", WorkspaceID: "ws-mcp-sender", WorkflowID: "wf-mcp-sender",
		WorkflowStepID: "step-work", Title: "Caller task", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-mcp-caller", TaskID: "task-mcp-caller", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-mcp-target", "workflow_id": "wf-mcp-sender", "workflow_step_id": "step-done", "position": 0,
		"sender_session_id": "sess-mcp-caller",
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type)

	rows := ledgerRowsForTask(t, repo, "task-mcp-target")
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPMove) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerMCPMove)
	}
	if last.actorKind != string(steptelemetry.ActorAgent) {
		t.Fatalf("actor_kind = %q, want %q", last.actorKind, steptelemetry.ActorAgent)
	}
	if last.actorID == nil || *last.actorID != "sess-mcp-caller" {
		t.Fatalf("actor_id = %v, want sess-mcp-caller (the caller, not the target's own idle session sess-mcp-target-idle)", last.actorID)
	}
}
