package acp

import (
	"slices"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type modelCatalogEvidence struct {
	modelID     string
	generation  uint64
	complete    bool
	verified    []string
	displayOnly bool
}

func (a *Adapter) stampModelCatalogLocked(event *AgentEvent, raw []acp.SessionConfigOption, reportedModelID string) {
	a.modelCatalog = modelCatalogEvidence{
		modelID: event.CurrentModelID, generation: event.SessionSettingsGeneration,
	}
	ids, valid := streams.ModelConfigOptionIDs(event.CurrentModelID, event.ConfigOptions)
	if !valid || raw == nil || event.SessionSettingsGeneration == 0 {
		return
	}
	observed := make(map[string]struct{}, len(raw))
	modelReported := reportedModelID == event.CurrentModelID
	for _, option := range raw {
		if option.Select == nil {
			continue
		}
		selected := option.Select
		observed[string(selected.Id)] = struct{}{}
		if selected.Id == configOptionIDModel || (selected.Category != nil && *selected.Category == acp.SessionConfigOptionCategoryModel) {
			modelReported = string(selected.CurrentValue) == event.CurrentModelID
		}
	}
	if !modelReported {
		return
	}
	verified := slices.DeleteFunc(ids, func(id string) bool {
		_, ok := observed[id]
		return !ok
	})
	complete := len(verified) == len(raw) && len(verified) == len(event.ConfigOptions)
	a.recordModelCatalogLocked(event, verified, complete)
}

func (a *Adapter) recordModelCatalogLocked(event *AgentEvent, verified []string, complete bool) {
	if !complete && len(verified) == 0 {
		return
	}
	a.modelCatalog = modelCatalogEvidence{
		modelID: event.CurrentModelID, generation: event.SessionSettingsGeneration, complete: complete,
		displayOnly: a.modelCatalog.displayOnly,
	}
	if complete {
		event.Data[streams.ConfigOptionsCompleteKey] = true
		return
	}
	a.modelCatalog.verified = verified
	event.VerifiedConfigOptionIDs = verified
}

func verifiedModelStateFromEvent(event AgentEvent) *streams.SessionModelState {
	complete, _ := event.Data[streams.ConfigOptionsCompleteKey].(bool)
	if !complete && len(event.VerifiedConfigOptionIDs) == 0 {
		return nil
	}
	return &streams.SessionModelState{
		SessionID:                 event.SessionID,
		CurrentModelID:            event.CurrentModelID,
		Models:                    cloneSessionModels(event.SessionModels),
		ConfigOptions:             cloneConfigOptions(event.ConfigOptions),
		SessionSettingsGeneration: event.SessionSettingsGeneration,
		ConfigOptionsComplete:     complete,
		VerifiedConfigOptionIDs:   slices.Clone(event.VerifiedConfigOptionIDs),
	}
}
