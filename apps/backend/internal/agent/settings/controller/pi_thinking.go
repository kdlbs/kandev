package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func (c *Controller) canonicalizePiThinkingProfile(ctx context.Context, profile *models.AgentProfile) error {
	name, err := c.agentNameForProfile(ctx, profile)
	if err != nil {
		return err
	}
	if name != acpcompat.PiAgentID {
		return nil
	}
	const configID = "thought_level"
	if profile.ConfigOptions[configID] == "" && profile.Mode != "" {
		if profile.ConfigOptions == nil {
			profile.ConfigOptions = make(map[string]string)
		}
		profile.ConfigOptions[configID] = profile.Mode
	}
	profile.Mode = ""
	return nil
}
