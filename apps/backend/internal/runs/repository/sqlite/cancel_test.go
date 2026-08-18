package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestCancelRun_CancelsQueuedAndClaimedRuns pins every column the
// terminal cancel write owns, for both cancellable states.
func TestCancelRun_CancelsQueuedAndClaimedRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	before := now.Add(-time.Second)

	queued := queueRunAt(t, repo, "queued", "a1", now)
	claimed := queueRunAt(t, repo, "claimed", "a2", now)
	setStatus(t, repo, claimed.ID, "claimed", timePtr(now), nil)

	for _, id := range []string{queued.ID, claimed.ID} {
		if err := repo.CancelRun(ctx, id, "user stopped the task"); err != nil {
			t.Fatalf("cancel %q: %v", id, err)
		}
		got := mustGetRun(t, repo, id)
		checkString(t, id+" status", string(got.Status), "cancelled")
		checkStringPtr(t, id+" cancel_reason", got.CancelReason, strPtr("user stopped the task"))
		if got.FinishedAt == nil {
			t.Errorf("%s finished_at = nil, want a stamp", id)
		} else if !got.FinishedAt.After(before) {
			t.Errorf("%s finished_at = %s, want after %s", id, got.FinishedAt, before)
		}
	}
}

// TestCancelRun_TerminalRunIsLeftAlone is the guard that keeps a run
// which completed between the caller's SELECT and this UPDATE from
// having its real outcome overwritten.
func TestCancelRun_TerminalRunIsLeftAlone(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	finishedAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

	for _, status := range []string{"finished", "failed", "cancelled"} {
		run := queueRunAt(t, repo, "run-"+status, "a1", finishedAt.Add(-time.Hour))
		setStatus(t, repo, run.ID, status, nil, timePtr(finishedAt))

		if err := repo.CancelRun(ctx, run.ID, "too late"); err != nil {
			t.Fatalf("cancel %q: %v", run.ID, err)
		}
		got := mustGetRun(t, repo, run.ID)
		checkString(t, run.ID+" status", string(got.Status), status)
		if got.CancelReason != nil {
			t.Errorf("%s cancel_reason = %q, want nil", run.ID, *got.CancelReason)
		}
		if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
			t.Errorf("%s finished_at = %v, want the original %s", run.ID, got.FinishedAt, finishedAt)
		}
	}
}

// TestCancelRun_UnknownRunIsANoOp pins that a missing id is not an error.
func TestCancelRun_UnknownRunIsANoOp(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.CancelRun(context.Background(), "nope", "reason"); err != nil {
		t.Fatalf("cancel unknown run: %v", err)
	}
}

// TestCancelRunsWhere_ReportsRowsActuallyCancelled pins the returned
// count against a mixed set: only the queued/claimed rows count.
func TestCancelRunsWhere_ReportsRowsActuallyCancelled(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queueRunAt(t, repo, "q1", "a1", now)
	claimed := queueRunAt(t, repo, "c1", "a1", now)
	setStatus(t, repo, claimed.ID, "claimed", timePtr(now), nil)
	finished := queueRunAt(t, repo, "f1", "a1", now)
	setStatus(t, repo, finished.ID, "finished", nil, timePtr(now))
	queueRunAt(t, repo, "other-agent", "a2", now)

	cancelled, err := repo.CancelRunsWhere(ctx, "agent removed", `agent_profile_id = ?`, "a1")
	if err != nil {
		t.Fatalf("cancel where: %v", err)
	}
	if cancelled != 2 {
		t.Errorf("cancelled = %d, want 2", cancelled)
	}
	checkString(t, "q1", string(mustGetRun(t, repo, "q1").Status), "cancelled")
	checkString(t, "c1", string(mustGetRun(t, repo, "c1").Status), "cancelled")
	checkString(t, "f1", string(mustGetRun(t, repo, "f1").Status), "finished")
	checkString(t, "other-agent", string(mustGetRun(t, repo, "other-agent").Status), "queued")

	// Re-running the same selector cancels nothing: everything eligible
	// has already reached a terminal state.
	cancelled, err = repo.CancelRunsWhere(ctx, "agent removed", `agent_profile_id = ?`, "a1")
	if err != nil {
		t.Fatalf("cancel where (second pass): %v", err)
	}
	if cancelled != 0 {
		t.Errorf("second pass cancelled = %d, want 0", cancelled)
	}
}

