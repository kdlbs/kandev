package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"go.uber.org/zap"
)

// A settled replacement has no admitted turn. Its process failures belong to
// the replacement receipt and must not terminalize the task's previous turn.
func (m *Manager) suppressProviderReplacementTerminal(event adapter.AgentEvent) bool {
	if event.Type != adapter.EventTypeComplete && event.Type != adapter.EventTypeError {
		return false
	}
	m.providerAdmission.mu.Lock()
	owned := m.providerAdmission.operationID != ""
	m.providerAdmission.mu.Unlock()
	return owned && m.activeDeliverySubmissionID() == ""
}

func (m *Manager) publishProviderExitError(event adapter.AgentEvent) {
	if m.suppressProviderReplacementTerminal(event) {
		return
	}
	m.recordTerminalOutcome(&event)
	if persisted, err := m.persistDeliveryEvent(event); err != nil {
		m.logger.Error("failed to commit durable exit error event", zap.Error(err))
	} else {
		m.sendUpdateBlockingRecorded(persisted)
	}
}
