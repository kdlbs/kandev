package agents

import (
	"strings"
	"testing"
)

func TestCursorACPRemoteAuth(t *testing.T) {
	auth := NewCursorACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected env-var auth method")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "env" {
		t.Errorf("Type = %q, want %q", m.Type, "env")
	}
	if m.EnvVar != "CURSOR_API_KEY" {
		t.Errorf("EnvVar = %q, want %q", m.EnvVar, "CURSOR_API_KEY")
	}
}

func TestCursorACPInstallScriptIsNativeInstaller(t *testing.T) {
	script := NewCursorACP().InstallScript()
	if !strings.Contains(script, "cursor.com/install") {
		t.Errorf("InstallScript must reference the cursor.com installer, got: %q", script)
	}
	// Ensure ~/.local/bin gets onto PATH so the cursor-agent binary is
	// discoverable for subsequent prepare-script steps and shells.
	if !strings.Contains(script, "$HOME/.local/bin") {
		t.Errorf("InstallScript must add $HOME/.local/bin to PATH, got: %q", script)
	}
}
