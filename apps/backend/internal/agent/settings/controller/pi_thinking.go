package controller

import (
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func canonicalizePiThinkingProfile(agentName string, profile *models.AgentProfile) {
	if agentName != acpcompat.PiAgentID {
		return
	}
	const configID = acpcompat.PiThinkingConfigID
	if profile.ConfigOptions[configID] == "" && profile.Mode != "" {
		if profile.ConfigOptions == nil {
			profile.ConfigOptions = make(map[string]string)
		}
		profile.ConfigOptions[configID] = profile.Mode
	}
	profile.Mode = ""
}
