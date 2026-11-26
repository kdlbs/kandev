package streams

import "strings"

// ModelConfigOptionIDs checks that the projected IDs are unique and agree with
// the active model. The transport establishes the model and report provenance.
func ModelConfigOptionIDs(modelID string, options []ConfigOption) ([]string, bool) {
	if strings.TrimSpace(modelID) == "" {
		return nil, false
	}
	seen := make(map[string]struct{}, len(options))
	ids := make([]string, 0, len(options))
	modelOptionSeen := false
	for _, option := range options {
		id := option.ID
		if _, exists := seen[id]; exists || id == "" || strings.TrimSpace(id) != id {
			return nil, false
		}
		if id == "model" || option.Category == "model" {
			if modelOptionSeen || option.CurrentValue != modelID {
				return nil, false
			}
			modelOptionSeen = true
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}

// ConfigOptionsForIDs selects the observed options in their advertised order.
func ConfigOptionsForIDs(options []ConfigOption, ids []string) []ConfigOption {
	if len(ids) == 0 {
		return nil
	}
	verified := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		verified[id] = struct{}{}
	}
	var selected []ConfigOption
	for _, option := range options {
		if _, ok := verified[option.ID]; ok {
			selected = append(selected, option)
		}
	}
	return selected
}

// HasConfigOptionEvidence reports whether the state verifies any catalog entries.
func (state *SessionModelState) HasConfigOptionEvidence() bool {
	return state != nil && (state.ConfigOptionsComplete || len(state.VerifiedConfigOptionIDs) > 0)
}
