package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestIsRecoveryDuplicateEvent pins AC-EXECUTORS-SURVIVAL-004.4's decision
// rule directly on AgentExecution, independent of the Manager-level
// apply/dedup flow covered elsewhere: only an event whose ControlTurnID
// matches the identifier this execution recorded as applied is a duplicate.
func TestIsRecoveryDuplicateEvent(t *testing.T) {
	t.Run("no outcome applied yet", func(t *testing.T) {
		var e AgentExecution
		if e.isRecoveryDuplicateEvent(&agentctl.AgentEvent{ControlTurnID: 9}) {
			t.Fatal("expected no match before markRecoveryTurnOutcomeApplied was ever called")
		}
	})

	t.Run("nil event", func(t *testing.T) {
		var e AgentExecution
		e.markRecoveryTurnOutcomeApplied(9)
		if e.isRecoveryDuplicateEvent(nil) {
			t.Fatal("expected a nil event to never match")
		}
	})

	t.Run("event with no ControlTurnID stamp", func(t *testing.T) {
		var e AgentExecution
		e.markRecoveryTurnOutcomeApplied(9)
		if e.isRecoveryDuplicateEvent(&agentctl.AgentEvent{}) {
			t.Fatal("expected an unstamped event (ControlTurnID zero) to never match")
		}
	})

	t.Run("mismatched turn ID", func(t *testing.T) {
		var e AgentExecution
		e.markRecoveryTurnOutcomeApplied(9)
		if e.isRecoveryDuplicateEvent(&agentctl.AgentEvent{ControlTurnID: 10}) {
			t.Fatal("expected a different turn's event to never match")
		}
	})

	t.Run("matching turn ID", func(t *testing.T) {
		var e AgentExecution
		e.markRecoveryTurnOutcomeApplied(9)
		if !e.isRecoveryDuplicateEvent(&agentctl.AgentEvent{ControlTurnID: 9}) {
			t.Fatal("expected the same turn's redelivered event to match")
		}
	})

	t.Run("zero turnID is a no-op", func(t *testing.T) {
		var e AgentExecution
		e.markRecoveryTurnOutcomeApplied(0)
		if e.isRecoveryDuplicateEvent(&agentctl.AgentEvent{ControlTurnID: 0}) {
			t.Fatal("expected marking zero to never make an unstamped event match")
		}
	})
}
