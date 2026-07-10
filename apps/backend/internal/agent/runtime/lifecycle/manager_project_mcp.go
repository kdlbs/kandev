package lifecycle

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// materializeRuntimeProjectMCP writes project-local MCP config for protocol-mode
// agents whose underlying CLI does not consume ACP session/new mcpServers.
func (m *Manager) materializeRuntimeProjectMCP(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) error {
	if execution == nil || agentConfig == nil {
		return nil
	}
	rt := agentConfig.Runtime()
	if rt == nil || rt.ProjectMCPStrategy == nil {
		return nil
	}
	servers, err := m.runtimeProjectMCPServers(ctx, execution, agentConfig)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return nil
	}
	artifacts, err := rt.ProjectMCPStrategy.BuildPassthroughMCP(servers, m.passthroughMCPPaths(execution))
	if err != nil {
		return fmt.Errorf("build project MCP config: %w", err)
	}
	return m.writePassthroughMCPFiles(execution, artifacts.Files)
}

func (m *Manager) runtimeProjectMCPServers(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) ([]agentctltypes.McpServer, error) {
	servers, err := m.passthroughMCPServers(ctx, execution, agentConfig)
	if err == nil {
		return servers, nil
	}
	if passthroughMCPConfigPort(execution) <= 0 {
		m.logger.Warn("skipping project MCP config: agentctl instance port unavailable",
			zap.String("execution_id", execution.ID),
			zap.String("agent_id", agentConfig.ID()))
		return nil, nil
	}
	return nil, err
}
