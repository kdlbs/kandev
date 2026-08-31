package models

import "testing"

func TestIsActiveSessionState(t *testing.T) {
	tests := []struct {
		state TaskSessionState
		want  bool
	}{
		{TaskSessionStateCreated, true},
		{TaskSessionStateStarting, true},
		{TaskSessionStateRunning, true},
		{TaskSessionStateIdle, false},
		{TaskSessionStateWaitingForInput, true},
		{TaskSessionStateCompleted, false},
		{TaskSessionStateFailed, false},
		{TaskSessionStateCancelled, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := IsActiveSessionState(tc.state); got != tc.want {
				t.Errorf("IsActiveSessionState(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}
