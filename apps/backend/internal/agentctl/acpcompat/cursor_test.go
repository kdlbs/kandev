package acpcompat

import (
	"reflect"
	"testing"
)

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

func TestParseVariantModelID(t *testing.T) {
	model, options, ok := ParseVariantModelID("grok-4.5[effort=high,fast=true]")
	if !ok {
		t.Fatal("ParseVariantModelID returned ok=false")
	}
	if model != "grok-4.5" {
		t.Errorf("model = %q, want grok-4.5", model)
	}
	want := map[string]string{"effort": "high", "fast": "true"}
	if !reflect.DeepEqual(options, want) {
		t.Errorf("options = %#v, want %#v", options, want)
	}
}

func TestParseVariantModelID_RejectsMalformedIDs(t *testing.T) {
	for _, model := range []string{
		"grok-4.5",
		"grok-4.5[]",
		"grok-4.5[fast]",
		"grok-4.5[fast=true,fast=false]",
		"[fast=true]",
	} {
		t.Run(model, func(t *testing.T) {
			gotModel, gotOptions, ok := ParseVariantModelID(model)
			if ok || gotModel != model || gotOptions != nil {
				t.Fatalf("ParseVariantModelID(%q) = (%q, %#v, %v), want unchanged and false",
					model, gotModel, gotOptions, ok)
			}
		})
	}
}

func TestMigrateCursorModel_PreservesExplicitOptions(t *testing.T) {
	existing := map[string]string{"effort": "low"}
	model, options, changed := MigrateCursorModel(
		CursorAgentID,
		"grok-4.5[effort=high,fast=true]",
		existing,
	)
	if !changed || model != "grok-4.5" {
		t.Fatalf("MigrateCursorModel() = (%q, %#v, %v), want migrated model", model, options, changed)
	}
	if options["effort"] != "low" || options["fast"] != "true" {
		t.Errorf("options = %#v, want explicit effort and migrated fast", options)
	}
	if _, ok := existing["fast"]; ok {
		t.Error("MigrateCursorModel mutated the input options")
	}
}

func TestNormalizeCommandDescription_CursorClearsDashPlaceholder(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "single dash", in: "---", want: ""},
		{name: "padded dash run", in: "  ---  ", want: ""},
		{name: "one dash", in: "-", want: ""},
		{name: "real description", in: "Commit changes", want: "Commit changes"},
		{name: "already empty", in: "", want: ""},
		{name: "dash in text kept", in: "foo - bar", want: "foo - bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeCommandDescription(CursorAgentID, tc.in); got != tc.want {
				t.Errorf("NormalizeCommandDescription(cursor, %q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A dashes-only description from a non-Cursor agent is legitimate and must be
// returned unchanged; the placeholder cleanup is Cursor-scoped.
func TestNormalizeCommandDescription_OtherAgentsUnchanged(t *testing.T) {
	for _, agentID := range []string{"claude-acp", "codex-acp", GrokAgentID, ""} {
		if got := NormalizeCommandDescription(agentID, "---"); got != "---" {
			t.Errorf("NormalizeCommandDescription(%q, %q) = %q, want %q (non-Cursor untouched)",
				agentID, "---", got, "---")
		}
	}
}
