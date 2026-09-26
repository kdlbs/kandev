package manifest

import (
	"strings"
	"testing"
)

// keybindingManifest returns a valid bundle-carrying manifest holding exactly
// the supplied keybindings.
func keybindingManifest(t *testing.T, keybindings ...UIKeybinding) *Manifest {
	t.Helper()
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Keybindings = keybindings
	return m
}

func TestValidate_AllowInEditorAcceptsModifierCombos(t *testing.T) {
	for _, combo := range []string{"mod+shift+m", "ctrl+m", "cmd+shift+v", "alt+k", "meta+j", "option+p"} {
		m := keybindingManifest(t, UIKeybinding{
			ID:            "toggle-dictation",
			Default:       combo,
			Description:   "Toggle dictation",
			AllowInEditor: true,
		})
		if err := m.Validate(); err != nil {
			t.Errorf("Validate() with allow_in_editor and combo %q: unexpected error: %v", combo, err)
		}
	}
}

func TestValidate_AllowInEditorRejectsCombosReachableByTyping(t *testing.T) {
	for _, combo := range []string{"m", "shift+m", "f5"} {
		m := keybindingManifest(t, UIKeybinding{
			ID:            "toggle-dictation",
			Default:       combo,
			Description:   "Toggle dictation",
			AllowInEditor: true,
		})
		err := m.Validate()
		if err == nil {
			t.Errorf("Validate() with allow_in_editor and combo %q: expected an error, got nil", combo)
			continue
		}
		if !strings.Contains(err.Error(), "allow_in_editor") {
			t.Errorf("Validate() with combo %q: error should name allow_in_editor, got %v", combo, err)
		}
	}
}

func TestValidate_ModifierlessComboStaysValidWithoutAllowInEditor(t *testing.T) {
	m := keybindingManifest(t, UIKeybinding{
		ID:          "toggle-dictation",
		Default:     "f5",
		Description: "Toggle dictation",
	})
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_AllowInEditorDoesNotMaskAnInvalidCombo(t *testing.T) {
	m := keybindingManifest(t, UIKeybinding{
		ID:            "toggle-dictation",
		Default:       "mod+nope",
		Description:   "Toggle dictation",
		AllowInEditor: true,
	})
	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected an error for an unknown key token, got nil")
	}
	if strings.Contains(err.Error(), "allow_in_editor") {
		t.Fatalf("an unparseable combo should report the parse failure, not the allow_in_editor rule: %v", err)
	}
}

func TestParse_ReadsAllowInEditorFromYAML(t *testing.T) {
	m, err := Parse([]byte(`
id: kandev-plugin-voice
api_version: 1
version: "0.1.0"
ui:
  bundle: "/ui/bundle.js"
  keybindings:
    - id: toggle-dictation
      default: "mod+shift+m"
      description: "Toggle dictation"
      allow_in_editor: true
    - id: open-settings
      default: "mod+shift+p"
      description: "Open settings"
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if len(m.UI.Keybindings) != 2 {
		t.Fatalf("expected 2 keybindings, got %d", len(m.UI.Keybindings))
	}
	if !m.UI.Keybindings[0].AllowInEditor {
		t.Error("toggle-dictation should have allow_in_editor set")
	}
	if m.UI.Keybindings[1].AllowInEditor {
		t.Error("open-settings should default allow_in_editor to false")
	}
}
