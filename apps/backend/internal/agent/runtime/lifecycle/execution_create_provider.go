package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type executionCreateProvider struct {
	agentConfig    agents.Agent
	profileInfo    *AgentProfileInfo
	commandOptions agents.CommandOptions
}

// Workspace-only admission can retain local agentctl access when its historical
// provider is unavailable. Valid provider preparation stays unchanged, as do all
// explicit agent launches and runtimes without provider-free local creation.
func (m *Manager) prepareExecutionProvider(ctx context.Context, info *WorkspaceInfo, agentLaunch bool) (*executionCreateProvider, error) {
	if info.AgentID == "" {
		return workspaceOnlyProviderFallback(info, agentLaunch, fmt.Errorf("agent ID is required in WorkspaceInfo"))
	}
	config, err := m.resolveLaunchableAgent(info.AgentID)
	if err != nil {
		return workspaceOnlyProviderFallback(info, agentLaunch, err)
	}
	if !agentLaunch && !config.Enabled() {
		return workspaceOnlyProviderFallback(info, false, fmt.Errorf("agent type %q is disabled", info.AgentID))
	}
	options, err := m.resolveManagedRuntimeCommandOptions(ctx, models.ExecutorType(info.ExecutorType).Runtime(), config)
	if err != nil {
		return nil, err
	}
	return &executionCreateProvider{
		agentConfig:    config,
		profileInfo:    m.resolveWorkspaceExecutionProfile(ctx, workspaceExecutionProfileID(info)),
		commandOptions: options,
	}, nil
}

func workspaceOnlyProviderFallback(info *WorkspaceInfo, agentLaunch bool, cause error) (*executionCreateProvider, error) {
	if !agentLaunch && models.ExecutorType(info.ExecutorType).Runtime() == agentruntime.RuntimeStandalone {
		return &executionCreateProvider{}, nil
	}
	return nil, cause
}
