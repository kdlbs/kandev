package sqlite

// Core writer coverage for task_step_transitions: one row per committed step
// change, no row for no-ops/rejections/rollbacks, NULL normalization, and the
// genesis-row shape on task creation. Trigger/actor attribution is exercised
// here by manually wrapping the context (steptelemetry.WithAttribution) the
// way Task 05's real callers do — this file pins the WRITER's behavior given
// an attribution, independent of which production call site sets it.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
)

func newStepTransitionsTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "step-transitions.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

type stepTransitionRow struct {
	id                 int64
	taskID             string
	sessionID          *string
	fromWorkflowID     *string
	fromWorkflowStepID *string
	toWorkflowID       *string
	toWorkflowStepID   *string
	trigger            string
	actorKind          string
	actorID            *string
	contractVersion    int
	occurredAt         time.Time
}

func stepTransitionRowsForTask(t *testing.T, repo *Repository, taskID string) []stepTransitionRow {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), repo.db.Rebind(`
		SELECT id, task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at
		FROM task_step_transitions WHERE task_id = ? ORDER BY occurred_at ASC, id ASC
	`), taskID)
	if err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []stepTransitionRow
	for rows.Next() {
		var r stepTransitionRow
		if err := rows.Scan(&r.id, &r.taskID, &r.sessionID, &r.fromWorkflowID, &r.fromWorkflowStepID, &r.toWorkflowID, &r.toWorkflowStepID, &r.trigger, &r.actorKind, &r.actorID, &r.contractVersion, &r.occurredAt); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func createStepTransitionsTestTask(t *testing.T, repo *Repository, taskID, workflowID, stepID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Test Task",
		Priority:       "medium",
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func createStepTransitionsTestTaskWithCtx(t *testing.T, repo *Repository, ctx context.Context, taskID, workflowID, stepID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Test Task",
		Priority:       "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func TestGenesisRowOnTaskCreation(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "task-genesis", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-genesis")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.fromWorkflowID != nil || row.fromWorkflowStepID != nil {
		t.Fatalf("genesis row from_* = (%v, %v), want (nil, nil)", row.fromWorkflowID, row.fromWorkflowStepID)
	}
	if row.toWorkflowID == nil || *row.toWorkflowID != "wf-1" {
		t.Fatalf("to_workflow_id = %v, want wf-1", row.toWorkflowID)
	}
	if row.toWorkflowStepID == nil || *row.toWorkflowStepID != "step-a" {
		t.Fatalf("to_workflow_step_id = %v, want step-a", row.toWorkflowStepID)
	}
	if row.trigger != string(steptelemetry.TriggerTaskCreated) {
		t.Fatalf("trigger = %q, want %q", row.trigger, steptelemetry.TriggerTaskCreated)
	}
	if row.contractVersion != steptelemetry.ContractVersion {
		t.Fatalf("contract_version = %d, want %d", row.contractVersion, steptelemetry.ContractVersion)
	}
}

func TestTaskCreatedWithNoWorkflowWritesNoGenesisRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "task-no-workflow", "", "")

	rows := stepTransitionRowsForTask(t, repo, "task-no-workflow")
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 (task created with no workflow writes nothing)", len(rows))
	}
}

func TestGenesisRowRecordsFeederStepWhenWIPDivertsPlacement(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()

	// Fill the target step to capacity with one admitted task.
	createStepTransitionsTestTask(t, repo, "task-occupant", "wf-1", "step-target")

	overflow := &models.Task{ID: "task-overflow", WorkspaceID: "ws-1", Title: "Overflow", Priority: "medium"}
	if err := repo.CreateTaskWithWorkflowStepAdmission(ctx, overflow, "step-target", 1, "step-feeder", 0); err != nil {
		t.Fatalf("CreateTaskWithWorkflowStepAdmission: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-overflow")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].toWorkflowStepID == nil || *rows[0].toWorkflowStepID != "step-feeder" {
		t.Fatalf("genesis row to_workflow_step_id = %v, want step-feeder (WIP-diverted placement)", rows[0].toWorkflowStepID)
	}
}

func TestUpdateTaskMoveWritesOneRowWithAttribution(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerManualMove,
		ActorKind: steptelemetry.ActorHuman,
		ActorID:   "user-1",
	})
	task := createStepTransitionsTestTask(t, repo, "task-move", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-move")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (genesis + move)", len(rows))
	}
	move := rows[1]
	if move.fromWorkflowStepID == nil || *move.fromWorkflowStepID != "step-a" {
		t.Fatalf("from_workflow_step_id = %v, want step-a", move.fromWorkflowStepID)
	}
	if move.toWorkflowStepID == nil || *move.toWorkflowStepID != "step-b" {
		t.Fatalf("to_workflow_step_id = %v, want step-b", move.toWorkflowStepID)
	}
	if move.trigger != string(steptelemetry.TriggerManualMove) {
		t.Fatalf("trigger = %q, want %q", move.trigger, steptelemetry.TriggerManualMove)
	}
	if move.actorKind != string(steptelemetry.ActorHuman) {
		t.Fatalf("actor_kind = %q, want %q", move.actorKind, steptelemetry.ActorHuman)
	}
	if move.actorID == nil || *move.actorID != "user-1" {
		t.Fatalf("actor_id = %v, want user-1", move.actorID)
	}
}

