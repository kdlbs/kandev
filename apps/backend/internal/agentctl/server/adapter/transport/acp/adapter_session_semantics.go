package acp

import (
	"slices"

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

// The model and paired mode events describe one committed settings snapshot.
func (a *Adapter) modeEventForConfigSnapshot(event AgentEvent) *AgentEvent {
	modes, found := a.sessionModesFromConfig(event.ConfigOptions)
	if !found {
		return nil
	}
	return &AgentEvent{
		Type:                      streams.EventTypeSessionMode,
		SessionID:                 event.SessionID,
		CurrentModeID:             a.currentModeFromConfig(event.ConfigOptions),
		AvailableModes:            modes,
		SessionSettingsPolicy:     event.SessionSettingsPolicy,
		SessionSettingsGeneration: event.SessionSettingsGeneration,
	}
}

func (a *Adapter) piModelChangeOptionsLocked(modelID string, fallback []streams.ConfigOption) []streams.ConfigOption {
	// A matching notification may already have supplied the new catalog.
	if currentModelFromConfig(a.availableConfigOptions) == modelID {
		return a.availableConfigOptions
	}
	return slices.DeleteFunc(fallback, func(option streams.ConfigOption) bool {
		return option.Category == acpcompat.ThoughtLevelCategory
	})
}
