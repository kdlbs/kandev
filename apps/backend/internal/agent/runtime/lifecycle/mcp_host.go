package lifecycle

import (
	"fmt"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
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

func (m *Manager) agentMCPURL(execution *AgentExecution, client *agentctl.Client, port int, path string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("agent helper is unavailable for native MCP URL construction")
	}
	instanceID, scoped := builtinMCPRouteMode(client)
	return agentctlconfig.MCPServerURLForInstance(m.agentMCPHost(execution), port, instanceID, scoped, path)
}
