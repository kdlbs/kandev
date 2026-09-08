package agents

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// Pins the transitions the Office agent recovery control depends on, before
// any caller of validateStatusTransition exists for that control. A future
// change to allowedTransitions that silently drops paused/stopped -> idle
// should fail here rather than surface as a UI regression.
func TestValidateStatusTransitionRecoveryPaths(t *testing.T) {
	cases := []struct {
		name string
		from models.AgentStatus
		to   models.AgentStatus
	}{
		{"paused to idle", models.AgentStatusPaused, models.AgentStatusIdle},
		{"stopped to idle", models.AgentStatusStopped, models.AgentStatusIdle},
		{"idle to idle is a same-status no-op", models.AgentStatusIdle, models.AgentStatusIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateStatusTransition(tc.from, tc.to); err != nil {
				t.Fatalf("validateStatusTransition(%q, %q) = %v, want nil", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateStatusTransitionRefusesIllegalOrUnknown(t *testing.T) {
	cases := []struct {
		name string
		from models.AgentStatus
		to   models.AgentStatus
	}{
		{"idle to working is not in the transition table", models.AgentStatusIdle, models.AgentStatusWorking},
		{"unknown source status", models.AgentStatus("bogus"), models.AgentStatusIdle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStatusTransition(tc.from, tc.to)
			if !errors.Is(err, ErrAgentStatusTransition) {
				t.Fatalf("validateStatusTransition(%q, %q) = %v, want ErrAgentStatusTransition", tc.from, tc.to, err)
			}
		})
	}
}
