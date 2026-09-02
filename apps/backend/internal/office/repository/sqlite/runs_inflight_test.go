package sqlite_test

import (
	"context"
	"testing"
	"time"
)

// HasInFlightRunForTask is the false-positive guard of the Office
// decision-waiting detector (REQ-OFFICE-STALL-VISIBILITY-002): a task with a
// queued or claimed run is being worked on, so it must never be surfaced as
// waiting on a decision. These tests pin both halves of that predicate — the
// two in-flight statuses answer true, every terminal status answers false —
// because a guard that silently answers false for `claimed` turns the whole
// detector into a false-positive generator.

func TestHasInFlightRunForTask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	cases := []struct {
		status string
		want   bool
	}{
		{"queued", true},
		{"claimed", true},
		{"finished", false},
		{"failed", false},
		{"cancelled", false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			taskID := "task-" + tc.status
			finishedAt := time.Now().UTC()
			var stamp *time.Time
			if !tc.want {
				stamp = &finishedAt
			}
			seedCancelRun(t, repo, taskID, tc.status, nil, stamp)

			got, err := repo.HasInFlightRunForTask(ctx, taskID)
			if err != nil {
				t.Fatalf("HasInFlightRunForTask(%s): %v", tc.status, err)
			}
			if got != tc.want {
				t.Errorf("HasInFlightRunForTask(status=%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestHasInFlightRunForTask_ScopedToTheNamedTask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedCancelRun(t, repo, "other-task", "claimed", nil, nil)

	got, err := repo.HasInFlightRunForTask(ctx, "quiet-task")
	if err != nil {
		t.Fatalf("HasInFlightRunForTask: %v", err)
	}
	if got {
		t.Error("got true, want false — the only in-flight run belongs to another task")
	}
}

func TestHasInFlightRunForTask_EmptyTaskIDIsNotInFlight(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// An empty task ID names no task, so it must never be considered in flight.
	// HasInFlightRunForTask returns false before the query runs, making this
	// independent of the dialect; this test pins that early return.
	seedCancelRun(t, repo, "", "queued", nil, nil)

	got, err := repo.HasInFlightRunForTask(ctx, "")
	if err != nil {
		t.Fatalf("HasInFlightRunForTask(\"\"): %v", err)
	}
	if got {
		t.Error("got true, want false — an empty task ID names no task")
	}
}

func TestHasInFlightRunForTask_AnyInFlightRunCounts(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// A task that has already run once and is now running again: the terminal
	// run must not mask the live one.
	finishedAt := time.Now().UTC().Add(-time.Hour)
	seedCancelRun(t, repo, "busy-task", "finished", nil, &finishedAt)
	seedCancelRun(t, repo, "busy-task", "queued", nil, nil)

	got, err := repo.HasInFlightRunForTask(ctx, "busy-task")
	if err != nil {
		t.Fatalf("HasInFlightRunForTask: %v", err)
	}
	if !got {
		t.Error("got false, want true — the task has a queued run alongside a finished one")
	}
}
