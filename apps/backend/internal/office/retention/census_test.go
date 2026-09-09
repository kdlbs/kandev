package retention

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSummarizeStatusCensus_SumsAllStatusesRegardlessOfClass(t *testing.T) {
	counts := map[string]int64{
		"done":     3,
		"received": 2,
	}
	retained, unknown := summarizeStatusCensus(counts, RoutineRunHistoryStatuses, RoutineRunLiveStatuses)
	if retained != 5 {
		t.Fatalf("retained = %d, want 5", retained)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none", unknown)
	}
}

func TestSummarizeStatusCensus_DetectsUnknownStatusesSortedAscending(t *testing.T) {
	counts := map[string]int64{
		"done":    1,
		"zeta":    1,
		"alpha":   1,
		"skipped": 1,
	}
	retained, unknown := summarizeStatusCensus(counts, RoutineRunHistoryStatuses, RoutineRunLiveStatuses)
	if retained != 4 {
		t.Fatalf("retained = %d, want 4", retained)
	}
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(unknown, want) {
		t.Fatalf("unknown = %v, want %v", unknown, want)
	}
}

func TestSummarizeStatusCensus_EmptyTableIsZeroNotUnknown(t *testing.T) {
	retained, unknown := summarizeStatusCensus(map[string]int64{}, RunHistoryStatuses, RunLiveStatuses)
	if retained != 0 {
		t.Fatalf("retained = %d, want 0", retained)
	}
	if unknown != nil {
		t.Fatalf("unknown = %v, want nil", unknown)
	}
}

func TestCensusTracker_SnapshotStartsNotComputedForEveryTable(t *testing.T) {
	tracker := NewCensusTracker()
	snap := tracker.Snapshot()
	for name, c := range map[string]TableCensus{
		"office_routine_runs": snap.OfficeRoutineRuns,
		"runs":                snap.Runs,
		"run_events":          snap.RunEvents,
	} {
		if c.State != CensusNotComputed {
			t.Fatalf("%s: state = %v, want CensusNotComputed", name, c.State)
		}
	}
}

func TestCensusTracker_SuccessfulEvaluationIsFresh(t *testing.T) {
	tracker := NewCensusTracker()
	now := time.Now().UTC()
	tracker.RecordRuns(TableCensus{RetainedCount: 42, AsOf: now}, nil)

	got := tracker.Snapshot().Runs
	if got.State != CensusFresh {
		t.Fatalf("state = %v, want CensusFresh", got.State)
	}
	if got.RetainedCount != 42 {
		t.Fatalf("retained = %d, want 42", got.RetainedCount)
	}
	if !got.AsOf.Equal(now) {
		t.Fatalf("asOf = %v, want %v", got.AsOf, now)
	}
}

func TestCensusTracker_FailedEvaluationWithNoPriorSuccessStaysNotComputed(t *testing.T) {
	tracker := NewCensusTracker()
	tracker.RecordRoutineRuns(TableCensus{}, errors.New("boom"))

	got := tracker.Snapshot().OfficeRoutineRuns
	if got.State != CensusNotComputed {
		t.Fatalf("state = %v, want CensusNotComputed", got.State)
	}
}

func TestCensusTracker_FailedEvaluationAfterSuccessKeepsLastCountsMarkedStale(t *testing.T) {
	tracker := NewCensusTracker()
	first := TableCensus{
		RetainedCount:   100,
		AsOf:            time.Now().UTC(),
		UnknownStatuses: []string{"weird"},
		TopRoutineID:    "r-1",
		TopRoutineShare: 0.5,
	}
	tracker.RecordRoutineRuns(first, nil)

	tracker.RecordRoutineRuns(TableCensus{}, errors.New("query failed"))

	got := tracker.Snapshot().OfficeRoutineRuns
	if got.State != CensusStale {
		t.Fatalf("state = %v, want CensusStale", got.State)
	}
	if got.RetainedCount != 100 {
		t.Fatalf("retained = %d, want 100 (carried over)", got.RetainedCount)
	}
	if !reflect.DeepEqual(got.UnknownStatuses, []string{"weird"}) {
		t.Fatalf("unknownStatuses = %v, want carried over", got.UnknownStatuses)
	}
	if got.TopRoutineID != "r-1" || got.TopRoutineShare != 0.5 {
		t.Fatalf("top routine attribution not carried over: %+v", got)
	}
}

func TestCensusTracker_OneTableFailureDoesNotTouchAnother(t *testing.T) {
	tracker := NewCensusTracker()
	tracker.RecordRuns(TableCensus{RetainedCount: 7, AsOf: time.Now().UTC()}, nil)

	tracker.RecordRoutineRuns(TableCensus{}, errors.New("boom"))

	snap := tracker.Snapshot()
	if snap.Runs.State != CensusFresh || snap.Runs.RetainedCount != 7 {
		t.Fatalf("runs entry disturbed by routine_runs failure: %+v", snap.Runs)
	}
	if snap.OfficeRoutineRuns.State != CensusNotComputed {
		t.Fatalf("routine_runs state = %v, want CensusNotComputed", snap.OfficeRoutineRuns.State)
	}
}
