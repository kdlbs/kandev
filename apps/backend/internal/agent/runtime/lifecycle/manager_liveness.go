package lifecycle

import "context"

// agentctlAbsenceConfirmer is an optional runtime capability for positive
// process-absence evidence when the agentctl HTTP endpoint is unavailable.
// False or an error leaves liveness unknown; neither authorizes reclaim.
type agentctlAbsenceConfirmer interface {
	ConfirmAgentctlAbsent(ctx context.Context, instanceID string) (bool, error)
}

func (m *Manager) confirmAgentctlAbsent(ctx context.Context, execution *AgentExecution) bool {
	if ctx.Err() != nil || m.executorRegistry == nil || execution.RuntimeName == "" {
		return false
	}
	rt, err := m.executorRegistry.GetBackend(execution.RuntimeName)
	if err != nil {
		return false
	}
	prober, ok := rt.(agentctlAbsenceConfirmer)
	if !ok {
		return false
	}
	absent, err := prober.ConfirmAgentctlAbsent(ctx, execution.ID)
	return absent && err == nil && ctx.Err() == nil
}
