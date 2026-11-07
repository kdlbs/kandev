package process

import (
	"errors"

	"go.uber.org/zap"
)

func (m *Manager) markProcessStarting() {
	m.logger.Info("starting agent process",
		zap.String("protocol", string(m.cfg.Protocol)),
		zap.Strings("args", m.cfg.AgentArgs),
		zap.String("workdir", m.cfg.WorkDir),
		zap.Int("mcp_servers", len(m.cfg.McpServers)))
	m.status.Store(StatusStarting)
	m.exitCode.Store(-1)
	m.exitErr.Store(errorWrapper{err: nil})
}

func finishProcessFlow(finish func(string, string), result *error, failurePhase, successOutcome *string) {
	if *result != nil {
		finish("failed", *failurePhase)
	} else {
		finish(*successOutcome, "")
	}
}

func (m *Manager) processAdmissionFailureReason(err error) string {
	if errors.Is(err, ErrManagerStopping) {
		return "admission_closed"
	}
	if m.inferenceOwner != nil {
		return m.inferenceOwner.ActivationFailureReason()
	}
	return "owner_unavailable"
}
