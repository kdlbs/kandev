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

func TestCursorACPPermissionSettingsCursorForce(t *testing.T) {
	settings := NewCursorACP().PermissionSettings()
	setting, ok := settings[PermissionKeyCursorForce]
	if !ok {
		t.Fatal("PermissionSettings() missing cursor_force")
	}
	if _, hasAuto := settings[PermissionKeyAutoApprove]; hasAuto {
		t.Fatal("PermissionSettings() must not include auto_approve; that is agentctl-only in the catalog")
	}
	if !setting.Supported {
		t.Error("cursor_force must be Supported")
	}
	if setting.ApplyMethod != PermissionApplyMethodCLIFlag {
		t.Errorf("ApplyMethod = %q, want %q", setting.ApplyMethod, PermissionApplyMethodCLIFlag)
	}
	if setting.CLIFlag != "--force" {
		t.Errorf("CLIFlag = %q, want --force", setting.CLIFlag)
	}
}

func TestCursorACPCatalogSeparatesAgentctlAutoApprove(t *testing.T) {
	catalog := CatalogPermissionSettings(NewCursorACP())
	auto, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if auto.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Fatalf("auto_approve ApplyMethod = %q, want %q", auto.ApplyMethod, PermissionApplyMethodAgentctlAutoApprove)
	}
	force, ok := catalog[PermissionKeyCursorForce]
	if !ok {
		t.Fatal("catalog missing cursor_force")
	}
	if force.ApplyMethod != PermissionApplyMethodCLIFlag {
		t.Fatalf("cursor_force ApplyMethod = %q, want cli_flag", force.ApplyMethod)
	}
}

func TestCursorACPBuildCommand(t *testing.T) {
	c := NewCursorACP()

	plain := strings.Join(c.BuildCommand(CommandOptions{}).Args(), " ")
	if plain != "cursor-agent acp" {
		t.Fatalf("default BuildCommand = %q, want %q", plain, "cursor-agent acp")
	}
	if strings.Contains(plain, "--force") {
		t.Error("default command must not include --force")
	}

	withAutoApprove := strings.Join(
		c.BuildCommand(CommandOptions{PermissionValues: map[string]bool{PermissionKeyAutoApprove: true}}).Args(),
		" ",
	)
	if strings.Contains(withAutoApprove, "--force") {
		t.Errorf("auto_approve PermissionValues must not add --force, got %q", withAutoApprove)
	}
}

func TestCursorACPInstallScriptIsNativeInstaller(t *testing.T) {
	script := NewCursorACP().InstallScript()
	if !strings.Contains(script, "cursor.com/install") {
		t.Errorf("InstallScript must reference the cursor.com installer, got: %q", script)
	}
	if !strings.Contains(script, "$HOME/.local/bin") {
		t.Errorf("InstallScript must add $HOME/.local/bin to PATH, got: %q", script)
	}
}
