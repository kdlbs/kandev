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

func TestCursorACPPermissionSettingsAutoApprove(t *testing.T) {
	settings := NewCursorACP().PermissionSettings()
	setting, ok := settings[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("PermissionSettings() missing auto_approve; cursor must expose an approval-bypass toggle")
	}
	if !setting.Supported {
		t.Error("auto_approve must be Supported")
	}
	if setting.ApplyMethod != PermissionApplyMethodCLIFlag {
		t.Errorf("ApplyMethod = %q, want %q", setting.ApplyMethod, PermissionApplyMethodCLIFlag)
	}
	if setting.CLIFlag != "--force" {
		t.Errorf("CLIFlag = %q, want --force (the only flag that bypasses Cursor's ACP allowlist)", setting.CLIFlag)
	}
}

func TestCursorACPBuildCommandAutoApprove(t *testing.T) {
	c := NewCursorACP()

	// Default: a bare `cursor-agent acp`, so Cursor keeps prompting per command.
	plain := strings.Join(c.BuildCommand(CommandOptions{}).Args(), " ")
	if plain != "cursor-agent acp" {
		t.Fatalf("default BuildCommand = %q, want %q", plain, "cursor-agent acp")
	}
	if strings.Contains(plain, "--force") {
		t.Error("default command must not include --force")
	}

	// auto_approve enabled: --force is appended so Cursor stops sending a
	// session/request_permission for every non-allowlisted command.
	approved := strings.Join(
		c.BuildCommand(CommandOptions{PermissionValues: map[string]bool{PermissionKeyAutoApprove: true}}).Args(),
		" ",
	)
	if !strings.Contains(approved, "--force") {
		t.Errorf("auto_approve BuildCommand = %q, want it to contain --force", approved)
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
