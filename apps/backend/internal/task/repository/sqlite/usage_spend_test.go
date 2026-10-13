package sqlite

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func spendEvent(t *testing.T, repo *Repository, id, taskID, sessionID, turnID string, at time.Time, cost int64, source string) {
	t.Helper()
	e := newTestUsageEvent(id, taskID, sessionID)
	e.OccurredAt = at
	e.CostSubcents = cost
	e.CostSource = source
	e.TurnID = turnID
	mustCreateUsageEvent(t, repo, e)
}

func TestSumUsageForTasks_WindowIsHalfOpenAndScopedToIDs(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "t1")
	createUsageEventsTestTask(t, repo, "t2")
	createUsageEventsTestTask(t, repo, "other")
	from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	spendEvent(t, repo, "e-before", "t1", "", "", from.Add(-time.Nanosecond), 1000, "actual")
	spendEvent(t, repo, "e-at-from", "t1", "", "", from, 1, "actual")
	spendEvent(t, repo, "e-mid", "t2", "", "", from.Add(time.Hour), 20, "actual")
	spendEvent(t, repo, "e-at-to", "t1", "", "", to, 1000, "actual")
	spendEvent(t, repo, "e-other", "other", "", "", from.Add(time.Hour), 1000, "actual")

	got, err := repo.SumUsageForTasks(t.Context(), []string{"t1", "t2"}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got.CostSubcents != 21 || got.HasUnpriced {
		t.Fatalf("got %+v, want {21 false}", got)
	}
}

func TestSumUsageForTasks_UnpricedRowFlagsAndIsNotSummed(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "t1")
	from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	spendEvent(t, repo, "a", "t1", "", "", from.Add(time.Minute), 5, "actual")
	spendEvent(t, repo, "b", "t1", "", "", from.Add(time.Minute), 900, "unpriced")

	got, err := repo.SumUsageForTasks(t.Context(), []string{"t1"}, from, from.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.CostSubcents != 5 || !got.HasUnpriced {
		t.Fatalf("got %+v, want {5 true}", got)
	}
}

func TestSumUsageForTasks_NoIDsOrNoRowsIsZero(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "t1")
	now := time.Now().UTC()
	for _, ids := range [][]string{nil, {"t1"}} {
		got, err := repo.SumUsageForTasks(t.Context(), ids, now.Add(-time.Hour), now)
		if err != nil || got.CostSubcents != 0 || got.HasUnpriced {
			t.Fatalf("ids=%v got %+v err %v", ids, got, err)
		}
	}
}

func TestSumUsageForTasks_RejectsMoreThanMaxIDs(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ids := make([]string, MaxSpendTaskIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("t%d", i)
	}
	now := time.Now().UTC()
	if _, err := repo.SumUsageForTasks(t.Context(), ids, now.Add(-time.Hour), now); err == nil {
		t.Fatal("expected error for oversized id list")
	}
}

func TestSumUsageForTasks_SaturatesOrFailsClosedOnOverflow(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "t1")
	from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	spendEvent(t, repo, "a", "t1", "", "", from.Add(time.Minute), math.MaxInt64, "actual")
	spendEvent(t, repo, "b", "t1", "", "", from.Add(time.Minute), 10, "actual")

	got, err := repo.SumUsageForTasks(t.Context(), []string{"t1"}, from, from.Add(time.Hour))
	if err == nil && got.CostSubcents != math.MaxInt64 {
		t.Fatalf("overflow must saturate or error, got %+v", got)
	}
}

func TestSumUsageForTurn_MatchesTurnAndSessionUpToInclusiveBound(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "t1")
	createUsageEventsTestSession(t, repo, "s1", "t1")
	createUsageEventsTestSession(t, repo, "s2", "t1")
	finished := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	bound := finished.Add(10 * time.Minute)
	spendEvent(t, repo, "in", "t1", "s1", "turn-1", finished.Add(-time.Minute), 3, "actual")
	spendEvent(t, repo, "at-bound", "t1", "s1", "turn-1", bound, 4, "actual")
	spendEvent(t, repo, "past", "t1", "s1", "turn-1", bound.Add(time.Nanosecond), 100, "actual")
	spendEvent(t, repo, "other-turn", "t1", "s1", "turn-2", finished, 100, "actual")
	spendEvent(t, repo, "other-session", "t1", "s2", "turn-1", finished, 100, "actual")
	spendEvent(t, repo, "unpriced", "t1", "s1", "turn-1", finished, 100, "unpriced")

	got, err := repo.SumUsageForTurn(t.Context(), "s1", "turn-1", bound)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestSumUsageForTurn_NoRowsIsZero(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	got, err := repo.SumUsageForTurn(t.Context(), "s", "t", time.Now().UTC())
	if err != nil || got != 0 {
		t.Fatalf("got %d err %v", got, err)
	}
}
