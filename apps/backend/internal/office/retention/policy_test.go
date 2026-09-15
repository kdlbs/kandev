package retention

import (
	"sort"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestClassifyRoutineRunStatus(t *testing.T) {
	for _, s := range RoutineRunHistoryStatuses {
		if got := ClassifyRoutineRunStatus(s); got != StatusHistory {
			t.Errorf("ClassifyRoutineRunStatus(%q) = %v, want StatusHistory", s, got)
		}
	}
	for _, s := range RoutineRunLiveStatuses {
		if got := ClassifyRoutineRunStatus(s); got != StatusLiveState {
			t.Errorf("ClassifyRoutineRunStatus(%q) = %v, want StatusLiveState", s, got)
		}
	}
	if got := ClassifyRoutineRunStatus("some_future_status"); got != StatusUnknown {
		t.Errorf("ClassifyRoutineRunStatus(unrecognized) = %v, want StatusUnknown", got)
	}
}

func TestClassifyRunStatus(t *testing.T) {
	for _, s := range RunHistoryStatuses {
		if got := ClassifyRunStatus(s); got != StatusHistory {
			t.Errorf("ClassifyRunStatus(%q) = %v, want StatusHistory", s, got)
		}
	}
	for _, s := range RunLiveStatuses {
		if got := ClassifyRunStatus(s); got != StatusLiveState {
			t.Errorf("ClassifyRunStatus(%q) = %v, want StatusLiveState", s, got)
		}
	}
	if got := ClassifyRunStatus("some_future_status"); got != StatusUnknown {
		t.Errorf("ClassifyRunStatus(unrecognized) = %v, want StatusUnknown", got)
	}
}

// TestRoutineRunStatusSets_CoverEveryEnumValue pins the closed-set claim in
// the system design: history + live-state must equal exactly the
// office/models RoutineRunStatus enumeration. A future eighth status added
// to enums.go without updating this file fails here rather than silently
// falling through ClassifyRoutineRunStatus's StatusUnknown branch on every
// install that never runs this test — the exact gap that hid `cancelled`.
func TestRoutineRunStatusSets_CoverEveryEnumValue(t *testing.T) {
	wantAll := []string{
		models.RoutineRunStatusReceived.String(),
		models.RoutineRunStatusTaskCreated.String(),
		models.RoutineRunStatusSkipped.String(),
		models.RoutineRunStatusCoalesced.String(),
		models.RoutineRunStatusFailed.String(),
		models.RoutineRunStatusDone.String(),
		models.RoutineRunStatusCancelled.String(),
	}
	gotAll := append(append([]string{}, RoutineRunHistoryStatuses...), RoutineRunLiveStatuses...)
	assertSameSet(t, "office_routine_runs", wantAll, gotAll)
}

// TestRunStatusSets_CoverEveryLiteralSQLWriter pins the closed-set claim for
// runs.status. models.RunStatus itself is incomplete (it does not list
// "cancelled", even though CancelRunsWhere in
// internal/runs/repository/sqlite/cancel.go writes it) — this test asserts
// against the actual literal statuses written by SQL in that package
// instead, per the system design's Testing section, so the enum's own
// incompleteness cannot mask a gap here the way it masked `cancelled`
// before this capability existed.
func TestRunStatusSets_CoverEveryLiteralSQLWriter(t *testing.T) {
	// Literal statuses written by internal/runs/repository/sqlite:
	//   cancel.go:   'cancelled'                              (CancelRunsWhere)
	//   runs.go:172: 'claimed'                                (ClaimNextRun family)
	//   runs.go:512: 'claimed'                                (claim by reason)
	//   runs.go:539: 'queued'                                 (ScheduleRetry)
	//   runs.go:562: 'queued'                                 (recoverStaleClaimed)
	//   runs.go:195/737: status passed as a bound parameter, produced by
	//     callers with 'finished' or 'failed' (FinishRun/MarkRunFailed).
	wantAll := []string{"queued", "claimed", "finished", "failed", "cancelled"}
	gotAll := append(append([]string{}, RunHistoryStatuses...), RunLiveStatuses...)
	assertSameSet(t, "runs", wantAll, gotAll)
}

func assertSameSet(t *testing.T, table string, want, got []string) {
	t.Helper()
	w := append([]string{}, want...)
	g := append([]string{}, got...)
	sort.Strings(w)
	sort.Strings(g)
	if len(w) != len(g) {
		t.Fatalf("%s: status set size = %d, want %d (got=%v want=%v)", table, len(g), len(w), g, w)
	}
	for i := range w {
		if w[i] != g[i] {
			t.Fatalf("%s: status set mismatch at %d: got %v, want %v", table, i, g, w)
		}
	}
}
