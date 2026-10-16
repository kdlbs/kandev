package wire

import (
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/ledger"
	"github.com/kandev/kandev/internal/coordinator/outcomes"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

// TestRedeclaredConstantsMatchTheirOriginals pins the strings replay
// redeclares so it need not import the packages that own them.
func TestRedeclaredConstantsMatchTheirOriginals(t *testing.T) {
	for _, c := range []struct{ name, got, want string }{
		{"approved", replay.DecisionApproved, outcomes.DecisionApproved},
		{"edited", replay.DecisionEdited, outcomes.DecisionEdited},
		{"rejected", replay.DecisionRejected, outcomes.DecisionRejected},
		{"returned", replay.DecisionReturned, outcomes.DecisionReturned},
		{"undone", replay.DecisionUndone, outcomes.DecisionUndone},
		{"message trigger", replay.TriggerMessage, ledger.TriggerMessage},
		{"wake trigger", replay.TriggerWake, ledger.TriggerWake},
		{"dream trigger", replay.TriggerDream, ledger.TriggerDream},
		{"create_task", replay.ProposalCreateTask, coordinator.ProposalKindCreateTask},
		{"message", replay.ProposalMessage, coordinator.ProposalKindMessage},
		{"move", replay.ProposalMove, coordinator.ProposalKindMove},
		{"resume", replay.ProposalResume, coordinator.ProposalKindResume},
		{"improvement", replay.ProposalImprovement, coordinator.ProposalKindImprovement},
	} {
		if c.got != c.want {
			t.Errorf("%s: replay has %q, original is %q", c.name, c.got, c.want)
		}
	}
}
