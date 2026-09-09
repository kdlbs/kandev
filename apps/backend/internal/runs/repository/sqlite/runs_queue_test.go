package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// countRuns is the row-count assertion that proves a coalesce merged
// instead of inserting.
func countRuns(t *testing.T, repo *runssqlite.Repository) int {
	t.Helper()
	var got int
	if err := repo.Reader().Get(&got, `SELECT COUNT(*) FROM runs`); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return got
}

// TestCoalesceRun_MergesIntoMostRecentQueuedRun asserts the identity of
// the row that absorbed the request: the newest queued run for the same
// agent + reason, with its payload replaced and its counter bumped, and
// no second row created.
func TestCoalesceRun_MergesIntoMostRecentQueuedRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	older := queueRunAt(t, repo, "older", "a1", now.Add(-30*time.Second))
	newer := queueRunAt(t, repo, "newer", "a1", now.Add(-10*time.Second))

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{"merged":true}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if !merged {
		t.Fatalf("coalesce = false, want true")
	}

	got := mustGetRun(t, repo, newer.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 2)
	checkString(t, "payload", got.Payload, `{"merged":true}`)

	untouched := mustGetRun(t, repo, older.ID)
	checkInt(t, "older coalesced_count", untouched.CoalescedCount, 1)
	checkString(t, "older payload", untouched.Payload, `{"task_id":"older"}`)

	if n := countRuns(t, repo); n != 2 {
		t.Errorf("%d rows after coalescing, want 2 (no new run may be created)", n)
	}
}

func TestCoalesceRun_DoesNotMergeDifferentTaskRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	queued := queueRunAt(t, repo, "task-a", "a1", time.Now().UTC())

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{"task_id":"task-b"}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if merged {
		t.Fatal("coalesce = true for a different task, want false")
	}

	got := mustGetRun(t, repo, queued.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 1)
	checkString(t, "payload", got.Payload, `{"task_id":"task-a"}`)
}

// TestCoalesceRun_RepeatedCoalesceAccumulatesOnOneRow pins the counter
// arithmetic across several merges into the same run.
func TestCoalesceRun_RepeatedCoalesceAccumulatesOnOneRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := queueRunAt(t, repo, "target", "a1", time.Now().UTC().Add(-time.Second))

	for i := range 3 {
		merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{"n":1}`)
		if err != nil {
			t.Fatalf("coalesce %d: %v", i, err)
		}
		if !merged {
			t.Fatalf("coalesce %d = false, want true", i)
		}
	}

	checkInt(t, "coalesced_count", mustGetRun(t, repo, run.ID).CoalescedCount, 4)
	if n := countRuns(t, repo); n != 1 {
		t.Errorf("%d rows, want 1", n)
	}
}

// TestCoalesceRun_NoEligibleRunLeavesEverythingAlone covers the three
// selector dimensions that disqualify a candidate.
func TestCoalesceRun_NoEligibleRunLeavesEverythingAlone(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sameAgent := queueRunAt(t, repo, "other-reason", "a1", now.Add(-time.Second))
	otherAgent := queueRunAt(t, repo, "other-agent", "a2", now.Add(-time.Second))
	claimed := queueRunAt(t, repo, "claimed", "a1", now.Add(-time.Second))
	setStatus(t, repo, claimed.ID, "claimed", timePtr(now), nil)

	merged, err := repo.CoalesceRun(ctx, "a1", "heartbeat", 3600, `{"merged":true}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if merged {
		t.Fatalf("coalesce = true, want false (no queued run with reason=heartbeat for a1)")
	}
	for _, id := range []string{sameAgent.ID, otherAgent.ID, claimed.ID} {
		got := mustGetRun(t, repo, id)
		checkInt(t, "coalesced_count of "+id, got.CoalescedCount, 1)
		checkString(t, "payload of "+id, got.Payload, `{"task_id":"`+id+`"}`)
	}
}

