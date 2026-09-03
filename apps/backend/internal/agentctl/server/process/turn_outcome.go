package process

import "github.com/kandev/kandev/internal/agentctl/server/adapter"

// TurnOutcomeRecorder retains the last terminal outcome observed for an
// instance so a backend that re-attaches after a restart
// (AC-EXECUTORS-SURVIVAL-004) can learn what happened to a turn it never saw
// complete. Satisfied by instance.Manager.RetainTurnOutcome; defined here
// (rather than imported from the instance package) because process must not
// import instance, which already imports process.
type TurnOutcomeRecorder interface {
	RetainTurnOutcome(instanceID string, event adapter.AgentEvent) (turnID int64, ok bool)
}

// SetTurnOutcomeRecorder wires the instance-level outcome retention sink for
// this process manager. Called once by instance.Manager.CreateInstance right
// after constructing the process manager and before Start can be reached
// through any path, mirroring the StderrProviderSetter optional-interface
// pattern used elsewhere in this package. A Manager with no recorder set
// (most existing tests, and the e2e harness) simply never retains --
// retention is additive, not required for correct event delivery.
func (m *Manager) SetTurnOutcomeRecorder(instanceID string, recorder TurnOutcomeRecorder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnOutcomeInstanceID = instanceID
	m.turnOutcomeRecorder = recorder
}

// recordTerminalOutcome retains event if it is one of the two terminal event
// types AC-EXECUTORS-SURVIVAL-004 cares about (EventTypeComplete,
// EventTypeError) and a recorder has been wired. Every non-terminal event
// (permission request/cancelled, MCP attachment, context window, session
// models, ...) is filtered out here rather than at each producer, since
// EventTypeComplete/EventTypeError is emitted from several independent
// places (this manager's own agent-ERROR and process-exit-error sends, and
// the ACP adapter's session/prompt completion and protocol-error paths) that
// all converge on updatesCh regardless of origin -- this is the single
// filter for all of them, called from every point an event is actually
// delivered onto updatesCh (forwardUpdates, sendUpdateBlocking).
func (m *Manager) recordTerminalOutcome(event adapter.AgentEvent) {
	if event.Type != adapter.EventTypeComplete && event.Type != adapter.EventTypeError {
		return
	}
	m.mu.RLock()
	recorder := m.turnOutcomeRecorder
	instanceID := m.turnOutcomeInstanceID
	m.mu.RUnlock()
	if recorder == nil {
		return
	}
	recorder.RetainTurnOutcome(instanceID, event)
}
