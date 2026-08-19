package models

import "testing"

func TestCompletionIntentStateCanTransitionTo(t *testing.T) {
	tests := []struct {
		name string
		from CompletionIntentState
		to   CompletionIntentState
		want bool
	}{
		{name: "pending settles", from: CompletionIntentStatePending, to: CompletionIntentStateSettling, want: true},
		{name: "settling settles", from: CompletionIntentStateSettling, to: CompletionIntentStateSettled, want: true},
		{name: "pending reopens", from: CompletionIntentStatePending, to: CompletionIntentStateReopened, want: true},
		{name: "settled cannot reopen", from: CompletionIntentStateSettled, to: CompletionIntentStateReopened, want: false},
		{name: "reopened cannot settle", from: CompletionIntentStateReopened, to: CompletionIntentStateSettling, want: false},
		{name: "superseded terminal", from: CompletionIntentStateSuperseded, to: CompletionIntentStateSettled, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Fatalf("CanTransitionTo(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