// TestCoalesceRun_HonoursTheWindow pins the requested_at cutoff: the
// same run is eligible under a wide window and ineligible under a
// narrow one.
func TestCoalesceRun_HonoursTheWindow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := queueRunAt(t, repo, "stale", "a1", time.Now().UTC().Add(-10*time.Second))

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 5, `{"merged":true}`)
	if err != nil {
		t.Fatalf("coalesce (narrow window): %v", err)
	}
	if merged {
		t.Fatalf("coalesce = true for a run older than the window, want false")
	}
	checkInt(t, "coalesced_count", mustGetRun(t, repo, run.ID).CoalescedCount, 1)

	merged, err = repo.CoalesceRun(ctx, "a1", "task_assigned", 600, `{"merged":true}`)
	if err != nil {
		t.Fatalf("coalesce (wide window): %v", err)
	}
	if !merged {
		t.Fatalf("coalesce = false inside the window, want true")
	}
	checkInt(t, "coalesced_count", mustGetRun(t, repo, run.ID).CoalescedCount, 2)
}

// TestCoalesceRun_SkipsCommentKeyedRuns pins the exclusion that keeps a
// comment-triggered run addressable by its own idempotency key: a
// salted or canonical task_comment run must never absorb an unrelated
// wake, while a key-less run in the same window still does.
func TestCoalesceRun_SkipsCommentKeyedRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	plain := mustCreateRun(t, repo, &models.Run{
		ID: "plain", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t1"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, plain.ID, now.Add(-20*time.Second))
	keyed := mustCreateRun(t, repo, &models.Run{
		ID: "keyed", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t1","comment_id":"cm-1"}`, Status: "queued",
		CoalescedCount: 1, IdempotencyKey: strPtr("task_comment:cm-1"),
	})
	setRequestedAt(t, repo, keyed.ID, now.Add(-5*time.Second))

	merged, err := repo.CoalesceRun(ctx, "a1", "task_comment", 600, `{"merged":true}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if !merged {
		t.Fatalf("coalesce = false, want true (the key-less run is eligible)")
	}
	checkInt(t, "comment-keyed coalesced_count", mustGetRun(t, repo, keyed.ID).CoalescedCount, 1)
	checkString(t, "comment-keyed payload", mustGetRun(t, repo, keyed.ID).Payload,
		`{"task_id":"t1","comment_id":"cm-1"}`)
	checkInt(t, "key-less coalesced_count", mustGetRun(t, repo, plain.ID).CoalescedCount, 2)
}

// TestCheckIdempotencyKey_WindowEdges pins both sides of the cutoff for
// the same key and row.
func TestCheckIdempotencyKey_WindowEdges(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "idem", AgentProfileID: "a1", Reason: "task_assigned",
		IdempotencyKey: strPtr("wake:t1"),
	})

	// Just inside a one-hour window.
	setRequestedAt(t, repo, run.ID, time.Now().UTC().Add(-time.Hour+time.Minute))
	seen, err := repo.CheckIdempotencyKey(ctx, "wake:t1", 1)
	if err != nil {
		t.Fatalf("check (inside window): %v", err)
	}
	if !seen {
		t.Errorf("seen = false for a run inside the window, want true")
	}

	// Just outside the same window; still inside a wider one.
	setRequestedAt(t, repo, run.ID, time.Now().UTC().Add(-time.Hour-time.Minute))
	seen, err = repo.CheckIdempotencyKey(ctx, "wake:t1", 1)
	if err != nil {
		t.Fatalf("check (outside window): %v", err)
	}
	if seen {
		t.Errorf("seen = true for a run older than the window, want false")
	}
	seen, err = repo.CheckIdempotencyKey(ctx, "wake:t1", 24)
	if err != nil {
		t.Fatalf("check (wide window): %v", err)
	}
	if !seen {
		t.Errorf("seen = false under a 24h window, want true")
	}
}

// TestCheckIdempotencyKey_MatchesOnTheExactKey guards against a
// weakened comparison (prefix/LIKE) letting a neighbouring key satisfy
// the check.
func TestCheckIdempotencyKey_MatchesOnTheExactKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	mustCreateRun(t, repo, &models.Run{
		ID: "idem", AgentProfileID: "a1", Reason: "task_assigned",
		IdempotencyKey: strPtr("wake:t1"),
	})
	mustCreateRun(t, repo, &models.Run{
		ID: "no-key", AgentProfileID: "a1", Reason: "task_assigned",
	})

	for _, key := range []string{"wake:t2", "wake:t1:salt", "wake:t", ""} {
		seen, err := repo.CheckIdempotencyKey(ctx, key, 24)
		if err != nil {
			t.Fatalf("check %q: %v", key, err)
		}
		if seen {
			t.Errorf("seen = true for key %q, want false", key)
		}
	}
}

// TestCreateRun_DuplicateIdempotencyKeyIsRejected pins the storage-level
// half of the idempotency contract: the same key twice cannot produce a
// second run, and the original row is the one that survives.
func TestCreateRun_DuplicateIdempotencyKeyIsRejected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	first := mustCreateRun(t, repo, &models.Run{
		ID: "first", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t1"}`, IdempotencyKey: strPtr("task_comment:cm-1"),
	})

	err := repo.CreateRun(ctx, &models.Run{
		ID: "second", AgentProfileID: "a1", Reason: "task_comment",
		Payload: `{"task_id":"t2"}`, IdempotencyKey: strPtr("task_comment:cm-1"),
	})
	if err == nil {
		t.Fatalf("second CreateRun with a duplicate idempotency key succeeded, want a constraint error")
	}
	if n := countRuns(t, repo); n != 1 {
		t.Errorf("%d rows after the duplicate insert, want 1", n)
	}
	checkString(t, "surviving run payload", mustGetRun(t, repo, first.ID).Payload, `{"task_id":"t1"}`)
}

