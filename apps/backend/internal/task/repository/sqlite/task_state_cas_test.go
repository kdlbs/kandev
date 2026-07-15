package sqlite

import (
	"context"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestUpdateTaskStateIfCurrentIn_SkipsArchivedTask reproduces the TOCTOU race
// flagged in PR #1706 review: a caller's earlier archived-state guard (a
// plain GetTask read) can observe archived_at == NULL, then have ArchiveTask
// commit before the caller's CAS write lands. Because state is untouched by
// ArchiveTask, the old `WHERE id = ? AND state = ?` clause alone would still
// match and resurrect the task to REVIEW. The archived_at IS NULL clause
// added to the UPDATE closes that window: the write becomes a no-op even
// though currentState (read moments earlier, inside the same transaction)
// was still in the allowed set.
func TestUpdateTaskStateIfCurrentIn_SkipsArchivedTask(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-archived-race")

	if err := repo.UpdateTaskState(ctx, "task-archived-race", v1.TaskStateInProgress); err != nil {
		t.Fatalf("seed IN_PROGRESS: %v", err)
	}

	// Simulates the archive committing in the race window between the
	// caller's taskArchived() guard read and this CAS call.
	if err := repo.ArchiveTask(ctx, "task-archived-race"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-archived-race", v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if updated {
		t.Fatal("expected archived task's state to be left untouched, got updated=true")
	}
	if gotState != v1.TaskStateInProgress {
		t.Errorf("returned currentState = %q, want %q (pre-CAS read)", gotState, v1.TaskStateInProgress)
	}

	task, err := repo.GetTask(ctx, "task-archived-race")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Errorf("persisted state = %q, want %q (archived task must not resurrect to REVIEW)", task.State, v1.TaskStateInProgress)
	}
	if task.ArchivedAt == nil {
		t.Error("expected task to remain archived")
	}
}

// TestUpdateTaskStateIfCurrentIn_UpdatesWhenNotArchived is the CAS positive
// path sanity check: a non-archived task whose state is in the allowed set
// still transitions normally. Guards against a too-broad archived_at fix
// (e.g. accidentally scoping the WHERE clause to always require archived_at
// IS NULL on every row, including ones with no archive concept at all) that
// would silently break every ordinary REVIEW/IN_PROGRESS transition.
func TestUpdateTaskStateIfCurrentIn_UpdatesWhenNotArchived(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "task-normal")

	if err := repo.UpdateTaskState(ctx, "task-normal", v1.TaskStateInProgress); err != nil {
		t.Fatalf("seed IN_PROGRESS: %v", err)
	}

	gotState, updated, err := repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-normal", v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfCurrentIn: %v", err)
	}
	if !updated {
		t.Fatal("expected non-archived task in the allowed set to transition, got updated=false")
	}
	if gotState != v1.TaskStateInProgress {
		t.Errorf("returned currentState = %q, want %q", gotState, v1.TaskStateInProgress)
	}

	task, err := repo.GetTask(ctx, "task-normal")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Errorf("persisted state = %q, want %q", task.State, v1.TaskStateReview)
	}
}
