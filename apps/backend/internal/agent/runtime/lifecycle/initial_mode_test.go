package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func claudeACPAgent(t *testing.T) agents.Agent {
	t.Helper()
	return agents.NewClaudeACP()
}

func initialModeManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{dataDir: t.TempDir(), logger: newTestLogger()}
}

func deliveredSettings(t *testing.T, configDir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if err != nil {
		t.Fatalf("read delivered settings: %v", err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode delivered settings: %v", err)
	}
	return decoded
}

func deliveredSettingsMode(t *testing.T, configDir string) string {
	t.Helper()
	decoded := deliveredSettings(t, configDir)
	permissions, ok := decoded["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("delivered settings carry no permissions object: %+v", decoded)
	}
	mode, _ := permissions["defaultMode"].(string)
	return mode
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7, .10
func TestApplyInitialModeConfiguresLaunchedProcess(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "local_docker")

	if outcome.Delivered || outcome.Request == nil {
		t.Fatalf("outcome = %+v, want a pending executor installation", outcome)
	}
	configDir := env["CLAUDE_CONFIG_DIR"]
	if configDir == "" {
		t.Fatal("CLAUDE_CONFIG_DIR was not exported to the launch environment")
	}
	if configDir != "/root/.claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want the container session path", configDir)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.12
func TestApplyInitialModeDoesNotCopyUnselectedHostConfiguration(t *testing.T) {
	m := initialModeManager(t)
	hostHome := t.TempDir()
	hostConfigDir := filepath.Join(hostHome, ".claude")
	if err := os.MkdirAll(hostConfigDir, 0o700); err != nil {
		t.Fatalf("create host config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostConfigDir, "settings.json"), []byte(`{"env":{"UNSELECTED_SENTINEL":"private"},"hooks":{"PreToolUse":[{"command":"secret-hook"}]}}`), 0o600); err != nil {
		t.Fatalf("seed host settings: %v", err)
	}
	t.Setenv("HOME", hostHome)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "local_docker")
	if outcome.Delivered || outcome.Request == nil {
		t.Fatalf("outcome = %+v, want a pending executor installation", outcome)
	}
	settingsDir := SessionDirHostPath(m.dataDir, "exec-1", claudeACPAgent(t).Runtime().SessionConfig.SessionDirTemplate)
	executor := &DockerExecutor{kandevHomeDir: m.dataDir, logger: newTestLogger()}
	req := &ExecutorCreateRequest{
		InstanceID: "exec-1", AgentConfig: claudeACPAgent(t), InitialMode: outcome.Request,
	}
	if err := executor.seedSessionDir(context.Background(), req); err != nil {
		t.Fatalf("seed session directory: %v", err)
	}
	settings := deliveredSettings(t, settingsDir)
	if _, ok := settings["env"]; ok {
		t.Fatalf("unselected host environment reached session settings: %+v", settings["env"])
	}
	if _, ok := settings["hooks"]; ok {
		t.Fatalf("unselected host hooks reached session settings: %+v", settings["hooks"])
	}
}

// A standalone host run can rely on authentication in its default config
// directory, so Kandev leaves that path and its credentials in place.
func TestApplyInitialModeLeavesHostAuthenticationDirectoryUntouched(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{"ANTHROPIC_API_KEY": "test-key"}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "worktree")

	if outcome.Delivered || outcome.Reason == "" {
		t.Fatalf("outcome = %+v, want an unavailable result with a reason", outcome)
	}
	if len(env) != 1 || env["ANTHROPIC_API_KEY"] != "test-key" {
		t.Fatalf("environment = %+v, want unchanged", env)
	}
}

// A profile that requests no mode must leave the launch environment untouched,
// which bounds this feature to sessions that actually ask for a mode.
func TestApplyInitialModeIsInertWithoutRequestedMode(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{"EXISTING": "value"}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "", "worktree")

	if outcome.Mode != "" || outcome.Delivered || outcome.Request != nil {
		t.Fatalf("outcome = %+v, want an inert result", outcome)
	}
	if len(env) != 1 || env["EXISTING"] != "value" {
		t.Fatalf("env = %+v, want it unchanged", env)
	}
}