// TestScheduleRetry_RequeuesAndClearsTerminalTimestamps asserts every
// column the retry write owns, from a run that had already finished.
func TestScheduleRetry_RequeuesAndClearsTerminalTimestamps(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	run := queueRunAt(t, repo, "retryable", "a1", now.Add(-time.Hour))
	setStatus(t, repo, run.ID, "failed", timePtr(now.Add(-30*time.Minute)), timePtr(now))
	untouched := queueRunAt(t, repo, "bystander", "a1", now.Add(-time.Hour))

	retryAt := time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC)
	if err := repo.ScheduleRetry(ctx, run.ID, retryAt, 4); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	got := mustGetRun(t, repo, run.ID)
	checkString(t, "status", string(got.Status), "queued")
	checkInt(t, "retry_count", got.RetryCount, 4)
	if got.ScheduledRetryAt == nil || !got.ScheduledRetryAt.Equal(retryAt) {
		t.Errorf("scheduled_retry_at = %v, want %s", got.ScheduledRetryAt, retryAt)
	}
	if got.ClaimedAt != nil {
		t.Errorf("claimed_at = %s, want nil", got.ClaimedAt)
	}
	if got.FinishedAt != nil {
		t.Errorf("finished_at = %s, want nil", got.FinishedAt)
	}
	checkString(t, "payload preserved", got.Payload, `{"task_id":"retryable"}`)

	bystander := mustGetRun(t, repo, untouched.ID)
	checkInt(t, "bystander retry_count", bystander.RetryCount, 0)
	if bystander.ScheduledRetryAt != nil {
		t.Errorf("bystander scheduled_retry_at = %s, want nil", bystander.ScheduledRetryAt)
	}
}

// TestScheduleRetry_WritesTheAttemptCountVerbatim pins that the caller
// owns the counter — the repository must not derive or clamp it.
func TestScheduleRetry_WritesTheAttemptCountVerbatim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := queueRunAt(t, repo, "counter", "a1", time.Now().UTC())

	for _, count := range []int{1, 5, 0} {
		if err := repo.ScheduleRetry(ctx, run.ID, fixedRetryAt, count); err != nil {
			t.Fatalf("schedule retry %d: %v", count, err)
		}
		checkInt(t, "retry_count", mustGetRun(t, repo, run.ID).RetryCount, count)
	}
}

