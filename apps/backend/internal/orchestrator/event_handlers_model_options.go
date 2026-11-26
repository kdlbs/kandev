package orchestrator

import (
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type runtimeConfigOptionsUpdate struct {
	options  []streams.ConfigOption
	complete bool
}

func runtimeOptionsFromModelEvent(payload *runtimeapi.AgentStreamEventPayload, source string) runtimeConfigOptionsUpdate {
	event := payload.Data
	data, _ := event.Data.(map[string]any)
	complete, _ := data[streams.ConfigOptionsCompleteKey].(bool)
	displayOnly, _ := data[streams.ConfigOptionsDisplayOnlyKey].(bool)
	complete = complete && !displayOnly
	if complete || len(event.VerifiedConfigOptionIDs) > 0 {
		if _, valid := streams.ModelConfigOptionIDs(event.CurrentModelID, event.ConfigOptions); !valid ||
			event.ACPSessionID == "" || event.SessionSettingsGeneration == 0 {
			return runtimeConfigOptionsUpdate{}
		}
		if complete {
			return runtimeConfigOptionsUpdate{options: event.ConfigOptions, complete: true}
		}
		return runtimeConfigOptionsUpdate{options: streams.ConfigOptionsForIDs(event.ConfigOptions, event.VerifiedConfigOptionIDs)}
	}
	if displayOnly {
		// A successful legacy setter confirms only the option it changed.
		id, _ := data[streams.ConfigOptionsConfigIDKey].(string)
		if id == "" {
			return runtimeConfigOptionsUpdate{}
		}
		return runtimeConfigOptionsUpdate{options: streams.ConfigOptionsForIDs(event.ConfigOptions, []string{id})}
	}
	switch source {
	case streams.ConfigOptionsSourceSessionSetup, streams.ConfigOptionsSourceProviderResponse, streams.ConfigOptionsSourceProviderUpdate:
		return runtimeConfigOptionsUpdate{options: event.ConfigOptions}
	}
	// Host snapshots and unsequenced legacy reports can still merge values.
	if event.ACPSessionID == "" || event.SessionSettingsGeneration == 0 {
		return runtimeConfigOptionsUpdate{options: event.ConfigOptions}
	}
	return runtimeConfigOptionsUpdate{}
}