func TestReportInitialModeWarningIncludesRequestedModeAndReason(t *testing.T) {
	var reported []PrepareStep
	reportInitialModeWarning := PrepareProgressCallback(func(step PrepareStep, _, _ int) {
		reported = append(reported, step)
	})
	(&Manager{}).reportInitialModeWarning(reportInitialModeWarning, "task-1", "session-1", "bypassPermissions", "SSH upload failed")

	if len(reported) != 1 {
		t.Fatalf("reported steps = %d, want one warning", len(reported))
	}
	step := reported[0]
	if step.Warning == "" || !strings.Contains(step.WarningDetail, "bypassPermissions") || !strings.Contains(step.WarningDetail, "SSH upload failed") {
		t.Fatalf("warning step = %+v, want the requested mode and delivery reason", step)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.9, .11
// A container runs the agent as root, where the bundled bridge disables the
// permissive mode unless the sandbox is declared.
func TestApplyInitialModeDeclaresSandboxForContainerExecutors(t *testing.T) {
	for executorType, wantSandbox := range map[string]bool{
		"local_docker": true,
		"k8s":          true,
		"worktree":     false,
		"local_pc":     false,
	} {
		t.Run(executorType, func(t *testing.T) {
			m := initialModeManager(t)
			env := map[string]string{}

			m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", executorType)

			_, present := env["IS_SANDBOX"]
			if present != wantSandbox {
				t.Fatalf("IS_SANDBOX present = %v, want %v for executor %q", present, wantSandbox, executorType)
			}
		})
	}
}

// An agent without a declared channel keeps the post-creation switch and says
// so, rather than reporting a delivery that did not happen.
func TestApplyInitialModeReportsMissingChannel(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", &agentWithoutInitialMode{}, "bypassPermissions", "worktree")

	if outcome.Delivered {
		t.Fatalf("outcome = %+v, want a non-delivered result", outcome)
	}
	if outcome.Reason == "" {
		t.Fatal("a non-delivered mode must carry a reason")
	}
	if len(env) != 0 {
		t.Fatalf("env = %+v, want it untouched for an agent without a channel", env)
	}
}

// A mode the agent's channel cannot express is not delivered silently.
func TestApplyInitialModeRejectsUnknownMode(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "totally-unknown", "worktree")

	if outcome.Delivered {
		t.Fatalf("outcome = %+v, want a non-delivered result", outcome)
	}
	if _, present := env["CLAUDE_CONFIG_DIR"]; present {
		t.Fatal("an undeliverable mode must not redirect the configuration directory")
	}
}

type agentWithoutInitialMode struct {
	agents.Agent
}

func (a *agentWithoutInitialMode) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{}
}

// The container reads this directory through the session bind mount, so the
// host path names nothing it can open. Exporting it left the agent without the
// mode and without the mounted session state.
func TestApplyInitialModeGivesContainersTheMountedPath(t *testing.T) {
	m := initialModeManager(t)
	env := map[string]string{}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "local_docker")

	if outcome.Delivered || outcome.Request == nil {
		t.Fatalf("outcome = %+v, want a pending executor installation", outcome)
	}
	if got := env["CLAUDE_CONFIG_DIR"]; got != "/root/.claude" {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want the container session directory", got)
	}
}

// The host launch keeps reading the materialized directory directly.
func TestApplyInitialModePreservesExplicitConfigurationDirectory(t *testing.T) {
	m := initialModeManager(t)
	expectedConfigDir := filepath.Join(t.TempDir(), "custom-claude-home")
	env := map[string]string{"CLAUDE_CONFIG_DIR": expectedConfigDir}

	outcome := m.applyInitialMode(env, "exec-1", claudeACPAgent(t), "bypassPermissions", "local_docker")

	if outcome.Delivered || outcome.Reason == "" || outcome.Request != nil {
		t.Fatalf("outcome = %+v, want an unavailable result with a reason", outcome)
	}
	if got := env["CLAUDE_CONFIG_DIR"]; got != expectedConfigDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want explicit path %q", got, expectedConfigDir)
	}
}
