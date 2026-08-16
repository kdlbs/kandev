package sqlite

// Ordering, chain, and concurrency coverage: reading a task's rows in
// (occurred_at, id) order, each row's from equals the previous row's to, and
// the last row's to equals the task's current step — including under
// concurrent moves, where the chain invariant is what the writer's
// read-inside-the-transaction discipline (readTaskStepInTx) exists to
// protect (see step_transitions.go).

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func assertChainIntact(t *testing.T, repo *Repository, taskID string) []stepTransitionRow {
	t.Helper()
	rows := stepTransitionRowsForTask(t, repo, taskID)
	for i := 1; i < len(rows); i++ {
		prevTo := ""
		if rows[i-1].toWorkflowStepID != nil {
			prevTo = *rows[i-1].toWorkflowStepID
		}
		curFrom := ""
		if rows[i].fromWorkflowStepID != nil {
			curFrom = *rows[i].fromWorkflowStepID
		}
		if prevTo != curFrom {
			t.Fatalf("chain broken at row %d: previous to_workflow_step_id = %q, this row's from_workflow_step_id = %q", i, prevTo, curFrom)
		}
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		task, err := repo.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		lastTo := ""
		if last.toWorkflowStepID != nil {
			lastTo = *last.toWorkflowStepID
		}
		if lastTo != task.WorkflowStepID {
			t.Fatalf("last row's to_workflow_step_id = %q, want it to equal tasks.workflow_step_id = %q", lastTo, task.WorkflowStepID)
		}
	}
	return rows
}

func TestChainABABYieldsFourRowsWithIntactChain(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-chain", "wf-1", "step-a")

	moves := []string{"step-b", "step-a", "step-b"}
	for _, step := range moves {
		task.WorkflowStepID = step
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("UpdateTask to %s: %v", step, err)
		}
	}

	rows := assertChainIntact(t, repo, "task-chain")
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (genesis + 3 moves)", len(rows))
	}
}

func TestChainRowsSharingOccurredAtAreTotallyOrderedByID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-chain-tiebreak", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	task.WorkflowStepID = "step-c"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	// Force two rows to share an occurred_at value directly, then confirm
	// reading ordered by (occurred_at, id) still produces a strictly
	// increasing id sequence — id is the tiebreak, not insertion luck.
	rows := stepTransitionRowsForTask(t, repo, "task-chain-tiebreak")
	if len(rows) < 2 {
		t.Fatalf("rows = %d, want at least 2", len(rows))
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_step_transitions SET occurred_at = ? WHERE id = ?`), rows[0].occurredAt, rows[1].id); err != nil {
		t.Fatalf("force shared occurred_at: %v", err)
	}

	reread := stepTransitionRowsForTask(t, repo, "task-chain-tiebreak")
	for i := 1; i < len(reread); i++ {
		if reread[i].id <= reread[i-1].id {
			t.Fatalf("row %d id = %d, want strictly greater than previous id %d", i, reread[i].id, reread[i-1].id)
		}
	}
}

func TestChainSurvivesBackwardsClockCorrectionWhenOrderedByID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-chain-clock", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask to step-b: %v", err)
	}
	task.WorkflowStepID = "step-c"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask to step-c: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, "task-chain-clock")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (genesis + 2 moves)", len(rows))
	}

	// Simulate a host clock correction: force the most recent row's
	// occurred_at to be earlier than the genesis row's, without touching
	// its id.
	corrected := rows[0].occurredAt.Add(-1 * time.Hour)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_step_transitions SET occurred_at = ? WHERE id = ?`), corrected, rows[2].id); err != nil {
		t.Fatalf("force backwards occurred_at: %v", err)
	}

	idOrdered, err := repo.db.Query(repo.db.Rebind(`
		SELECT id, from_workflow_step_id, to_workflow_step_id, occurred_at
		FROM task_step_transitions WHERE task_id = ? ORDER BY id ASC
	`), "task-chain-clock")
	if err != nil {
		t.Fatalf("query ordered by id: %v", err)
	}
	defer func() { _ = idOrdered.Close() }()

	var reread []stepTransitionRow
	for idOrdered.Next() {
		var r stepTransitionRow
		if err := idOrdered.Scan(&r.id, &r.fromWorkflowStepID, &r.toWorkflowStepID, &r.occurredAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		reread = append(reread, r)
	}
	if err := idOrdered.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(reread) != 3 {
		t.Fatalf("reread rows = %d, want 3", len(reread))
	}

	// The chain invariant (from == previous row's to) still holds when read
	// in id order, even though occurred_at now runs backwards.
	for i := 1; i < len(reread); i++ {
		prevTo := ""
		if reread[i-1].toWorkflowStepID != nil {
			prevTo = *reread[i-1].toWorkflowStepID
		}
		curFrom := ""
		if reread[i].fromWorkflowStepID != nil {
			curFrom = *reread[i].fromWorkflowStepID
		}
		if prevTo != curFrom {
			t.Fatalf("chain broken at row %d: previous to = %q, this row's from = %q", i, prevTo, curFrom)
		}
	}

	// The ledger is not repaired or reordered by timestamp: the forced
	// backwards value survives untouched.
	if !reread[2].occurredAt.Equal(corrected) {
		t.Fatalf("last row's occurred_at = %v, want unrepaired value %v", reread[2].occurredAt, corrected)
	}
	if !reread[2].occurredAt.Before(reread[0].occurredAt) {
		t.Fatalf("expected last row's occurred_at %v to remain before genesis row's %v (clock ran backwards, uncorrected)", reread[2].occurredAt, reread[0].occurredAt)
	}
}

func TestChainConcurrentMovesProduceTwoRowsWithIntactChain(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	task := createStepTransitionsTestTask(t, repo, "task-concurrent", "wf-1", "step-a")

	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		moved := *task
		moved.WorkflowStepID = "step-b"
		return repo.UpdateTask(ctx, &moved)
	})
	g.Go(func() error {
		moved := *task
		moved.WorkflowStepID = "step-c"
		return repo.UpdateTask(ctx, &moved)
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent UpdateTask: %v", err)
	}

	rows := assertChainIntact(t, repo, "task-concurrent")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (genesis + 2 concurrent moves)", len(rows))
	}
}
