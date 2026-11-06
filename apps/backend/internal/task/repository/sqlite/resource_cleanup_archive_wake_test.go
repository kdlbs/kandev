package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type waitingArchiveReclaimWaker interface {
	WakeWaitingArchiveReclaimJobs(context.Context, time.Time, string, int) (string, bool, error)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
func TestWakeWaitingArchiveReclaimJobs(t *testing.T) {
	ctx := t.Context()
	repo := newRepoForEntityTests(t)
	waker, ok := any(repo).(waitingArchiveReclaimWaker)
	if !ok {
		t.Fatal("repository cannot wake legacy archive reclaim jobs")
	}
	cutoff := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	future := cutoff.Add(24 * time.Hour)
	for _, item := range []struct {
		id      string
		trigger models.TaskResourceCleanupTrigger
		state   models.TaskResourceCleanupState
		fresh   bool
	}{
		{"a", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateWaitingForClean, false},
		{"b", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateWaitingForClean, false},
		{"cancelled", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateCancelled, false},
		{"running", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateRunning, false},
		{"succeeded", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateSucceeded, false},
		{"retry", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateRetryWait, false},
		{"ordinary", models.TaskResourceCleanupTriggerArchive, models.TaskResourceCleanupStateWaitingForClean, false},
		{"fresh", models.TaskResourceCleanupTriggerArchiveReclaim, models.TaskResourceCleanupStateWaitingForClean, true},
	} {
		job := &models.TaskResourceCleanupJob{ID: item.id, OperationID: item.id, TaskID: "legacy-owner", Trigger: item.trigger, State: item.state, ResourceSnapshot: `{"archive_identity":"unchanged"}`, Attempts: 3, NextAttemptAt: &future}
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatal(err)
		}
		updated := cutoff.Add(-time.Hour)
		if item.fresh {
			updated = cutoff.Add(time.Hour)
		}
		if _, err := repo.db.ExecContext(ctx, `UPDATE task_resource_cleanup_jobs SET updated_at = ? WHERE id = ?`, updated, item.id); err != nil {
			t.Fatal(err)
		}
	}
	cursor, done, err := waker.WakeWaitingArchiveReclaimJobs(ctx, cutoff, "", 1)
	if err != nil || done || cursor != "a" {
		t.Fatalf("first page = %q, %v, %v", cursor, done, err)
	}
	// A concurrent claim wins over a subsequent startup pass.
	if claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, "a"); err != nil || !claimed {
		t.Fatalf("claim = %v: %v", claimed, err)
	}
	cursor, _, err = waker.WakeWaitingArchiveReclaimJobs(ctx, cutoff, cursor, 1)
	if err != nil || cursor != "b" {
		t.Fatalf("second page = %q: %v", cursor, err)
	}
	if _, done, err := waker.WakeWaitingArchiveReclaimJobs(ctx, cutoff, cursor, 1); err != nil || !done {
		t.Fatalf("last page = %v: %v", done, err)
	}
	for id, want := range map[string]models.TaskResourceCleanupState{"a": models.TaskResourceCleanupStateRunning, "b": models.TaskResourceCleanupStatePending, "cancelled": models.TaskResourceCleanupStateCancelled, "running": models.TaskResourceCleanupStateRunning, "succeeded": models.TaskResourceCleanupStateSucceeded, "retry": models.TaskResourceCleanupStateRetryWait, "ordinary": models.TaskResourceCleanupStateWaitingForClean, "fresh": models.TaskResourceCleanupStateWaitingForClean} {
		job, err := repo.GetTaskResourceCleanupJob(ctx, id)
		if err != nil || job.State != want {
			t.Fatalf("%s = %+v: %v", id, job, err)
		}
		if id == "b" && (job.Attempts != 3 || job.ResourceSnapshot != `{"archive_identity":"unchanged"}` || job.NextAttemptAt != nil) {
			t.Fatalf("wake altered cleanup intent: %+v", job)
		}
	}
}
