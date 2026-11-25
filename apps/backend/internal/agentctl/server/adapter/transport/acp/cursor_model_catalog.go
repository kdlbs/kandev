package acp

import (
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) stampModelCatalogLocked(event *AgentEvent, raw []acp.SessionConfigOption) {
	a.configOptionsComplete = a.agentID == cursorAgentID &&
		event.SessionSettingsGeneration != 0 && len(raw) == len(event.ConfigOptions) &&
		hasCompleteModelCatalog(event.CurrentModelID, event.ConfigOptions)
	if a.configOptionsComplete {
		event.Data[streams.ConfigOptionsCompleteKey] = true
	}
}

func hasCompleteModelCatalog(modelID string, options []streams.ConfigOption) bool {
	if strings.TrimSpace(modelID) == "" {
		return false
	}
	ids := make(map[string]struct{}, len(options))
	modelOptions := 0
	for _, option := range options {
		id := strings.TrimSpace(option.ID)
		if _, exists := ids[id]; id == "" || exists {
			return false
		}
		ids[id] = struct{}{}
		if id == configOptionIDModel || option.Category == configOptionIDModel {
			if option.CurrentValue != modelID {
				return false
			}
			modelOptions++
		}
	}
	return modelOptions == 1
}

func completeModelStateFromEvent(event AgentEvent) *streams.SessionModelState {
	if complete, _ := event.Data[streams.ConfigOptionsCompleteKey].(bool); !complete {
		return nil
	}
	return &streams.SessionModelState{
		SessionID:                 event.SessionID,
		CurrentModelID:            event.CurrentModelID,
		Models:                    cloneSessionModels(event.SessionModels),
		ConfigOptions:             cloneConfigOptions(event.ConfigOptions),
		SessionSettingsGeneration: event.SessionSettingsGeneration,
		ConfigOptionsComplete:     true,
	}
}
