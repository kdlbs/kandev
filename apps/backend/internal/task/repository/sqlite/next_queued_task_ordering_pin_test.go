package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestNextQueuedTaskForStepExcluding_FullPromotionOrderLadder pins
// AC-PLUGINS-STEP-MOVE-005.2: queued-task promotion (the query a plugin
// move's vacate-and-reconcile step relies on, same as the board's own move)
// orders candidates by position, then priority tier, then queued_at falling
// back to created_at, then created_at, then task id — exactly the ladder the
// design file names for NextQueuedTaskForStepExcluding. Each candidate below
// differs from the next by exactly one key so the whole ladder is exercised
// in one pass rather than only its first tiebreaker.
func TestNextQueuedTaskForStepExcluding_FullPromotionOrderLadder(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-order")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-order", WorkspaceID: "workspace-order", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.April, 5, 6, 7, 8, 0, time.UTC)
	for _, stepID := range []string{"step-feeder", "step-destination"} {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
			(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`),
			stepID, "workflow-order", stepID, 0, now, now); err != nil {
			t.Fatalf("insert step %s: %v", stepID, err)
		}
	}

	// Rank 0 (best): position 0 beats every other candidate regardless of
	// its own priority/time fields.
	mustCreateOrderedTask(t, ctx, repo, "task-position-wins", "step-feeder", 0, "low", now.Add(time.Hour))
	// Among position 1 candidates, priority tier is the next key: critical
	// beats medium beats none, independent of timestamps.
	mustCreateOrderedTask(t, ctx, repo, "task-priority-critical", "step-feeder", 1, "critical", now.Add(2*time.Hour))
	mustCreateOrderedTask(t, ctx, repo, "task-priority-medium-early", "step-feeder", 1, "medium", now.Add(3*time.Hour))
	mustCreateOrderedTask(t, ctx, repo, "task-priority-none", "step-feeder", 1, "none", now)
	// Among position 1 / priority medium candidates, queued_at (falling back
	// to created_at) is the next key: an explicit earlier queued_at wins over
	// a later one even though this task's row was created later.
	mustCreateOrderedTask(t, ctx, repo, "task-priority-medium-late", "step-feeder", 1, "medium", now.Add(4*time.Hour))
	setQueuedAt(t, ctx, repo, "task-priority-medium-early", "step-destination", now.Add(90*time.Minute))
	setQueuedAt(t, ctx, repo, "task-priority-medium-late", "step-destination", now.Add(150*time.Minute))
	// Among position 1 / priority medium / equal queued_at candidates,
	// created_at is the next key.
	sameQueuedAt := now.Add(5 * time.Hour)
	mustCreateOrderedTaskWithCreatedAt(t, ctx, repo, "task-created-later", "step-feeder", 1, "medium", now.Add(6*time.Hour))
	mustCreateOrderedTaskWithCreatedAt(t, ctx, repo, "task-created-earlier", "step-feeder", 1, "medium", now.Add(5*time.Hour))
	setQueuedAt(t, ctx, repo, "task-created-later", "step-destination", sameQueuedAt)
	setQueuedAt(t, ctx, repo, "task-created-earlier", "step-destination", sameQueuedAt)
	// Among position 1 / priority medium / equal queued_at / equal created_at
	// candidates, task id is the final tiebreaker (lexical ASC).
	sameCreatedAt := now.Add(7 * time.Hour)
	mustCreateOrderedTaskWithCreatedAt(t, ctx, repo, "task-id-z", "step-feeder", 1, "medium", sameCreatedAt)
	mustCreateOrderedTaskWithCreatedAt(t, ctx, repo, "task-id-a", "step-feeder", 1, "medium", sameCreatedAt)
	setQueuedAt(t, ctx, repo, "task-id-z", "step-destination", sameQueuedAt.Add(time.Hour))
	setQueuedAt(t, ctx, repo, "task-id-a", "step-destination", sameQueuedAt.Add(time.Hour))

	want := []string{
		"task-position-wins",
		"task-priority-critical",
		"task-priority-medium-early",
		"task-priority-medium-late",
		"task-created-earlier",
		"task-created-later",
		"task-id-a",
		"task-id-z",
		"task-priority-none",
	}
	var got []string
	excluded := []string{}
	for range want {
		candidate, err := repo.NextQueuedTaskForStepExcluding(ctx, "step-feeder", "step-destination", excluded)
		if err != nil {
			t.Fatalf("NextQueuedTaskForStepExcluding: %v", err)
		}
		if candidate == nil {
			t.Fatalf("NextQueuedTaskForStepExcluding returned nil after %v", got)
		}
		got = append(got, candidate.ID)
		excluded = append(excluded, candidate.ID)
	}
	if len(got) != len(want) {
		t.Fatalf("promotion order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("promotion order = %v, want %v (mismatch at index %d)", got, want, i)
		}
	}
}

func mustCreateOrderedTask(t *testing.T, ctx context.Context, repo *Repository, id, stepID string, position int, priority string, createdAt time.Time) {
	t.Helper()
	mustCreateOrderedTaskWithCreatedAt(t, ctx, repo, id, stepID, position, priority, createdAt)
}

func mustCreateOrderedTaskWithCreatedAt(t *testing.T, ctx context.Context, repo *Repository, id, stepID string, position int, priority string, createdAt time.Time) {
	t.Helper()
	task := &models.Task{
		ID:             id,
		WorkspaceID:    "workspace-order",
		WorkflowID:     "workflow-order",
		WorkflowStepID: stepID,
		Title:          id,
		Priority:       priority,
		Position:       position,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET created_at = ? WHERE id = ?`), createdAt, id); err != nil {
		t.Fatalf("backdate created_at(%s): %v", id, err)
	}
}

// setQueuedAt marks a task as queued for destinationStepID with an explicit
// queued_at, mirroring the state UpdateTaskIfWorkflowStepHasCapacity leaves
// an overflowed move in.
func setQueuedAt(t *testing.T, ctx context.Context, repo *Repository, id, destinationStepID string, queuedAt time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE tasks SET queued_for_step_id = ?, queued_at = ?, wip_admitted = 0 WHERE id = ?`),
		destinationStepID, queuedAt, id); err != nil {
		t.Fatalf("setQueuedAt(%s): %v", id, err)
	}
	_ = ctx
}
