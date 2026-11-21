package acp

import (
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) modeConfigOption(options []streams.ConfigOption) (streams.ConfigOption, bool) {
	return acpcompat.SessionModeOption(a.agentID, options)
}

func (a *Adapter) currentModeFromConfig(options []streams.ConfigOption) string {
	option, ok := a.modeConfigOption(options)
	if !ok {
		return ""
	}
	return option.CurrentValue
}

func (a *Adapter) sessionModesFromConfig(options []streams.ConfigOption) ([]streams.SessionModeInfo, bool) {
	option, ok := a.modeConfigOption(options)
	if !ok {
		return nil, a.agentID == acpcompat.PiAgentID
	}
	return sessionModesFromConfigOption(option), true
}

func (a *Adapter) isModeConfigOptionID(options []streams.ConfigOption, id string) bool {
	option, ok := a.modeConfigOption(options)
	return ok && option.ID == id
}

func (a *Adapter) clearUnknownThinkingModeLocked(options []streams.ConfigOption) {
	if a.agentID == acpcompat.PiAgentID && a.currentModeFromConfig(options) == "" {
		a.currentModeID = ""
	}
}