// TestUpdateTaskOccurredAtEqualsTaskUpdatedAt proves the spec's "Ledger vs.
// tasks.updated_at" guarantee — occurred_at and the task's updated_at agree
// at write time — by asserting exact equality rather than approximate
// closeness. Before this round's fix, UpdateTask stamped task.UpdatedAt with
// its own time.Now().UTC() call and recordStepTransition independently made
// a second, separate time.Now().UTC() call for occurred_at; under lock
// contention the two could drift. Both now share one timestamp.
func TestUpdateTaskOccurredAtEqualsTaskUpdatedAt(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-occurred-at", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	reread, err := repo.GetTask(ctx, "task-occurred-at")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-occurred-at")
	move := rows[len(rows)-1]
	if !move.occurredAt.Equal(reread.UpdatedAt) {
		t.Fatalf("occurred_at = %v, want exactly tasks.updated_at %v", move.occurredAt, reread.UpdatedAt)
	}
}

func TestUpdateTaskNoTriggerDeclaredRecordsUnknown(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background() // no attribution wrapped
	task := createStepTransitionsTestTask(t, repo, "task-unknown", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-unknown")
	move := rows[len(rows)-1]
	if move.trigger != string(steptelemetry.TriggerUnknown) {
		t.Fatalf("trigger = %q, want %q", move.trigger, steptelemetry.TriggerUnknown)
	}
	if move.actorKind != string(steptelemetry.ActorUnknown) {
		t.Fatalf("actor_kind = %q, want %q", move.actorKind, steptelemetry.ActorUnknown)
	}
	if move.actorID != nil {
		t.Fatalf("actor_id = %v, want nil", move.actorID)
	}
}

func TestUpdateTaskNoOpMoveToCurrentStepWritesNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-noop", "wf-1", "step-a")

	before := stepTransitionRowsForTask(t, repo, "task-noop")

	task.Position = 5 // position-only reorder within the same step
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	after := stepTransitionRowsForTask(t, repo, "task-noop")
	if len(after) != len(before) {
		t.Fatalf("rows after no-op update = %d, want unchanged %d", len(after), len(before))
	}
}

func TestUpdateTaskEmptyStringStepNormalizesToNULL(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-empty-from", "wf-1", "step-a")

	task.WorkflowID = ""
	task.WorkflowStepID = ""
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-empty-from")
	detach := rows[len(rows)-1]
	if detach.toWorkflowStepID != nil {
		t.Fatalf("to_workflow_step_id = %v, want nil (empty string normalizes to NULL)", detach.toWorkflowStepID)
	}
	if detach.fromWorkflowStepID == nil || *detach.fromWorkflowStepID != "step-a" {
		t.Fatalf("from_workflow_step_id = %v, want step-a", detach.fromWorkflowStepID)
	}
}

func TestUpdateTaskIfWorkflowStepHasCapacityRejectionWritesNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-occupant-cap", "wf-1", "step-full")
	task := createStepTransitionsTestTask(t, repo, "task-rejected", "wf-1", "step-a")

	before := stepTransitionRowsForTask(t, repo, "task-rejected")

	task.WorkflowStepID = "step-full"
	err := repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, task, "step-full", "task-rejected", 1)
	if err == nil {
		t.Fatal("expected WIP limit error, got nil")
	}

	after := stepTransitionRowsForTask(t, repo, "task-rejected")
	if len(after) != len(before) {
		t.Fatalf("rows after WIP rejection = %d, want unchanged %d", len(after), len(before))
	}

	reread, err := repo.GetTask(ctx, "task-rejected")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reread.WorkflowStepID != "step-a" {
		t.Fatalf("task step after rejected move = %q, want unchanged %q", reread.WorkflowStepID, "step-a")
	}
}

func TestUpdateTaskIfWorkflowStepHasCapacityAdmitsAndWritesRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman})
	task := createStepTransitionsTestTask(t, repo, "task-admit", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, task, "step-b", "task-admit", 5); err != nil {
		t.Fatalf("UpdateTaskIfWorkflowStepHasCapacity: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-admit")
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-b" {
		t.Fatalf("to_workflow_step_id = %v, want step-b", last.toWorkflowStepID)
	}
}

