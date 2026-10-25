package lifecycle

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"go.uber.org/zap"
)

func (m *Manager) reportModeOutcome(execution *AgentExecution, result agentctl.ModeResult) {
	if execution == nil || result.Requested == "" {
		return
	}
	if result.Applied() {
		m.logger.Info("session mode applied",
			zap.String("execution_id", execution.ID),
			zap.String("requested_mode", result.Requested),
			zap.String("effective_mode", result.Effective),
			zap.Bool("confirmed", result.Confirmed))
		return
	}
	m.logger.Warn("session mode was not confirmed by the agent",
		zap.String("execution_id", execution.ID),
		zap.String("requested_mode", result.Requested),
		zap.String("effective_mode", result.Effective),
		zap.Bool("confirmed", result.Confirmed))
}