// TestRecoverStale_ResetsOnlyClaimedRunsStrictlyOlderThanTheCutoff
// exercises the staleness boundary in both directions. The cutoff is a
// parameter, so the equality case is exact rather than clock-dependent.
func TestRecoverStale_ResetsOnlyClaimedRunsStrictlyOlderThanTheCutoff(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	stale := queueRunAt(t, repo, "stale", "a1", cutoff.Add(-time.Hour))
	setStatus(t, repo, stale.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	atCutoff := queueRunAt(t, repo, "at-cutoff", "a2", cutoff.Add(-time.Hour))
	setStatus(t, repo, atCutoff.ID, "claimed", timePtr(cutoff), nil)
	fresh := queueRunAt(t, repo, "fresh", "a3", cutoff.Add(-time.Hour))
	setStatus(t, repo, fresh.ID, "claimed", timePtr(cutoff.Add(time.Minute)), nil)
	finished := queueRunAt(t, repo, "finished", "a4", cutoff.Add(-time.Hour))
	setStatus(t, repo, finished.ID, "finished",
		timePtr(cutoff.Add(-time.Hour)), timePtr(cutoff.Add(-time.Minute)))

	recovered, err := repo.RecoverStale(ctx, cutoff)
	if err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	if recovered != 1 {
		t.Errorf("recovered = %d, want 1", recovered)
	}

	got := mustGetRun(t, repo, stale.ID)
	checkString(t, "stale status", string(got.Status), "queued")
	if got.ClaimedAt != nil {
		t.Errorf("stale claimed_at = %s, want nil", got.ClaimedAt)
	}
	checkString(t, "at-cutoff status", string(mustGetRun(t, repo, atCutoff.ID).Status), "claimed")
	checkString(t, "fresh status", string(mustGetRun(t, repo, fresh.ID).Status), "claimed")
	checkString(t, "finished status", string(mustGetRun(t, repo, finished.ID).Status), "finished")
	if got := mustGetRun(t, repo, finished.ID); got.ClaimedAt == nil {
		t.Errorf("finished run lost its claimed_at, want it preserved")
	}
}

// TestRecoverStale_NothingToRecoverReportsZero pins the count when the
// sweep is a no-op.
func TestRecoverStale_NothingToRecoverReportsZero(t *testing.T) {
	repo := newTestRepo(t)
	queueRunAt(t, repo, "queued", "a1", time.Now().UTC())

	recovered, err := repo.RecoverStale(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	if recovered != 0 {
		t.Errorf("recovered = %d, want 0", recovered)
	}
}

// TestCleanExpired_DeletesOnlyTerminalRunsBeforeTheCutoff covers the
// status filter, the strict finished_at comparison, and the returned
// row count. Cancelled runs are deliberately not swept.
func TestCleanExpired_DeletesOnlyTerminalRunsBeforeTheCutoff(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	old := cutoff.Add(-time.Hour)

	seedTerminal(t, repo, "old-finished", "finished", timePtr(old))
	seedTerminal(t, repo, "old-failed", "failed", timePtr(old))
	seedTerminal(t, repo, "at-cutoff", "finished", timePtr(cutoff))
	seedTerminal(t, repo, "new-finished", "finished", timePtr(cutoff.Add(time.Hour)))
	seedTerminal(t, repo, "old-cancelled", "cancelled", timePtr(old))
	seedTerminal(t, repo, "no-finished-at", "finished", nil)
	queueRunAt(t, repo, "queued", "a9", old)

	deleted, err := repo.CleanExpired(ctx, cutoff)
	if err != nil {
		t.Fatalf("clean expired: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	survivors := []string{"at-cutoff", "new-finished", "old-cancelled", "no-finished-at", "queued"}
	for _, id := range survivors {
		if _, err := repo.GetRunByID(ctx, id); err != nil {
			t.Errorf("run %q was deleted (%v), want it kept", id, err)
		}
	}
	if n := countRuns(t, repo); n != len(survivors) {
		t.Errorf("%d rows left, want %d", n, len(survivors))
	}
}

// seedTerminal creates a run already in a terminal state with the given
// finished_at, bypassing the state machine the way the sweeper's
// fixtures do.
func seedTerminal(
	t *testing.T, repo *runssqlite.Repository, id, status string, finishedAt *time.Time,
) {
	t.Helper()
	run := queueRunAt(t, repo, id, "a1", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	setStatus(t, repo, run.ID, status, nil, finishedAt)
}
