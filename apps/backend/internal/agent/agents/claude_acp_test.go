package agents

import (
	"reflect"
	"strings"
	"testing"
)

func TestClaudeACPCommandsUseUnversionedBridgePackage(t *testing.T) {
	want := []string{"npx", "-y", "@agentclientprotocol/claude-agent-acp"}
	agent := NewClaudeACP()

	if got := agent.BuildCommand(CommandOptions{}).Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("BuildCommand() = %q, want %q", got, want)
	}
	if got := agent.Runtime().Cmd.Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("Runtime().Cmd = %q, want %q", got, want)
	}
	if got := agent.InferenceConfig().Command.Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("InferenceConfig().Command = %q, want %q", got, want)
	}
	if got := agent.InstallScript(); got != "npm install -g @anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp" {
		t.Errorf("InstallScript() = %q, want unversioned Claude ACP package", got)
	}
}

func TestClaudeACPPermissionSettingsSkipPermissions(t *testing.T) {
	settings := NewClaudeACP().PermissionSettings()
	setting, ok := settings[PermissionKeyDangerouslySkipPermissions]
	if !ok {
		t.Fatalf("PermissionSettings() missing key %q", PermissionKeyDangerouslySkipPermissions)
	}
	if !setting.Supported {
		t.Error("dangerously_skip_permissions must be Supported")
	}
	if setting.ApplyMethod != PermissionApplyMethodCLIFlag {
		t.Errorf("ApplyMethod = %q, want %q", setting.ApplyMethod, PermissionApplyMethodCLIFlag)
	}
	if setting.CLIFlag != "--dangerously-skip-permissions" {
		t.Errorf("CLIFlag = %q, want --dangerously-skip-permissions", setting.CLIFlag)
	}
}

// Passthrough launch path drives the flag via PermissionValues. Verify the
// resulting command includes --dangerously-skip-permissions when the toggle is
// on, and excludes it otherwise. Regression coverage for issue #1261.
func TestClaudeACPBuildPassthroughCommandSkipPermissions(t *testing.T) {
	c := NewClaudeACP()

	without := strings.Join(c.BuildPassthroughCommand(PassthroughOptions{}).Args(), " ")
	if strings.Contains(without, "--dangerously-skip-permissions") {
		t.Errorf("default passthrough command must not include --dangerously-skip-permissions, got %q", without)
	}

	with := strings.Join(
		c.BuildPassthroughCommand(PassthroughOptions{
			PermissionValues: map[string]bool{PermissionKeyDangerouslySkipPermissions: true},
		}).Args(),
		" ",
	)
	if !strings.Contains(with, "--dangerously-skip-permissions") {
		t.Errorf("passthrough command with dangerously_skip_permissions=true must include --dangerously-skip-permissions, got %q", with)
	}
}
