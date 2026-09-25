package models

import "testing"

// TestIsTaskLookupActiveSessionState pins each state's expected boolean against
// an independent, hand-written table rather than the predicate under test, so a
// semantic regression (for example flipping Completed to active) fails here
// instead of passing silently through callers that derive their own expectation
// from the same predicate.
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

	for state, wantActive := range want {
		if got := IsTaskLookupActiveSessionState(state); got != wantActive {
			t.Errorf("IsTaskLookupActiveSessionState(%q) = %v, want %v", state, got, wantActive)
		}
	}
}

// TestAdmittedSessionStatesCoverEveryState fails when a member of
// AllTaskSessionStates has no explicit entry in admittedSessionStates. Go does not
// enforce switch/slice exhaustiveness, so without this a state added later is
// silently classified by the map's zero value instead of by a decision.
func TestAdmittedSessionStatesCoverEveryState(t *testing.T) {
	for _, state := range AllTaskSessionStates {
		if _, classified := admittedSessionStates[state]; !classified {
			t.Errorf("state %q has no entry in admittedSessionStates: classify it as counted or not counted", state)
		}
	}
	if len(admittedSessionStates) != len(AllTaskSessionStates) {
		t.Errorf("admittedSessionStates has %d entries, AllTaskSessionStates has %d: the map classifies a state that is not in the canonical list",
			len(admittedSessionStates), len(AllTaskSessionStates))
	}
}

// TestIsAdmittedSessionStateCountsOnlyStartingAndRunning pins the counted set to
// the two states that hold an agent process.
func TestIsAdmittedSessionStateCountsOnlyStartingAndRunning(t *testing.T) {
	for _, state := range AllTaskSessionStates {
		want := state == TaskSessionStateStarting || state == TaskSessionStateRunning
		if got := IsAdmittedSessionState(state); got != want {
			t.Errorf("IsAdmittedSessionState(%q) = %v, want %v", state, got, want)
		}
	}
}

// TestIsAdmittedSessionStateIsDistinctFromLookupActive guards the two predicates
// against being collapsed into one: Created and WaitingForInput are active for the
// task-lookup set and not counted here.
func TestIsAdmittedSessionStateIsDistinctFromLookupActive(t *testing.T) {
	for _, state := range []TaskSessionState{TaskSessionStateCreated, TaskSessionStateWaitingForInput} {
		if !IsTaskLookupActiveSessionState(state) {
			t.Fatalf("precondition: IsTaskLookupActiveSessionState(%q) = false, want true", state)
		}
		if IsAdmittedSessionState(state) {
			t.Errorf("IsAdmittedSessionState(%q) = true, want false: it holds no agent process", state)
		}
	}
}
