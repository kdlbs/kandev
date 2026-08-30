package sqlite

// Behavioral coverage for GetTaskUsageTotals and GetSessionUsageTotals
// (docs/specs/task-cost-ledger/spec.md AC-12, AC-18, AC-19, AC-20): the
// read-side aggregation the two HTTP usage routes surface.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestScanUsageTotals_RejectsUnsupportedScopeColumn(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	_, err := repo.scanUsageTotals(context.Background(), "task_id; DROP TABLE tasks;--", "task-1")
	if err == nil || !strings.Contains(err.Error(), "unsupported scope column") {
		t.Fatalf("scanUsageTotals error = %v, want an unsupported-scope error", err)
	}
}

func mustCreateUsageEvent(t *testing.T, repo *Repository, event *models.TaskUsageEvent) {
	t.Helper()
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateTaskUsageEvent(%s): %v", event.UsageEventID, err)
	}
}

func TestGetTaskUsageTotals_NoRows_ReturnsZeroedTotals(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-empty")

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-empty")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", totals.EventCount)
	}
	if totals.TokensIn != 0 || totals.TokensTotal != 0 || totals.CostSubcents != 0 {
		t.Errorf("totals = %+v, want every sum zero", totals)
	}
	if !totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true for a scope with no rows")
	}
	if totals.FirstEventAt != nil || totals.LastEventAt != nil {
		t.Errorf("timestamps = (%v, %v), want (nil, nil)", totals.FirstEventAt, totals.LastEventAt)
	}
}

func TestGetTaskUsageTotals_UnknownTask_ReturnsZeroedTotals(t *testing.T) {
	repo := newUsageEventsTestRepo(t)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-does-not-exist")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", totals.EventCount)
	}
}

// TestGetTaskUsageTotals_SumsAcrossSessionsAndClearedSessionRows pins that a
// task-scoped total includes every row for the task regardless of session_id,
// including a row whose session_id is NULL (e.g. cleared by session
// deletion or an AC-33 ownership mismatch).
func TestGetTaskUsageTotals_SumsAcrossSessionsAndClearedSessionRows(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-multi")
	createUsageEventsTestSession(t, repo, "session-a", "task-multi")
	createUsageEventsTestSession(t, repo, "session-b", "task-multi")

	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-a", "task-multi", "session-a"))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-b", "task-multi", "session-b"))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-c", "task-multi", ""))

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-multi")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 3 {
		t.Fatalf("EventCount = %d, want 3", totals.EventCount)
	}
	if totals.TokensIn != 300 {
		t.Errorf("TokensIn = %d, want 300 (3 rows of 100 each)", totals.TokensIn)
	}
	if totals.CostSubcents != 126 {
		t.Errorf("CostSubcents = %d, want 126 (3 rows of 42 each)", totals.CostSubcents)
	}
}

// TestGetTaskUsageTotals_TokensTotalIsStoredSumNeverRecomputed pins AC-19: the
// response's tokens_total is the sum of each row's stored tokens_total
// column, not a value recomputed from the per-kind sums at read time.
func TestGetTaskUsageTotals_TokensTotalIsStoredSumNeverRecomputed(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-stored-total")

	event := newTestUsageEvent("evt-stored-total", "task-stored-total", "")
	event.TokensIn = 10
	event.TokensCachedRead = nil
	event.TokensCachedWrite = nil
	event.TokensOut = nil
	event.TokensThought = nil
	event.TokensTotal = 999999 // deliberately inconsistent with the per-kind sum (10)
	mustCreateUsageEvent(t, repo, event)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-stored-total")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.TokensTotal != 999999 {
		t.Errorf("TokensTotal = %d, want the stored value 999999 verbatim, not a recomputed 10", totals.TokensTotal)
	}
	if totals.TokensIn != 10 {
		t.Errorf("TokensIn = %d, want 10", totals.TokensIn)
	}
}

// TestGetTaskUsageTotals_NilTokenColumnsCoalesceToZero pins AC-12: a
// not-recorded (nil) token column contributes zero to the sum rather than
// nulling out the whole aggregate.
func TestGetTaskUsageTotals_NilTokenColumnsCoalesceToZero(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-nil-cols")

	first := newTestUsageEvent("evt-nil-cols-1", "task-nil-cols", "")
	first.TokensCachedRead = nil
	first.TokensCachedWrite = nil
	first.TokensThought = nil
	measuredZero := int64(0)
	first.TokensOut = &measuredZero
	mustCreateUsageEvent(t, repo, first)

	second := newTestUsageEvent("evt-nil-cols-2", "task-nil-cols", "")
	mustCreateUsageEvent(t, repo, second)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-nil-cols")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.TokensCachedRead != 20 {
		t.Errorf("TokensCachedRead = %d, want 20 (nil row contributes 0, not NULL)", totals.TokensCachedRead)
	}
	if totals.TokensOut != 30 {
		t.Errorf("TokensOut = %d, want 30 (measured-zero row contributes 0, non-nil row contributes 30)", totals.TokensOut)
	}
}

// TestGetTaskUsageTotals_OutputTokensComplete_FalseWhenAnyRowMissingTokensOut
// pins AC-12/AC-19: a single contributing row with an unrecorded output
// count marks the whole total a lower bound.
func TestGetTaskUsageTotals_OutputTokensComplete_FalseWhenAnyRowMissingTokensOut(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-incomplete-output")

	complete := newTestUsageEvent("evt-complete", "task-incomplete-output", "")
	mustCreateUsageEvent(t, repo, complete)

	incomplete := newTestUsageEvent("evt-incomplete", "task-incomplete-output", "")
	incomplete.TokensOut = nil
	mustCreateUsageEvent(t, repo, incomplete)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-incomplete-output")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = true, want false (one row has an unrecorded output count)")
	}
}

