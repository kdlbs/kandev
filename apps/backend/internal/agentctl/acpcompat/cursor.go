package acpcompat

import "strings"

const (
	CursorAgentID = "cursor-acp"

	// ParameterizedModelPickerMetaKey opts the client into cursor-agent's
	// "parameterized" model picker. cursor-agent decides its picker mode once,
	// from the initialize handshake, and the choice is not renegotiable later:
	//
	//	absent  -> "variants" mode. Models are advertised as exploded strings
	//	           ("grok-4.5[effort=high,fast=true]") and the per-model
	//	           parameters are baked in. Cursor pins grok-4.5 and
	//	           composer-2.5 to fast=true there and offers no other row, so
	//	           those two models can only be run at the fast tier —
	//	           session/set_model rejects any value that is not on the
	//	           advertised list.
	//	present -> "parameterized" mode. Models are advertised as bare ids
	//	           ("grok-4.5") and their parameters become ordinary session
	//	           config options ("effort", "fast"), settable through
	//	           session/set_config_option like any other agent's knobs.
	//
	// Advertising this is what makes a regular-tier Cursor session expressible.
	// Zed carries the same opt-in for the same reason (zed-industries/zed#59694).
	//
	// The two modes advertise disjoint model ids. Existing variant IDs are
	// migrated to the bare model plus config options before the opt-in is used,
	// while the fail-closed SetModel in the lifecycle session manager prevents a
	// bare profile from silently running at the fast tier if the opt-in stops
	// taking effect.
	ParameterizedModelPickerMetaKey = "parameterizedModelPicker"
)

// ClientCapabilityMeta builds the ACP `_meta` map a Kandev client advertises to
// one agent.
//
// This lives here, rather than next to either caller, because the picker mode
// must be identical across all three ACP handshake consumers: the live session
// adapter, the host-utility capability probe, and one-shot inference. They are
// separate handshakes, and when only one of them opted in, the agent-models
// surface advertised the exploded fast=true rows while sessions ran on the
// bare ids — a model id the UI could not offer and the probe could not validate.
//
// base is copied, never mutated, so callers can pass a shared literal.
func ClientCapabilityMeta(agentID string, base map[string]any) map[string]any {
	meta := make(map[string]any, len(base)+1)
	for k, v := range base {
		meta[k] = v
	}
	if agentID == CursorAgentID {
		meta[ParameterizedModelPickerMetaKey] = true
	}
	return meta
}

// ParseVariantModelID splits a legacy Cursor model ID into its bare model ID
// and encoded session config options. Cursor's variants picker used IDs such
// as "grok-4.5[effort=high,fast=true]"; parameterized picker mode advertises
// "grok-4.5" and exposes those values as separate config options.
//
// Invalid or ambiguous IDs are returned unchanged so callers fail closed
// rather than guessing at a model or option value.
func ParseVariantModelID(modelID string) (string, map[string]string, bool) {
	open := strings.LastIndexByte(modelID, '[')
	if open <= 0 || !strings.HasSuffix(modelID, "]") {
		return modelID, nil, false
	}
	encoded := strings.TrimSpace(modelID[open+1 : len(modelID)-1])
	if encoded == "" {
		return modelID, nil, false
	}

	options := make(map[string]string)
	for _, part := range strings.Split(encoded, ",") {
		key, value, ok := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return modelID, nil, false
		}
		if _, exists := options[key]; exists {
			return modelID, nil, false
		}
		options[key] = value
	}

	return modelID[:open], options, true
}

// MigrateCursorModel converts a legacy Cursor variant ID to the parameterized
// picker representation. Existing config options win over values encoded in
// the old ID so a later explicit profile edit is not overwritten. The input
// map is never mutated.
func MigrateCursorModel(
	agentID string,
	modelID string,
	configOptions map[string]string,
) (string, map[string]string, bool) {
	if agentID != CursorAgentID {
		return modelID, configOptions, false
	}
	bareModelID, variantOptions, ok := ParseVariantModelID(modelID)
	if !ok {
		return modelID, configOptions, false
	}

	merged := make(map[string]string, len(configOptions)+len(variantOptions))
	for key, value := range configOptions {
		merged[key] = value
	}
	for key, value := range variantOptions {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	return bareModelID, merged, true
}

// NormalizeCommandDescription drops cursor-agent's placeholder slash-command
// description. cursor-agent emits a bare dash run ("---") as the description
// for project/user slash commands that have no real description; passing it
// through renders a meaningless "---" in the command palette, so treat it as
// absent. The rewrite is Cursor-scoped: another agent may legitimately
// advertise a dashes-only description, so descriptions from non-Cursor agents
// are returned unchanged.
//
// This lives here, rather than next to either caller, because both the live
// session adapter and the host-utility capability probe surface the same
// commands and must agree; a cleanup applied to only one path leaves the other
// rendering the placeholder.
func NormalizeCommandDescription(agentID, description string) string {
	if agentID != CursorAgentID {
		return description
	}
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return description
	}
	if strings.Trim(trimmed, "-") == "" {
		return ""
	}
	return description
}
