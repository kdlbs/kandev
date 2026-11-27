package lifecycle

import "github.com/kandev/kandev/internal/common/processidentity"

// Retired leases are detached only after proving the original process and its
// owned process group have terminated. Their SQL row remains for caller CAS.
// The caller must hold agentctlLifecycleMu.
func retiredExecutionProcessTerminated(execution *AgentExecution) bool {
	client := execution.currentAgentCtlClient()
	if client == nil || client.RuntimeEpoch() != execution.runtimeEpoch {
		return false
	}
	terminated, err := processidentity.OwnedSessionTerminated(client.RuntimeProcessIdentity())
	return err == nil && terminated
}
