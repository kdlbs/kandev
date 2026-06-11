package acp

import (
	acp "github.com/coder/acp-go-sdk"
)

// modelInfo is the kandev-internal representation of a single model advertised
// by an ACP session. v0.13.0 of the SDK exposed an equivalent acp.ModelInfo
// type; v0.13.5 removed it in favor of routing model selection through
// SessionConfigOption with category="model". We keep the local shape so the
// rest of the adapter (caching, convergence events, fallback resolution)
// doesn't have to spread the configOption walk across every call site.
type modelInfo struct {
	ModelId     string
	Name        string
	Description *string
	Meta        map[string]any
}

// sessionModelState mirrors the removed v0.13.0 acp.SessionModelState shape.
// Populated by initialSessionModelState from the session response's typed
// SessionConfigOption list (with _meta fallback for legacy agents).
type sessionModelState struct {
	CurrentModelId  string
	AvailableModels []modelInfo
}

// modelsFromConfigOptions extracts the current model id and the available
// model list from a typed SessionConfigOption slice, looking for the option
// whose Select.Category is "model". Returns nil when no such option exists.
func modelsFromConfigOptions(opts []acp.SessionConfigOption) *sessionModelState {
	for _, opt := range opts {
		if opt.Select == nil {
			continue
		}
		sel := opt.Select
		isModel := string(sel.Id) == configOptionIDModel ||
			(sel.Category != nil && string(*sel.Category) == configOptionIDModel)
		if !isModel {
			continue
		}
		state := &sessionModelState{
			CurrentModelId:  string(sel.CurrentValue),
			AvailableModels: modelsFromSelectOptions(sel.Options),
		}
		return state
	}
	return nil
}

// modelsFromSelectOptions flattens the ungrouped/grouped variants of a
// SessionConfigSelectOptions payload into a flat []modelInfo. Grouped options
// are walked in declaration order so the resulting slice preserves the agent's
// presentation order.
func modelsFromSelectOptions(opts acp.SessionConfigSelectOptions) []modelInfo {
	if opts.Ungrouped != nil {
		return convertSelectOptionList(*opts.Ungrouped)
	}
	if opts.Grouped != nil {
		var out []modelInfo
		for _, group := range *opts.Grouped {
			out = append(out, convertSelectOptionList(group.Options)...)
		}
		return out
	}
	return nil
}

// modelsFromLegacy extracts the session model state from the pre-v0.13.5
// top-level `models` field still emitted by agents (e.g. auggie 0.29.x) that
// haven't migrated to the SessionConfigOption(category="model") surface.
// Upstream removed this field from the SDK struct in v0.13.5; the kdlbs fork
// restores read-only parsing via acp.LegacyModels. Returns nil when the field
// is absent or carries no models.
func modelsFromLegacy(legacy *acp.LegacyModels) *sessionModelState {
	if legacy == nil || len(legacy.AvailableModels) == 0 {
		return nil
	}
	out := make([]modelInfo, 0, len(legacy.AvailableModels))
	for _, m := range legacy.AvailableModels {
		out = append(out, modelInfo{
			ModelId:     m.ModelId,
			Name:        m.Name,
			Description: m.Description,
			Meta:        m.Meta,
		})
	}
	return &sessionModelState{
		CurrentModelId:  legacy.CurrentModelId,
		AvailableModels: out,
	}
}

func convertSelectOptionList(in []acp.SessionConfigSelectOption) []modelInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]modelInfo, 0, len(in))
	for _, o := range in {
		out = append(out, modelInfo{
			ModelId:     string(o.Value),
			Name:        o.Name,
			Description: o.Description,
			Meta:        o.Meta,
		})
	}
	return out
}
