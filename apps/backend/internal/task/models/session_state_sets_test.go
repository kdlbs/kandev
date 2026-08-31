package models

import "testing"

func TestIsActiveSessionState(t *testing.T) {
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

	// AllTaskSessionStates is the canonical, exhaustive state list; this length
	// check trips if a new TaskSessionState constant is added to models.go
	// without extending AllTaskSessionStates alongside it.
	if len(AllTaskSessionStates) != 8 {
		t.Fatalf("len(AllTaskSessionStates) = %d, want 8 (a TaskSessionState constant was added or "+
			"removed without updating this test's expectations table)", len(AllTaskSessionStates))
	}

	for _, state := range AllTaskSessionStates {
		t.Run(string(state), func(t *testing.T) {
			wantActive, ok := want[state]
			if !ok {
				t.Fatalf("no expectation recorded for state %q; add one to this test's want map", state)
			}
			if got := IsActiveSessionState(state); got != wantActive {
				t.Errorf("IsActiveSessionState(%q) = %v, want %v", state, got, wantActive)
			}
		})
	}
}
