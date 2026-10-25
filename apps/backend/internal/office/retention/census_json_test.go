package retention

import (
	"encoding/json"
	"testing"
)

func TestCensusState_MarshalJSON_UsesStableNames(t *testing.T) {
	cases := map[CensusState]string{
		CensusNotComputed: `"not_computed"`,
		CensusFresh:       `"fresh"`,
		CensusStale:       `"stale"`,
	}
	for state, want := range cases {
		got, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", state, err)
		}
		if string(got) != want {
			t.Fatalf("Marshal(%v) = %s, want %s", state, got, want)
		}
	}
}

func TestCensusState_UnmarshalJSON_RoundTrips(t *testing.T) {
	for _, state := range []CensusState{CensusNotComputed, CensusFresh, CensusStale} {
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", state, err)
		}
		var decoded CensusState
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("Unmarshal(%s): %v", encoded, err)
		}
		if decoded != state {
			t.Fatalf("round trip %v -> %s -> %v", state, encoded, decoded)
		}
	}
}

func TestCensusState_UnmarshalJSON_RejectsUnknownName(t *testing.T) {
	var state CensusState
	if err := json.Unmarshal([]byte(`"bogus"`), &state); err == nil {
		t.Fatal("expected an error for an unrecognized census state name")
	}
}

func TestRetainedCounts_JSONUsesSnakeCaseFieldNames(t *testing.T) {
	counts := RetainedCounts{
		OfficeRoutineRuns: TableCensus{State: CensusFresh, RetainedCount: 3},
	}
	encoded, err := json.Marshal(counts)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal into map: %v", err)
	}
	for _, key := range []string{"office_routine_runs", "runs", "run_events"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("RetainedCounts JSON missing key %q; got %s", key, encoded)
		}
	}
}

func TestLastSweep_JSONUsesSnakeCaseFieldNames(t *testing.T) {
	sweep := LastSweep{
		OfficeRoutineRuns: SweptTableResult{
			TableSweepResult: TableSweepResult{Deleted: 5, Backlog: true},
			Previewed:        true,
			WouldDelete:      7,
		},
	}
	encoded, err := json.Marshal(sweep)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal into map: %v", err)
	}
	for _, key := range []string{
		"started_at", "finished_at", "office_routine_runs", "runs",
		"run_events", "route_attempts", "run_skills",
	} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("LastSweep JSON missing key %q; got %s", key, encoded)
		}
	}

	var routineRuns map[string]json.RawMessage
	if err := json.Unmarshal(raw["office_routine_runs"], &routineRuns); err != nil {
		t.Fatalf("Unmarshal office_routine_runs: %v", err)
	}
	for _, key := range []string{"deleted", "backlog", "error", "previewed", "would_delete"} {
		if _, ok := routineRuns[key]; !ok {
			t.Fatalf("SweptTableResult JSON missing key %q; got %s", key, raw["office_routine_runs"])
		}
	}
}
