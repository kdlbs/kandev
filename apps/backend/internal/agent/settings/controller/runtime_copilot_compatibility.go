package controller

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// copilotPluginCompatibilityPreview projects the selected Copilot runtime's
// explicit-plugin preparation eligibility. Other providers return nil so their
// preview contract is unchanged.
func (c *Controller) copilotPluginCompatibilityPreview(
	ctx context.Context,
	agentName string,
	targetVersion string,
) (*dto.CopilotPluginCompatibilityDTO, error) {
	if agentName != agents.CopilotACPAgentID {
		return nil, nil
	}
	compatibility := &dto.CopilotPluginCompatibilityDTO{
		RuntimeVersion: strings.TrimSpace(targetVersion),
		State:          "not_checked",
	}
	selectedProfileCount, err := c.copilotPluginSelectedProfileCount(ctx)
	if err != nil {
		return nil, err
	}
	compatibility.SelectedProfileCount = selectedProfileCount
	return compatibility, nil
}

// copilotPluginSelectedProfileCount counts non-deleted concrete Copilot
// profiles with at least one explicit plugin selection. A read failure is
// unknown impact, never zero.
func (c *Controller) copilotPluginSelectedProfileCount(ctx context.Context) (*int, error) {
	if c.repo == nil {
		return nil, nil
	}
	agent, err := c.repo.GetAgentByName(ctx, agents.CopilotACPAgentID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	if agent == nil {
		return nil, nil
	}
	profiles, err := c.repo.ListAgentProfiles(ctx, agent.ID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	count := 0
	for _, profile := range profiles {
		if profile == nil || profile.DeletedAt != nil {
			continue
		}
		if len(profile.CopilotSelectedPlugins) > 0 {
			count++
		}
	}
	return &count, nil
}
