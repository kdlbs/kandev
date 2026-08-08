package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// The three cancel entry points (CancelRun, BulkCancelRuns,
// CancelRunsForTasks) are thin selectors over one guarded writer, so these
// tests assert the shared guard through each of them: a run may only move to
// cancelled from queued or claimed, and a run that already reached a terminal
// state keeps its status, cancel_reason and finished_at.

// seedCancelRun inserts a run for taskID in the given status. When finishedAt is
// non-nil it is stamped onto the row, so terminal fixtures carry the
// finished_at a cancel must not overwrite.
func seedCancelRun(
	t *testing.T,
	repo *sqlite.Repository,
	taskID, status string,
	cancelReason *string,
	finishedAt *time.Time,
) string {
	t.Helper()
	ctx := context.Background()
	run := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"` + taskID + `"}`,
		Status:         models.RunStatus(status),
		CoalescedCount: 1,
		CancelReason:   cancelReason,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run (%s): %v", status, err)
	}
	if finishedAt != nil {
		if err := repo.SetRunStatusForTest(ctx, run.ID, status, nil, finishedAt); err != nil {
			t.Fatalf("stamp finished_at: %v", err)
		}
	}
	return run.ID
}

func fetchRun(t *testing.T, repo *sqlite.Repository, id string) *models.Run {
	t.Helper()
	run, err := repo.GetRunByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get run %s: %v", id, err)
	}
	return run
}

func TestCancelRun_QueuedRunIsCancelled(t *testing.T) {
	repo := newTestRepo(t)
	id := seedCancelRun(t, repo, "t1", "queued", nil, nil)

	if err := repo.CancelRun(context.Background(), id, "run_too_old"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	run := fetchRun(t, repo, id)
	if run.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", run.Status)
	}
	if run.CancelReason == nil || *run.CancelReason != "run_too_old" {
		t.Errorf("cancel_reason = %v, want run_too_old", run.CancelReason)
	}
	if run.FinishedAt == nil {
		t.Error("finished_at = nil, want a timestamp")
	}
}

func TestCancelRun_ClaimedRunIsCancelled(t *testing.T) {
	repo := newTestRepo(t)
	id := seedCancelRun(t, repo, "t1", "claimed", nil, nil)

	if err := repo.CancelRun(context.Background(), id, "execution_too_old"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	run := fetchRun(t, repo, id)
	if run.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", run.Status)
	}
	if run.CancelReason == nil || *run.CancelReason != "execution_too_old" {
		t.Errorf("cancel_reason = %v, want execution_too_old", run.CancelReason)
	}
	if run.FinishedAt == nil {
		t.Error("finished_at = nil, want a timestamp")
	}
}

// A run that reached a terminal state between the caller's SELECT and this
// UPDATE must keep its own outcome — this is the race the guard closes.
func TestCancelRun_TerminalRunIsUntouched(t *testing.T) {
	finished := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name         string
		status       string
		cancelReason *string
	}{
		{name: "finished", status: "finished"},
		{name: "failed", status: "failed"},
		{name: "already cancelled", status: "cancelled", cancelReason: strPtr("tree_cancelled")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepo(t)
			id := seedCancelRun(t, repo, "t1", tc.status, tc.cancelReason, &finished)

			if err := repo.CancelRun(context.Background(), id, "task_reassigned"); err != nil {
				t.Fatalf("cancel: %v", err)
			}

			run := fetchRun(t, repo, id)
			if string(run.Status) != tc.status {
				t.Errorf("status = %q, want %q", run.Status, tc.status)
			}
			if run.FinishedAt == nil {
				t.Fatal("finished_at = nil, want the original timestamp preserved")
			}
			if !run.FinishedAt.UTC().Equal(finished) {
				t.Errorf("finished_at = %v, want the original %v", run.FinishedAt.UTC(), finished)
			}
			switch {
			case tc.cancelReason == nil && run.CancelReason != nil:
				t.Errorf("cancel_reason = %q, want unset", *run.CancelReason)
			case tc.cancelReason != nil && run.CancelReason == nil:
				t.Errorf("cancel_reason = nil, want the original %q", *tc.cancelReason)
			case tc.cancelReason != nil && *run.CancelReason != *tc.cancelReason:
				t.Errorf("cancel_reason = %q, want the original %q", *run.CancelReason, *tc.cancelReason)
			}
		})
	}
}

// CancelRunsWhere is the writer the three entry points share; assert the
// rows-affected contract directly on it.
func TestCancelRunsWhere_RowsAffected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	finished := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	queued := seedCancelRun(t, repo, "t1", "queued", nil, nil)
	terminal := seedCancelRun(t, repo, "t1", "finished", nil, &finished)

	rows, err := repo.CancelRunsWhere(ctx, "tree_cancelled", `id = ?`, terminal)
	if err != nil {
		t.Fatalf("cancel terminal: %v", err)
	}
	if rows != 0 {
		t.Errorf("rows affected = %d, want 0 for an already-terminal run", rows)
	}

	rows, err = repo.CancelRunsWhere(ctx, "tree_cancelled", `id = ?`, queued)
	if err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows affected = %d, want 1", rows)
	}
}

func TestBulkCancelRuns_OnlyCancelsEligibleRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	finished := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	queued := seedCancelRun(t, repo, "t1", "queued", nil, nil)
	claimed := seedCancelRun(t, repo, "t1", "claimed", nil, nil)
	done := seedCancelRun(t, repo, "t1", "finished", nil, &finished)

	if err := repo.BulkCancelRuns(ctx, []string{queued, claimed, done}, "task_reassigned"); err != nil {
		t.Fatalf("bulk cancel: %v", err)
	}

	for _, id := range []string{queued, claimed} {
		if run := fetchRun(t, repo, id); run.Status != "cancelled" {
			t.Errorf("run %s status = %q, want cancelled", id, run.Status)
		}
	}
	run := fetchRun(t, repo, done)
	if run.Status != "finished" {
		t.Errorf("finished run status = %q, want finished", run.Status)
	}
	if run.FinishedAt == nil || !run.FinishedAt.UTC().Equal(finished) {
		t.Errorf("finished run finished_at = %v, want the original %v", run.FinishedAt, finished)
	}
}

func TestCancelRunsForTasks_MixedSetCountMatches(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	finished := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	queued := seedCancelRun(t, repo, "t1", "queued", nil, nil)
	claimed := seedCancelRun(t, repo, "t1", "claimed", nil, nil)
	done := seedCancelRun(t, repo, "t1", "finished", nil, &finished)

	count, err := repo.CancelRunsForTasks(ctx, []string{"t1"}, "tree_paused")
	if err != nil {
		t.Fatalf("cancel for tasks: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (queued + claimed, not the finished run)", count)
	}
	for _, id := range []string{queued, claimed} {
		if run := fetchRun(t, repo, id); run.Status != "cancelled" {
			t.Errorf("run %s status = %q, want cancelled", id, run.Status)
		}
	}
	if run := fetchRun(t, repo, done); run.Status != "finished" {
		t.Errorf("finished run status = %q, want finished", run.Status)
	}
}

// The by-task-id selector matches on the payload's task_id, not the run ID.
func TestCancelRunsForTasks_MatchesPayloadTaskID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	mine := seedCancelRun(t, repo, "t1", "queued", nil, nil)
	other := seedCancelRun(t, repo, "t2", "queued", nil, nil)

	count, err := repo.CancelRunsForTasks(ctx, []string{"t1"}, "tree_cancelled")
	if err != nil {
		t.Fatalf("cancel for tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if run := fetchRun(t, repo, mine); run.Status != "cancelled" {
		t.Errorf("t1 run status = %q, want cancelled", run.Status)
	}
	if run := fetchRun(t, repo, other); run.Status != "queued" {
		t.Errorf("t2 run status = %q, want queued", run.Status)
	}
}

// An empty slice is a no-op: no error, nothing cancelled. This pins the
// observable contract, not the short-circuit itself — SQLite accepts an empty
// "id IN ()" list, so removing the early return still passes here. The early
// return still matters: that same list is a syntax error on Postgres.
func TestCancelPaths_EmptySliceIsANoOp(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := seedCancelRun(t, repo, "t1", "queued", nil, nil)

	if err := repo.BulkCancelRuns(ctx, nil, "task_reassigned"); err != nil {
		t.Errorf("bulk cancel with no ids: %v", err)
	}
	count, err := repo.CancelRunsForTasks(ctx, nil, "tree_cancelled")
	if err != nil {
		t.Errorf("cancel for tasks with no task ids: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if run := fetchRun(t, repo, id); run.Status != "queued" {
		t.Errorf("unrelated run status = %q, want queued", run.Status)
	}
}
