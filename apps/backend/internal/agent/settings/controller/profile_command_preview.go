package controller

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

var ErrProfileCommandPreviewMissing = errors.New("profile command preview unavailable")

// PreviewSavedProfileCommand exposes only the safe composition of a saved global profile.
func (c *Controller) PreviewSavedProfileCommand(ctx context.Context, profileID string) (*dto.CommandPreviewResponse, error) {
	profile, err := c.repo.GetAgentProfile(ctx, profileID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && profile == nil) {
		return nil, ErrProfileCommandPreviewMissing
	}
	if err != nil {
		return nil, err
	}
	if profile.WorkspaceID != "" || profile.Role != "" || profile.AgentID == agents.DynamicAgentID {
		return nil, ErrProfileCommandPreviewMissing
	}
	agent, err := c.repo.GetAgent(ctx, profile.AgentID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && agent == nil) {
		return nil, ErrProfileCommandPreviewMissing
	}
	if err != nil {
		return nil, err
	}
	return c.PreviewAgentCommand(ctx, agent.Name, CommandPreviewRequest{
		SafeOnly: true, Model: profile.Model, CLIPassthrough: profile.CLIPassthrough,
		CLIFlags: cliFlagsToDTO(profile.CLIFlags), CommandPrefix: profile.CommandPrefix,
		PermissionSettings: map[string]bool{
			agents.PermissionKeyAutoApprove: profile.AutoApprove,
			"allow_indexing":                profile.AllowIndexing,
			"dangerously_skip_permissions":  profile.DangerouslySkipPermissions,
		},
	})
}
