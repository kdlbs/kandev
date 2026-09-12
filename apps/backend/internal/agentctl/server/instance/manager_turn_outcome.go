package instance

import "github.com/kandev/kandev/internal/agentctl/types/streams"

// RetainTurnOutcome assigns a fresh turn identifier and records event as the
// named instance's new last terminal outcome (AC-EXECUTORS-SURVIVAL-004.1).
// Returns ok=false without allocating an identifier when the instance is not
// found, so a caller racing instance teardown never burns an identifier for
// an outcome nobody can retrieve.
func (m *Manager) RetainTurnOutcome(instanceID string, event streams.AgentEvent) (turnID int64, ok bool) {
	inst, found := m.GetInstance(instanceID)
	if !found {
		return 0, false
	}
	turnID = m.turnIDSeq.Add(1)
	inst.turnOutcome.Retain(turnID, event)
	return turnID, true
}

// PeekTurnOutcome returns the named instance's retained outcome without
// discarding it (AC-EXECUTORS-SURVIVAL-004.6). instanceFound distinguishes
// "no such instance" from "instance exists, nothing retained" so the HTTP
// handler can 404 only on the former.
func (m *Manager) PeekTurnOutcome(instanceID string) (outcome TurnOutcome, hasOutcome, instanceFound bool) {
	inst, found := m.GetInstance(instanceID)
	if !found {
		return TurnOutcome{}, false, false
	}
	outcome, hasOutcome = inst.turnOutcome.Peek()
	return outcome, hasOutcome, true
}

// AckTurnOutcome discards the named instance's retained outcome only if
// turnID names the one currently retained. Naming any other identifier --
// including one for an instance that no longer exists -- is accepted and
// changes nothing (AC-EXECUTORS-SURVIVAL-004.6), so a retried acknowledgement
// is always safe to call again.
func (m *Manager) AckTurnOutcome(instanceID string, turnID int64) {
	inst, found := m.GetInstance(instanceID)
	if !found {
		return
	}
	inst.turnOutcome.Ack(turnID)
}
