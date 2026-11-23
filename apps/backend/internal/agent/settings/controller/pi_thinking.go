package controller

import (
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func canonicalizePiThinkingProfile(agentName string, profile *models.AgentProfile, modeOnlyUpdate bool) {
	if agentName != acpcompat.PiAgentID {
		return
	}
	const configID = acpcompat.PiThinkingConfigID
	if modeOnlyUpdate {
		delete(profile.ConfigOptions, configID)
	}
	if profile.ConfigOptions[configID] == "" && profile.Mode != "" {
		if profile.ConfigOptions == nil {
			profile.ConfigOptions = make(map[string]string)
		}
		profile.ConfigOptions[configID] = profile.Mode
	}
	profile.Mode = ""
}
