package models

import "testing"

// TestIsTaskLookupActiveSessionState verifies the active-state contract used by
// GetActiveTaskSessionByTaskID.
func TestIsTaskLookupActiveSessionState(t *testing.T) {
	want := map[TaskSessionState]bool{
		TaskSessionStateCreated:         true,
		TaskSessionStateStarting:        true,
		TaskSessionStateRunning:         true,
		TaskSessionStateIdle:            false,
		TaskSessionStateWaitingForInput: true,
		TaskSessionStateCompleted:       false,
		TaskSessionStateFailed:          false,
		TaskSessionStateCancelled:       false,
	}

	// AllTaskSessionStates is the canonical list this test ranges over. The checks
	// below assert it and want cover exactly the same set, with no duplicates — so
	// extending AllTaskSessionStates without extending want, or a duplicated/dropped
	// entry silently shrinking coverage, fails here. What this CANNOT catch: a new
	// TaskSessionState constant added to models.go but never added to
	// AllTaskSessionStates. Go does not enforce exhaustiveness and the exhaustive
	// linter is not enabled in this repo.
	seen := make(map[TaskSessionState]bool, len(AllTaskSessionStates))
	for _, state := range AllTaskSessionStates {
		if seen[state] {
			t.Fatalf("AllTaskSessionStates contains duplicate %q; coverage silently shrinks", state)
		}
		seen[state] = true
		if _, ok := want[state]; !ok {
			t.Fatalf("no expectation recorded for state %q; add one to this test's want map", state)
		}
	}
	for state := range want {
		if !seen[state] {
			t.Fatalf("want has %q but AllTaskSessionStates does not; the canonical list lost a state", state)
		}
	}

	for _, state := range AllTaskSessionStates {
		t.Run(string(state), func(t *testing.T) {
			wantActive := want[state]
			if got := IsTaskLookupActiveSessionState(state); got != wantActive {
				t.Errorf("IsTaskLookupActiveSessionState(%q) = %v, want %v", state, got, wantActive)
			}
		})
	}
}
