package initialmode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func claudeRequest(target string) Request {
	return Request{
		TargetDir:        target,
		SettingsFileName: "settings.json",
		ModeKeyPath:      []string{"permissions", "defaultMode"},
		ModeValue:        "bypassPermissions",
	}
}

func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return decoded
}

func modeOf(t *testing.T, settings map[string]any) string {
	t.Helper()
	permissions, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("settings carry no permissions object: %+v", settings)
	}
	mode, _ := permissions["defaultMode"].(string)
	return mode
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
func TestMaterializeWritesModeIntoSettings(t *testing.T) {
	target := filepath.Join(t.TempDir(), "session")

	dir, err := Materialize(claudeRequest(target))
	if err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	if dir != target {
		t.Fatalf("dir = %q, want %q", dir, target)
	}
	if got := modeOf(t, readSettings(t, target)); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// The executor passes only settings from a bundle selected for this session.
func TestMaterializeMergesExplicitSelectedSettings(t *testing.T) {
	target := filepath.Join(t.TempDir(), "session")
	req := claudeRequest(target)
	req.SettingsJSON = []byte(`{"model":"opus","permissions":{"allow":["Bash(git status:*)"]},"env":{"SELECTED":"value"}}`)

	if _, err := Materialize(req); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	settings := readSettings(t, target)
	if settings["model"] != "opus" || settings["env"].(map[string]any)["SELECTED"] != "value" {
		t.Fatalf("selected settings were not preserved: %+v", settings)
	}
	permissions := settings["permissions"].(map[string]any)
	allow, ok := permissions["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(git status:*)" {
		t.Fatalf("allow list not preserved: %+v", permissions["allow"])
	}
	if got := modeOf(t, settings); got != "bypassPermissions" {
		t.Fatalf("defaultMode = %q, want bypassPermissions", got)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.15
func TestMaterializeRejectsNonObjectSelectedSettings(t *testing.T) {
	for name, raw := range map[string]string{
		"null":      "null",
		"scalar":    `"settings"`,
		"array":     `[]`,
		"malformed": `{not json`,
	} {
		t.Run(name, func(t *testing.T) {
			req := claudeRequest(filepath.Join(t.TempDir(), "session"))
			req.SettingsJSON = []byte(raw)
			if _, err := Materialize(req); err == nil {
				t.Fatal("Materialize accepted a non-object settings root; want a controlled error")
			}
		})
	}
}

func TestMaterializeAcceptsEmptyObjectAndMissingSelection(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty object": []byte(`{}`),
		"no selection": nil,
	} {
		t.Run(name, func(t *testing.T) {
			req := claudeRequest(filepath.Join(t.TempDir(), "session"))
			req.SettingsJSON = raw
			if _, err := Materialize(req); err != nil {
				t.Fatalf("Materialize returned error: %v", err)
			}
			if got := modeOf(t, readSettings(t, req.TargetDir)); got != "bypassPermissions" {
				t.Fatalf("defaultMode = %q, want bypassPermissions", got)
			}
		})
	}
}

func TestMaterializeWritesSettingsWithPrivatePermissions(t *testing.T) {
	target := filepath.Join(t.TempDir(), "session")
	if _, err := Materialize(claudeRequest(target)); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	info, err := os.Stat(filepath.Join(target, "settings.json"))
	if err != nil {
		t.Fatalf("stat settings: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("settings permissions = %#o, want %#o", got, 0o600)
	}
}

func TestMaterializeRejectsIncompleteRequest(t *testing.T) {
	if _, err := Materialize(Request{TargetDir: t.TempDir()}); err == nil {
		t.Fatal("expected an error for a request without a settings file or mode key path")
	}
	if _, err := Materialize(Request{SettingsFileName: "settings.json", ModeKeyPath: []string{"a"}}); err == nil {
		t.Fatal("expected an error for a request without a target directory")
	}
}
