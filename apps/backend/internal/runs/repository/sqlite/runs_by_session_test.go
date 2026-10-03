package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestGetRunBySessionAt_MatchesTheRunClaimedAtOrBeforeTheGivenTime covers
// AC-OFFICE-RUN-CAUSATION-001.25's step-entry carrier resolution: the ledger
// row that recorded a step-entry wake carries only a session id and an
// occurred_at timestamp, never a run id, so the causing run must be found by
// (session_id, claimed_at <= occurred_at) rather than by any status filter.
func TestGetRunBySessionAt_MatchesTheRunClaimedAtOrBeforeTheGivenTime(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-1", AgentProfileID: "a1", Reason: "task_assigned", SessionID: "sess-1",
	})
	setStatus(t, repo, run.ID, "claimed", timePtr(base), nil)

	got, err := repo.GetRunBySessionAt(ctx, "sess-1", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("get run by session at: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("run = %q, want %q", got.ID, run.ID)
	}
}

// TestGetRunBySessionAt_RaceCoverage_MatchesAFinishedRunToo covers the race
// this method exists for: a step-entry dispatch resolving asynchronously
// after the runner run has already finished must still find it, not just
// while status='claimed' (unlike GetClaimedRunByTaskID/GetClaimedRunByTaskAndAgent).
func TestGetRunBySessionAt_RaceCoverage_MatchesAFinishedRunToo(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-finished", AgentProfileID: "a1", Reason: "task_assigned", SessionID: "sess-2",
	})
	setStatus(t, repo, run.ID, "claimed", timePtr(base), nil)
	setStatus(t, repo, run.ID, "finished", timePtr(base), timePtr(base.Add(time.Minute)))

	got, err := repo.GetRunBySessionAt(ctx, "sess-2", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("get run by session at: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("run = %q, want %q (finished run must still resolve)", got.ID, run.ID)
	}
}

// TestGetRunBySessionAt_ExcludesARunClaimedAfterTheGivenTime covers the
// upper bound: a run claimed after the ledger row's occurred_at cannot be
// the run that caused it.
func TestGetRunBySessionAt_ExcludesARunClaimedAfterTheGivenTime(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-later", AgentProfileID: "a1", Reason: "task_assigned", SessionID: "sess-3",
	})
	setStatus(t, repo, run.ID, "claimed", timePtr(base), nil)

	if _, err := repo.GetRunBySessionAt(ctx, "sess-3", base.Add(-time.Minute)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows (run claimed after the given time)", err)
	}
}

// TestGetRunBySessionAt_UnknownSessionReturnsNoRows pins the sentinel.
func TestGetRunBySessionAt_UnknownSessionReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.GetRunBySessionAt(context.Background(), "sess-unknown", time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestGetRunBySessionAt_PrefersTheMostRecentlyClaimedMatch covers ordering
// when a session id was reused across more than one run (retry/relaunch): the
// most recently claimed match at or before the given time wins.
func TestGetRunBySessionAt_PrefersTheMostRecentlyClaimedMatch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	older := mustCreateRun(t, repo, &models.Run{
		ID: "run-older", AgentProfileID: "a1", Reason: "task_assigned", SessionID: "sess-4",
	})
	setStatus(t, repo, older.ID, "claimed", timePtr(base), nil)
	newer := mustCreateRun(t, repo, &models.Run{
		ID: "run-newer", AgentProfileID: "a1", Reason: "task_assigned", SessionID: "sess-4",
	})
	setStatus(t, repo, newer.ID, "claimed", timePtr(base.Add(time.Minute)), nil)

	got, err := repo.GetRunBySessionAt(ctx, "sess-4", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("get run by session at: %v", err)
	}
	if got.ID != newer.ID {
		t.Errorf("run = %q, want %q (most recently claimed match)", got.ID, newer.ID)
	}
}
