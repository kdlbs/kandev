package models

import "testing"

func TestIsResumableSessionState(t *testing.T) {
	resumable := []TaskSessionState{
		TaskSessionStateStarting, TaskSessionStateRunning,
		TaskSessionStateWaitingForInput, TaskSessionStateIdle,
	}
	for _, s := range resumable {
		if !IsResumableSessionState(s) {
			t.Errorf("IsResumableSessionState(%q) = false, want true", s)
		}
	}
	notResumable := []TaskSessionState{
		TaskSessionStateCreated, TaskSessionStateCompleted,
		TaskSessionStateFailed, TaskSessionStateCancelled,
	}
	for _, s := range notResumable {
		if IsResumableSessionState(s) {
			t.Errorf("IsResumableSessionState(%q) = true, want false", s)
		}
	}
}

func TestRowMustBePreserved(t *testing.T) {
	tests := []struct {
		name    string
		running *ExecutorRunning
		state   TaskSessionState
		want    bool
	}{
		{"nil row is never preserved", nil, TaskSessionStateRunning, false},
		{"resume_token on terminal session is preserved",
			&ExecutorRunning{ResumeToken: "tok"}, TaskSessionStateCompleted, true},
		{"running session without token is preserved",
			&ExecutorRunning{}, TaskSessionStateRunning, true},
		{"waiting-for-input session without token is preserved",
			&ExecutorRunning{}, TaskSessionStateWaitingForInput, true},
		{"idle office session without token is preserved",
			&ExecutorRunning{}, TaskSessionStateIdle, true},
		{"never-started created session without token is prunable",
			&ExecutorRunning{}, TaskSessionStateCreated, false},
		{"terminal session without token is prunable",
			&ExecutorRunning{}, TaskSessionStateCancelled, false},
		{"terminal session with token is still preserved",
			&ExecutorRunning{ResumeToken: "tok"}, TaskSessionStateFailed, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RowMustBePreserved(tc.running, tc.state); got != tc.want {
				t.Errorf("RowMustBePreserved = %v, want %v", got, tc.want)
			}
		})
	}
}
