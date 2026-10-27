package lifecycle

import "context"

func (m *Manager) SetSandboxRecoveryAdmission(admit func(context.Context, *AgentExecution) error) {
	m.sandboxRecoveryAdmission = admit
}
func (m *Manager) admitSandboxRecovery(ctx context.Context, execution *AgentExecution) error {
	if m.sandboxRecoveryAdmission == nil {
		return nil
	}
	return m.sandboxRecoveryAdmission(ctx, execution)
}
