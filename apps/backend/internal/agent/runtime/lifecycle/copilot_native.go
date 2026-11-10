package lifecycle

import "github.com/kandev/kandev/internal/agent/agents"

// copilotNativeSessionEventsForLaunch enables Copilot's opt-in native event
// stream only when this launch also has a durable delivery consumer. An older
// agentctl ignores the field; a newer agentctl leaves the subscription off when
// the backend omits it.
func copilotNativeSessionEventsForLaunch(agentType, deliveryStreamID string) bool {
	return agentType == agents.CopilotACPAgentID && deliveryStreamID != ""
}

// applyCopilotNativeLaunchCapability enables the native counter only when the
// provider, durable projector, and authenticated runtime all support it. An
// older retained peer without the optional capability stays usable and reports
// native counting unavailable.
func (m *Manager) applyCopilotNativeLaunchCapability(
	execution *AgentExecution,
	rt ExecutorBackend,
	deliveryStreamID string,
) {
	if execution == nil || !m.copilotNativeLaunchAllowed(rt, execution.AgentID, deliveryStreamID) {
		return
	}
	execution.CopilotNativeSessionEvents = true
}

// copilotNativeLaunchAllowed is the single launch decision for the native
// subscription. A qualified agentctl suppresses its mixed gauge, so the opt-in
// must never be sent unless the backend can durably count the native events.
func (m *Manager) copilotNativeLaunchAllowed(rt ExecutorBackend, agentType, deliveryStreamID string) bool {
	if !copilotNativeSessionEventsForLaunch(agentType, deliveryStreamID) {
		return false
	}
	if !m.canonicalDeliveryProjectorAvailable() {
		return false
	}
	capable, ok := rt.(interface{ SupportsCopilotNativeSessionEvents() bool })
	return ok && capable.SupportsCopilotNativeSessionEvents()
}

// canonicalDeliveryProjectorAvailable reports whether the backend repository
// can commit native receipts and counts inside canonical projection.
func (m *Manager) canonicalDeliveryProjectorAvailable() bool {
	if m == nil || m.streamManager == nil {
		return false
	}
	_, ok := m.streamManager.deliveryRepository().(canonicalAgentDeliveryProjector)
	return ok
}
