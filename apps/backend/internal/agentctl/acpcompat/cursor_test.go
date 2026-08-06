package acpcompat

import "testing"

func TestClientCapabilityMeta_CursorGetsParameterizedPicker(t *testing.T) {
	meta := ClientCapabilityMeta(CursorAgentID, map[string]any{"terminal_output": true})

	if meta[ParameterizedModelPickerMetaKey] != true {
		t.Errorf("meta[%q] = %v, want true (meta: %v)",
			ParameterizedModelPickerMetaKey, meta[ParameterizedModelPickerMetaKey], meta)
	}
	if meta["terminal_output"] != true {
		t.Errorf("base entries dropped: meta = %v", meta)
	}
}

func TestClientCapabilityMeta_OtherAgentsUnchanged(t *testing.T) {
	for _, agentID := range []string{"claude-acp", "codex-acp", GrokAgentID, "opencode-acp", ""} {
		meta := ClientCapabilityMeta(agentID, map[string]any{"terminal_output": true})
		if _, ok := meta[ParameterizedModelPickerMetaKey]; ok {
			t.Errorf("%q carries %q; it is Cursor-only (meta: %v)",
				agentID, ParameterizedModelPickerMetaKey, meta)
		}
		if meta["terminal_output"] != true {
			t.Errorf("%q: base entries dropped: meta = %v", agentID, meta)
		}
	}
}

// The probe passes nil, so a nil base must still produce a usable map rather
// than panicking or returning a nil the caller writes into.
func TestClientCapabilityMeta_NilBase(t *testing.T) {
	meta := ClientCapabilityMeta(CursorAgentID, nil)
	if meta[ParameterizedModelPickerMetaKey] != true {
		t.Errorf("nil base lost the picker opt-in: %v", meta)
	}

	if got := ClientCapabilityMeta("claude-acp", nil); len(got) != 0 {
		t.Errorf("non-cursor with nil base = %v, want empty", got)
	}
}

// Callers pass shared literals; mutating one agent's meta must not leak into
// the next agent's handshake.
func TestClientCapabilityMeta_DoesNotMutateBase(t *testing.T) {
	base := map[string]any{"terminal_output": true}

	ClientCapabilityMeta(CursorAgentID, base)

	if _, ok := base[ParameterizedModelPickerMetaKey]; ok {
		t.Errorf("base map was mutated: %v", base)
	}
	if len(base) != 1 {
		t.Errorf("base map grew: %v", base)
	}
}
