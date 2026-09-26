package sqlite

// Lifecycle coverage for task_step_transitions: archive/unarchive/delete
// paths that never touch workflow_step_id write no row, a row survives
// deletion of the workflow step it names (no FK), the missing-table case
// fails the transaction rather than committing the step change alone, and
// cascade behavior on task/session deletion.

import (
	"context"
	"testing"
	"time"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestArchiveUnarchiveDeleteWriteNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()

	createStepTransitionsTestTask(t, repo, "task-archive", "wf-1", "step-a")
	before := stepTransitionRowsForTask(t, repo, "task-archive")

	if err := repo.ArchiveTask(ctx, "task-archive"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	afterArchive := stepTransitionRowsForTask(t, repo, "task-archive")
	if len(afterArchive) != len(before) {
		t.Fatalf("rows after archive = %d, want unchanged %d", len(afterArchive), len(before))
	}

	if _, err := repo.UnarchiveTask(ctx, "task-archive"); err != nil {
		t.Fatalf("UnarchiveTask: %v", err)
	}
	afterUnarchive := stepTransitionRowsForTask(t, repo, "task-archive")
	if len(afterUnarchive) != len(before) {
		t.Fatalf("rows after unarchive = %d, want unchanged %d", len(afterUnarchive), len(before))
	}
}

func TestArchiveTaskIfActiveWritesNoRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()

	createStepTransitionsTestTask(t, repo, "task-cascade-archive", "wf-1", "step-a")
	before := stepTransitionRowsForTask(t, repo, "task-cascade-archive")

	archived, err := repo.ArchiveTaskIfActive(ctx, "task-cascade-archive", "cascade-1")
	if err != nil {
		t.Fatalf("ArchiveTaskIfActive: %v", err)
	}
	if !archived {
		t.Fatal("expected ArchiveTaskIfActive to report the task as archived")
	}

	after := stepTransitionRowsForTask(t, repo, "task-cascade-archive")
	if len(after) != len(before) {
		t.Fatalf("rows after cascade archive = %d, want unchanged %d", len(after), len(before))
	}
}

func TestDeleteTaskCascadesLedgerRows(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-delete", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-delete")
	if len(rows) == 0 {
		t.Fatal("expected a genesis row before delete")
	}

	if err := repo.DeleteTask(ctx, "task-delete"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	after := stepTransitionRowsForTask(t, repo, "task-delete")
	if len(after) != 0 {
		t.Fatalf("rows after task delete = %d, want 0 (cascade)", len(after))
	}
}

func TestLedgerRowSurvivesSessionDeletionWithSessionIDNull(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)

	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-session-cascade", "ws-1", "Task", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-cascade", "task-session-cascade", "RUNNING", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_step_transitions
			(task_id, session_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), "task-session-cascade", "session-cascade", "wf-1", "step-a", "manual_move", "agent", 1, time.Now().UTC()); err != nil {
		t.Fatalf("seed ledger row: %v", err)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), "session-cascade"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-session-cascade")
	if len(rows) != 1 {
		t.Fatalf("rows after session delete = %d, want 1 (row survives)", len(rows))
	}
	if rows[0].sessionID != nil {
		t.Fatalf("session_id after session delete = %v, want nil", rows[0].sessionID)
	}
}

func TestLedgerRowSurvivesDeletionOfTheWorkflowStepItNames(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "task-step-deleted", "wf-1", "step-doomed")

	// There is deliberately no FK to workflow_steps, so nothing here needs to
	// "delete a step" — the point is simply that the row's step reference
	// keeps working with no matching workflow_steps row at all.
	rows := stepTransitionRowsForTask(t, repo, "task-step-deleted")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].toWorkflowStepID == nil || *rows[0].toWorkflowStepID != "step-doomed" {
		t.Fatalf("to_workflow_step_id = %v, want step-doomed (survives with no matching workflow_steps row)", rows[0].toWorkflowStepID)
	}
}

func TestMissingLedgerTableFailsTransactionRatherThanCommittingStepChangeAlone(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-missing-table", "wf-1", "step-a")

	if _, err := repo.db.Exec(`DROP TABLE task_step_transitions`); err != nil {
		t.Fatalf("drop task_step_transitions: %v", err)
	}

	task.WorkflowStepID = "step-b"
	err := repo.UpdateTask(ctx, task)
	if err == nil {
		t.Fatal("expected UpdateTask to fail when the ledger table is missing")
	}
	if !dbutil.IsMissingTableError(err) {
		t.Fatalf("error = %v, want a missing-table error", err)
	}

	reread, err := repo.GetTask(ctx, "task-missing-table")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reread.WorkflowStepID != "step-a" {
		t.Fatalf("task step after failed transaction = %q, want unchanged %q (rollback)", reread.WorkflowStepID, "step-a")
	}
}

// TestRolledBackTransactionLeavesNoLedgerRow proves the spec's other rollback
// scenario: unlike the test above (the ledger INSERT itself fails), here the
// step UPDATE and the ledger INSERT both succeed, and only a LATER statement
// in the same caller-owned transaction fails — UpdateTask's runner-participant
// sync (syncRunnerInTx), the concrete case the plan's own scope decision #4
// names. The whole transaction must still roll back: no ledger row, and the
// task's workflow_step_id unchanged. TestMissingLedgerTableFailsTransactionRatherThanCommittingStepChangeAlone
// does not cover this — it proves the inverse (ledger insert fails).
func TestRolledBackTransactionLeavesNoLedgerRow(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()

	task := createStepTransitionsTestTask(t, repo, "task-rollback", "wf-1", "step-a")
	task.AssigneeAgentProfileID = "agent-profile-1"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("seed UpdateTask: %v", err)
	}

	// Fires after UpdateTask's own recordStepTransition call (which runs
	// before syncRunnerInTx), forcing the INSERT into
	// workflow_step_participants that upsertRunnerInTx issues for this
	// step/task/agent-profile combination to fail.
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_runner_participant_insert
		BEFORE INSERT ON workflow_step_participants
		BEGIN SELECT RAISE(ABORT, 'injected runner-sync failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	before := stepTransitionRowsForTask(t, repo, "task-rollback")

	task.WorkflowStepID = "step-b"
	err := repo.UpdateTask(ctx, task)
	if err == nil {
		t.Fatal("expected UpdateTask to fail from the injected runner-sync failure")
	}

	after := stepTransitionRowsForTask(t, repo, "task-rollback")
	if len(after) != len(before) {
		t.Fatalf("rows after rolled-back transaction = %d, want unchanged %d (the ledger INSERT that succeeded mid-transaction must roll back with everything else)", len(after), len(before))
	}

	reread, err := repo.GetTask(ctx, "task-rollback")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reread.WorkflowStepID != "step-a" {
		t.Fatalf("task step after rolled-back transaction = %q, want unchanged %q", reread.WorkflowStepID, "step-a")
	}
}