func TestPromoteQueuedTaskIfWorkflowStepHasCapacityWritesRowOnlyWhenPromoted(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-promote", "wf-1", "step-queue")
	task.QueuedForStepID = "step-dest"

	task.WorkflowStepID = "step-dest"
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-queue", "step-dest", 5)
	if err != nil {
		t.Fatalf("PromoteQueuedTaskIfWorkflowStepHasCapacity: %v", err)
	}
	if !promoted {
		t.Fatal("expected promotion to succeed")
	}

	rows := stepTransitionRowsForTask(t, repo, "task-promote")
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-dest" {
		t.Fatalf("to_workflow_step_id = %v, want step-dest", last.toWorkflowStepID)
	}

	// A second attempt with a stale fromStepID (already moved) affects zero
	// rows and must write nothing.
	before := stepTransitionRowsForTask(t, repo, "task-promote")
	promotedAgain, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-queue", "step-dest", 5)
	if err != nil {
		t.Fatalf("second PromoteQueuedTaskIfWorkflowStepHasCapacity: %v", err)
	}
	if promotedAgain {
		t.Fatal("second promotion should be a no-op (task no longer in step-queue)")
	}
	after := stepTransitionRowsForTask(t, repo, "task-promote")
	if len(after) != len(before) {
		t.Fatalf("rows after no-op promotion = %d, want unchanged %d", len(after), len(before))
	}
}

func TestAddTaskToWorkflowWritesWorkflowAttachedRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	// AddTaskToWorkflow does not hardcode its trigger the way genesis does —
	// its one production caller (office/engine_adapters/workflow_switcher_adapter.go)
	// sets TriggerWorkflowAttached before calling. Wrap ctx here the way that
	// caller does, per this file's header comment on exercising attribution.
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerWorkflowAttached,
		ActorKind: steptelemetry.ActorHuman,
		ActorID:   "user-1",
	})
	createStepTransitionsTestTask(t, repo, "task-attach", "", "")

	if err := repo.AddTaskToWorkflow(ctx, "task-attach", "wf-2", "step-x", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-attach")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (bare create wrote none, attach writes the first)", len(rows))
	}
	row := rows[0]
	if row.fromWorkflowStepID != nil {
		t.Fatalf("from_workflow_step_id = %v, want nil", row.fromWorkflowStepID)
	}
	if row.toWorkflowStepID == nil || *row.toWorkflowStepID != "step-x" {
		t.Fatalf("to_workflow_step_id = %v, want step-x", row.toWorkflowStepID)
	}
	if row.trigger != string(steptelemetry.TriggerWorkflowAttached) {
		t.Fatalf("trigger = %q, want %q", row.trigger, steptelemetry.TriggerWorkflowAttached)
	}
}

// TestAddTaskToWorkflowPersistsSessionCausedAttribution proves the fix for
// Review round 2's must-fix #1 (workflow_switcher_adapter.go's
// SwitchTaskWorkflow) is actually observed at the layer that matters: the
// real row this writer persists. TestAddTaskToWorkflowWritesWorkflowAttachedRow
// above only exercises the ActorHuman case and never asserts actor_kind/
// actor_id/session_id at all — round 2's fix (agent + session attribution
// for a switch_workflow-caused attach) had no test proving the persisted
// row, only tests proving the ctx reaching the mock adapter/mover boundary.
func TestAddTaskToWorkflowPersistsSessionCausedAttribution(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "task-attach-agent", "", "")
	// session_id has a foreign key to task_sessions.
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "sess-1", TaskID: "task-attach-agent", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerWorkflowAttached,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   "sess-1",
		SessionID: "sess-1",
	})

	if err := repo.AddTaskToWorkflow(ctx, "task-attach-agent", "wf-2", "step-x", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-attach-agent")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.actorKind != string(steptelemetry.ActorAgent) {
		t.Fatalf("actor_kind = %q, want %q", row.actorKind, steptelemetry.ActorAgent)
	}
	if row.actorID == nil || *row.actorID != "sess-1" {
		t.Fatalf("actor_id = %v, want sess-1", row.actorID)
	}
	if row.sessionID == nil || *row.sessionID != "sess-1" {
		t.Fatalf("session_id = %v, want sess-1", row.sessionID)
	}
}