// TestCancelRunsWhere_MultiArgSelectorBindsInOrder covers a selector
// with several placeholders, the shape office's by-task selector uses.
func TestCancelRunsWhere_MultiArgSelectorBindsInOrder(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	mustCreateRun(t, repo, &models.Run{
		ID: "match", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t1"}`, Status: "queued",
	})
	mustCreateRun(t, repo, &models.Run{
		ID: "wrong-reason", AgentProfileID: "a1", Reason: "heartbeat",
		Payload: `{"task_id":"t1"}`, Status: "queued",
	})

	cancelled, err := repo.CancelRunsWhere(ctx, "superseded",
		`agent_profile_id = ? AND reason = ?`, "a1", "task_comment")
	if err != nil {
		t.Fatalf("cancel where: %v", err)
	}
	if cancelled != 1 {
		t.Errorf("cancelled = %d, want 1", cancelled)
	}
	checkString(t, "match", string(mustGetRun(t, repo, "match").Status), "cancelled")
	checkString(t, "wrong-reason", string(mustGetRun(t, repo, "wrong-reason").Status), "queued")
}

// TestBulkCancelRuns_CancelsOnlyTheListedEligibleRuns pins the id
// selector: rows in the set that are terminal, and rows outside it,
// are both left alone.
func TestBulkCancelRuns_CancelsOnlyTheListedEligibleRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queueRunAt(t, repo, "in-set-queued", "a1", now)
	claimed := queueRunAt(t, repo, "in-set-claimed", "a1", now)
	setStatus(t, repo, claimed.ID, "claimed", timePtr(now), nil)
	finished := queueRunAt(t, repo, "in-set-finished", "a1", now)
	setStatus(t, repo, finished.ID, "finished", nil, timePtr(now))
	queueRunAt(t, repo, "out-of-set", "a1", now)

	err := repo.BulkCancelRuns(ctx,
		[]string{"in-set-queued", "in-set-claimed", "in-set-finished", "missing-id"},
		"workspace deleted")
	if err != nil {
		t.Fatalf("bulk cancel: %v", err)
	}

	checkString(t, "in-set-queued", string(mustGetRun(t, repo, "in-set-queued").Status), "cancelled")
	checkString(t, "in-set-claimed", string(mustGetRun(t, repo, "in-set-claimed").Status), "cancelled")
	checkString(t, "in-set-finished", string(mustGetRun(t, repo, "in-set-finished").Status), "finished")
	checkString(t, "out-of-set", string(mustGetRun(t, repo, "out-of-set").Status), "queued")
	checkStringPtr(t, "in-set-queued reason",
		mustGetRun(t, repo, "in-set-queued").CancelReason, strPtr("workspace deleted"))
	if reason := mustGetRun(t, repo, "out-of-set").CancelReason; reason != nil {
		t.Errorf("out-of-set cancel_reason = %q, want nil", *reason)
	}
}

// TestBulkCancelRuns_EmptyIDListIsANoOp pins that the degenerate call
// does not fall through to an unbounded UPDATE.
func TestBulkCancelRuns_EmptyIDListIsANoOp(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	queueRunAt(t, repo, "survivor", "a1", time.Now().UTC())

	for _, ids := range [][]string{nil, {}} {
		if err := repo.BulkCancelRuns(ctx, ids, "nothing"); err != nil {
			t.Fatalf("bulk cancel %v: %v", ids, err)
		}
	}
	got := mustGetRun(t, repo, "survivor")
	checkString(t, "status", string(got.Status), "queued")
	if got.CancelReason != nil {
		t.Errorf("cancel_reason = %q, want nil", *got.CancelReason)
	}
}
