package models

import (
	"testing"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

func TestContinuationScopeForRun(t *testing.T) {
	tests := []struct {
		name           string
		run            *runsmodels.Run
		agentProfileID string
		want           string
	}{
		{
			name:           "routine scope",
			run:            &runsmodels.Run{ContextSnapshot: `{"routine_id":"routine-1"}`},
			agentProfileID: "agent-1",
			want:           "routine:routine-1",
		},
		{
			name:           "agent fallback",
			run:            &runsmodels.Run{ContextSnapshot: `{}`},
			agentProfileID: "agent-1",
			want:           "agent:agent-1",
		},
		{
			name:           "malformed snapshot fallback",
			run:            &runsmodels.Run{ContextSnapshot: "not-json"},
			agentProfileID: "agent-1",
			want:           "agent:agent-1",
		},
		{
			name:           "nil run fallback",
			agentProfileID: "agent-1",
			want:           "agent:agent-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContinuationScopeForRun(tt.run, tt.agentProfileID); got != tt.want {
				t.Fatalf("ContinuationScopeForRun() = %q, want %q", got, tt.want)
			}
		})
	}
}