// TestAddTaskToWorkflowNonexistentTaskIsNoOp pins Review round 4's must-fix:
// AddTaskToWorkflow discarded its UPDATE's RowsAffected() and called
// recordStepTransition unconditionally, unlike every sibling chokepoint
// (including its own twin RemoveTaskFromWorkflow). A taskID matching no row
// updates 0 rows without a Go error, but recordStepTransition then ran with
// fromWorkflowStepID="" (readTaskStepInTx's not-found result) paired with a
// non-empty toWorkflowStepID — the no-op guard never triggers on that pair —
// so the INSERT was attempted anyway and failed the ledger's FK to tasks,
// turning what was a harmless no-op before the ledger existed into a hard
// transaction-failing error. Must be a benign no-op instead.
func TestAddTaskToWorkflowNonexistentTaskIsNoOp(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()

	if err := repo.AddTaskToWorkflow(ctx, "task-does-not-exist", "wf-2", "step-x", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow on a nonexistent task: %v, want nil (benign no-op)", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-does-not-exist")
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestRemoveTaskFromWorkflowWritesWorkflowDetachedRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-detach", "wf-1", "step-a")

	if err := repo.RemoveTaskFromWorkflow(ctx, "task-detach", "wf-1"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-detach")
	last := rows[len(rows)-1]
	if last.toWorkflowID != nil || last.toWorkflowStepID != nil {
		t.Fatalf("detach row to_* = (%v, %v), want (nil, nil)", last.toWorkflowID, last.toWorkflowStepID)
	}
	if last.fromWorkflowStepID == nil || *last.fromWorkflowStepID != "step-a" {
		t.Fatalf("from_workflow_step_id = %v, want step-a", last.fromWorkflowStepID)
	}
	// RemoveTaskFromWorkflow hardcodes this trigger the way the genesis row
	// hardcodes task_created — it has exactly one semantic meaning regardless
	// of caller, and (unlike AddTaskToWorkflow) has no production caller today
	// to supply it externally. A bare context.Background() here must still
	// produce workflow_detached, not unknown.
	if last.trigger != string(steptelemetry.TriggerWorkflowDetached) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerWorkflowDetached)
	}
}

func TestRemoveTaskFromWorkflowWrongWorkflowIDWritesNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-detach-mismatch", "wf-1", "step-a")

	before := stepTransitionRowsForTask(t, repo, "task-detach-mismatch")
	if err := repo.RemoveTaskFromWorkflow(ctx, "task-detach-mismatch", "wf-does-not-match"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}
	after := stepTransitionRowsForTask(t, repo, "task-detach-mismatch")
	if len(after) != len(before) {
		t.Fatalf("rows after mismatched detach = %d, want unchanged %d", len(after), len(before))
	}
}

func TestEphemeralTaskRecordsExactlyAsAnyOther(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := &models.Task{
		ID: "task-ephemeral", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-a",
		Title: "Quick chat", Priority: "medium", IsEphemeral: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-ephemeral")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (genesis + move), ephemeral tasks are recorded like any other", len(rows))
	}
}

func TestRestoreTaskMessageRollbackWritesUnarchiveRestoreRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	// RestoreTaskMessageRollbackIfSessionState does not hardcode its trigger —
	// its production caller (task/service/service_tasks.go RestoreTaskMessageRollback)
	// sets TriggerUnarchiveRestore before calling. Wrap ctx here the way that
	// caller does, per this file's header comment on exercising attribution.
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerUnarchiveRestore,
		ActorKind: steptelemetry.ActorSystem,
	})
	task := createStepTransitionsTestTask(t, repo, "task-rollback", "wf-1", "step-a")

	// Seed a session in the expected state directly.
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-rollback", "task-rollback", "RUNNING", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	task.WorkflowStepID = "step-restored"
	restored, err := repo.RestoreTaskMessageRollbackIfSessionState(ctx, task, "session-rollback", models.TaskSessionStateRunning)
	if err != nil {
		t.Fatalf("RestoreTaskMessageRollbackIfSessionState: %v", err)
	}
	if !restored {
		t.Fatal("expected restore to apply")
	}

	rows := stepTransitionRowsForTask(t, repo, "task-rollback")
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-restored" {
		t.Fatalf("to_workflow_step_id = %v, want step-restored", last.toWorkflowStepID)
	}
	if last.trigger != string(steptelemetry.TriggerUnarchiveRestore) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerUnarchiveRestore)
	}
}

func TestRestoreTaskMessageRollbackWrongSessionStateWritesNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-rollback-mismatch", "wf-1", "step-a")

	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-mismatch", "task-rollback-mismatch", "COMPLETED", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	before := stepTransitionRowsForTask(t, repo, "task-rollback-mismatch")
	task.WorkflowStepID = "step-restored"
	restored, err := repo.RestoreTaskMessageRollbackIfSessionState(ctx, task, "session-mismatch", models.TaskSessionStateRunning)
	if err != nil {
		t.Fatalf("RestoreTaskMessageRollbackIfSessionState: %v", err)
	}
	if restored {
		t.Fatal("expected restore to be rejected (session state mismatch)")
	}
	after := stepTransitionRowsForTask(t, repo, "task-rollback-mismatch")
	if len(after) != len(before) {
		t.Fatalf("rows after rejected rollback = %d, want unchanged %d", len(after), len(before))
	}
}
