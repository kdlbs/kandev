package agents

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestClaudeACPUsesManagedRuntime(t *testing.T) {
	a := NewClaudeACP()
	want := a.ManagedNPMRuntime().CachedACPCommand().Args()

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	wantInstall := "npm install -g @anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp"
	if got := a.InstallScript(); got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

// TestClaudeACPSeparatesMCPStartupAndToolBudgets guards against
// anthropics/claude-code#91414: MCP_TIMEOUT governs both the MCP connect
// deadline and the first-turn prewait, so a large value here blocks the
// first token for MCP_TIMEOUT-5000ms. The startup budget must stay short;
// the long budget for blocking tool calls belongs on MCP_TOOL_TIMEOUT.
func TestClaudeACPSeparatesMCPStartupAndToolBudgets(t *testing.T) {
	env := NewClaudeACP().Runtime().Env

	const wantStartupBudgetMS = 30000
	const maxStartupBudgetMS = 60000
	startup, ok := env["MCP_TIMEOUT"]
	if !ok {
		t.Fatal(`Runtime().Env missing "MCP_TIMEOUT"`)
	}
	startupMS, err := strconv.Atoi(startup)
	if err != nil {
		t.Fatalf("MCP_TIMEOUT = %q, want an integer", startup)
	}
	if startupMS != wantStartupBudgetMS {
		t.Errorf("MCP_TIMEOUT = %d, want %d", startupMS, wantStartupBudgetMS)
	}
	if startupMS > maxStartupBudgetMS {
		t.Errorf("MCP_TIMEOUT = %d, want <= %d (startup budget, not a tool-call budget)", startupMS, maxStartupBudgetMS)
	}

	toolTimeout, ok := env["MCP_TOOL_TIMEOUT"]
	if !ok {
		t.Fatal(`Runtime().Env missing "MCP_TOOL_TIMEOUT"`)
	}
	if toolTimeout != "7200000" {
		t.Errorf("MCP_TOOL_TIMEOUT = %q, want %q", toolTimeout, "7200000")
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
