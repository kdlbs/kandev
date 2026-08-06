package acpcompat

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
	// The two modes advertise disjoint model ids, which makes the pin the
	// backstop: a profile pinned to bare "grok-4.5" cannot match anything in
	// variants mode, so if this opt-in ever stops taking effect the fail-closed
	// SetModel in the lifecycle session manager refuses to start the session
	// instead of quietly running it at the fast tier.
	ParameterizedModelPickerMetaKey = "parameterizedModelPicker"
)

// ClientCapabilityMeta builds the ACP `_meta` map a Kandev client advertises to
// one agent.
//
// This lives here, rather than next to either caller, because the picker mode
// must be identical on BOTH ACP entry points: the live session adapter and the
// host-utility capability probe. They are separate handshakes, and when only
// one of them opted in, the agent-models surface advertised the exploded
// fast=true rows while sessions ran on the bare ids — a model id the UI could
// not offer and the probe could not validate.
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
