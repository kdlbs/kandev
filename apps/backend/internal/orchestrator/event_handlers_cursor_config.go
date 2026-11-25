package orchestrator

import (
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type runtimeConfigOptionsUpdate struct {
	options  []streams.ConfigOption
	complete bool
}

func runtimeOptionsFromModelEvent(payload *runtimeapi.AgentStreamEventPayload, source string) runtimeConfigOptionsUpdate {
	event := payload.Data
	if payload.AgentType != acpcompat.CursorAgentID {
		return runtimeConfigOptionsUpdate{options: event.ConfigOptions}
	}
	if event.CurrentModelID == "" {
		return runtimeConfigOptionsUpdate{}
	}
	for _, option := range event.ConfigOptions {
		if (option.ID == sessionModelConfigKey || option.Category == sessionModelConfigKey) &&
			option.CurrentValue == event.CurrentModelID {
			return cursorRuntimeOptionsFromReport(payload, source)
		}
	}
	return runtimeConfigOptionsUpdate{}
}

func cursorRuntimeOptionsFromReport(payload *runtimeapi.AgentStreamEventPayload, source string) runtimeConfigOptionsUpdate {
	event := payload.Data
	data, _ := event.Data.(map[string]any)
	complete, _ := data[streams.ConfigOptionsCompleteKey].(bool)
	if complete && event.SessionSettingsGeneration != 0 {
		return runtimeConfigOptionsUpdate{options: event.ConfigOptions, complete: true}
	}
	switch source {
	case streams.ConfigOptionsSourceSessionSetup, "provider_response", "provider_update":
		return runtimeConfigOptionsUpdate{options: event.ConfigOptions}
	default:
		return runtimeConfigOptionsUpdate{}
	}
}
