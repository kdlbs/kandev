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
	if execution == nil || execution.AgentID != agents.CopilotACPAgentID || deliveryStreamID == "" {
		return
	}
	if !m.canonicalDeliveryProjectorAvailable() {
		return
	}
	capable, ok := rt.(interface{ SupportsCopilotNativeSessionEvents() bool })
	if !ok || !capable.SupportsCopilotNativeSessionEvents() {
		return
	}
	execution.CopilotNativeSessionEvents = true
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
