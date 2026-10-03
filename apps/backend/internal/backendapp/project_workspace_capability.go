package backendapp

import (
	"context"

	agenttypes "github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

type projectWorkspaceAgentStore interface {
	GetAgent(context.Context, string) (*settingsmodels.Agent, error)
}

type projectWorkspaceAgentRegistry interface {
	Get(string) (agenttypes.Agent, bool)
}

func projectWorkspaceAgentSupported(
	ctx context.Context,
	profileAgentID string,
	profiles projectWorkspaceAgentStore,
	registry projectWorkspaceAgentRegistry,
) bool {
	if profiles == nil || registry == nil || profileAgentID == "" {
		return false
	}
	agent, err := profiles.GetAgent(ctx, profileAgentID)
	if err != nil || agent == nil {
		return false
	}
	registered, ok := registry.Get(agent.Name)
	capability, supported := registered.(agenttypes.ProjectWorkspaceDirectoriesAgent)
	return ok && supported && capability.SupportsProjectWorkspaceDirectories()
}
