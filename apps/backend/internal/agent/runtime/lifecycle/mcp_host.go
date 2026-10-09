package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/executor"
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentruntime"
)

func (m *Manager) agentMCPHost(execution *AgentExecution) string {
	if execution == nil || execution.RuntimeName != agentruntime.RuntimeStandalone || m.executorRegistry == nil {
		return "localhost"
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return "localhost"
	}
	standalone, ok := backend.(*StandaloneExecutor)
	if !ok {
		return "localhost"
	}
	return agentctlconfig.MCPReachableHost(standalone.host)
}

func (m *Manager) agentMCPURL(execution *AgentExecution, port int, path string) string {
	return agentctlconfig.MCPServerURL(m.agentMCPHost(execution), port, path)
}