func TestGetTaskUsageTotals_OutputTokensComplete_TrueWhenEveryRowHasTokensOut(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-complete-output")

	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-1", "task-complete-output", ""))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-2", "task-complete-output", ""))

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-complete-output")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if !totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true (every row has a recorded output count)")
	}
}

// TestGetTaskUsageTotals_EstimatedAndUnpricedCounts pins AC-19's confidence
// flags: estimated_event_count and unpriced_event_count let a caller
// distinguish a confident total from a partial one.
func TestGetTaskUsageTotals_EstimatedAndUnpricedCounts(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-flags")

	priced := newTestUsageEvent("evt-priced", "task-flags", "")
	priced.CostSource = "models_dev_list"
	mustCreateUsageEvent(t, repo, priced)

	unpriced := newTestUsageEvent("evt-unpriced", "task-flags", "")
	unpriced.CostSource = "unpriced"
	mustCreateUsageEvent(t, repo, unpriced)

	estimated := newTestUsageEvent("evt-estimated", "task-flags", "")
	estimated.Estimated = true
	mustCreateUsageEvent(t, repo, estimated)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-flags")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 3 {
		t.Fatalf("EventCount = %d, want 3", totals.EventCount)
	}
	if totals.UnpricedEventCount != 1 {
		t.Errorf("UnpricedEventCount = %d, want 1", totals.UnpricedEventCount)
	}
	if totals.EstimatedEventCount != 1 {
		t.Errorf("EstimatedEventCount = %d, want 1", totals.EstimatedEventCount)
	}
}

func TestGetTaskUsageTotals_FirstAndLastEventAt_MinAndMaxOccurredAt(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-timestamps")

	base := time.Date(2026, 8, 23, 4, 0, 0, 0, time.UTC)
	earliest := newTestUsageEvent("evt-earliest", "task-timestamps", "")
	earliest.OccurredAt = base
	earliest.CreatedAt = base
	mustCreateUsageEvent(t, repo, earliest)

	latest := newTestUsageEvent("evt-latest", "task-timestamps", "")
	latest.OccurredAt = base.Add(10 * time.Minute)
	latest.CreatedAt = latest.OccurredAt
	mustCreateUsageEvent(t, repo, latest)

	totals, err := repo.GetTaskUsageTotals(context.Background(), "task-timestamps")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.FirstEventAt == nil || !totals.FirstEventAt.Equal(base) {
		t.Errorf("FirstEventAt = %v, want %v", totals.FirstEventAt, base)
	}
	if totals.LastEventAt == nil || !totals.LastEventAt.Equal(latest.OccurredAt) {
		t.Errorf("LastEventAt = %v, want %v", totals.LastEventAt, latest.OccurredAt)
	}
}

// TestGetSessionUsageTotals_OnlyIncludesRowsForThatSession pins the
// session-scoped read's isolation: one task with two sessions, each
// session-scoped read returns only its own rows and their sum, never the
// other session's or the task's session-less rows.
func TestGetSessionUsageTotals_OnlyIncludesRowsForThatSession(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-two-sessions")
	createUsageEventsTestSession(t, repo, "session-x", "task-two-sessions")
	createUsageEventsTestSession(t, repo, "session-y", "task-two-sessions")

	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-x1", "task-two-sessions", "session-x"))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-x2", "task-two-sessions", "session-x"))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-y1", "task-two-sessions", "session-y"))
	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-task-only", "task-two-sessions", ""))

	totalsX, err := repo.GetSessionUsageTotals(context.Background(), "session-x")
	if err != nil {
		t.Fatalf("GetSessionUsageTotals(session-x): %v", err)
	}
	if totalsX.EventCount != 2 {
		t.Errorf("session-x EventCount = %d, want 2", totalsX.EventCount)
	}
	if totalsX.TokensIn != 200 {
		t.Errorf("session-x TokensIn = %d, want 200", totalsX.TokensIn)
	}

	totalsY, err := repo.GetSessionUsageTotals(context.Background(), "session-y")
	if err != nil {
		t.Fatalf("GetSessionUsageTotals(session-y): %v", err)
	}
	if totalsY.EventCount != 1 {
		t.Errorf("session-y EventCount = %d, want 1", totalsY.EventCount)
	}
}

// TestGetSessionUsageTotals_ZeroUsageSessionInsideTaskWithUsage_ReturnsZeroed
// pins the explicitly-flagged Round-5 scenario: a session with zero rows,
// inside a task that does have usage, still reports zeroed totals rather
// than picking up the task's other rows.
func TestGetSessionUsageTotals_ZeroUsageSessionInsideTaskWithUsage_ReturnsZeroed(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-has-usage")
	createUsageEventsTestSession(t, repo, "session-busy", "task-has-usage")
	createUsageEventsTestSession(t, repo, "session-idle", "task-has-usage")

	mustCreateUsageEvent(t, repo, newTestUsageEvent("evt-busy", "task-has-usage", "session-busy"))

	totals, err := repo.GetSessionUsageTotals(context.Background(), "session-idle")
	if err != nil {
		t.Fatalf("GetSessionUsageTotals(session-idle): %v", err)
	}
	if totals.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", totals.EventCount)
	}
	if totals.TokensIn != 0 || totals.TokensTotal != 0 {
		t.Errorf("totals = %+v, want every sum zero", totals)
	}
	if !totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true for a scope with no rows")
	}
}
