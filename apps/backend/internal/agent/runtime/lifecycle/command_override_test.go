package lifecycle

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
)

func TestBuildAgentCommandCommandOverride(t *testing.T) {
	const path = "/opt/pinned/bin/codex"
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	runtimes := []agentruntime.Runtime{
		agentruntime.RuntimeStandalone, agentruntime.RuntimePluginRemote, agentruntime.RuntimeSSH,
		agentruntime.RuntimeDocker,
	}
	for _, runtime := range runtimes {
		t.Run(string(runtime), func(t *testing.T) {
			profile := &AgentProfileInfo{ProfileID: "p1", CommandOverride: path,
				CLIFlags: []settingsmodels.CLIFlag{{Flag: "-c key=value", Enabled: true}}}
			cmds, err := manager.buildAgentCommandWithContext(context.Background(), &LaunchRequest{},
				profile, agents.NewCodexAppServer(true), runtime, false)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			want := []string{path, "app-server", "-c", "key=value"}
			if !slices.Equal(cmds.args, want) {
				t.Fatalf("args = %q, want %q", cmds.args, want)
			}
			if strings.Contains(cmds.initial, "npx") || strings.Contains(cmds.initial, "@openai/codex") {
				t.Fatalf("command %q still uses the npx launch", cmds.initial)
			}
		})
	}
}

func TestBuildAgentCommandWithoutOverrideKeepsNpx(t *testing.T) {
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	for _, profile := range []*AgentProfileInfo{nil, {ProfileID: "p1"}} {
		cmds, err := manager.buildAgentCommandWithContext(context.Background(), &LaunchRequest{},
			profile, agents.NewCodexAppServer(true), agentruntime.RuntimePluginRemote, false)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if len(cmds.args) < 2 || cmds.args[0] != "npx" || !strings.Contains(cmds.initial, "@openai/codex@") {
			t.Fatalf("default args = %q, want the unchanged npx launch", cmds.args)
		}
	}
}

func TestBuildAgentCommandRejectsUnusableOverride(t *testing.T) {
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	tests := []struct {
		name  string
		agent agents.Agent
		value string
	}{
		{name: "relative", agent: agents.NewCodexAppServer(true), value: "codex"},
		{name: "shell string", agent: agents.NewCodexAppServer(true), value: "/opt/codex && id"},
		{name: "agent without managed npm runtime", agent: &cliFlagTestAgent{}, value: "/opt/pinned/bin/tool"},
		{name: "opencode", agent: agents.NewOpenCodeACP(), value: "/opt/pinned/bin/opencode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &AgentProfileInfo{ProfileID: "p1", CommandOverride: tt.value}
			_, err := manager.buildAgentCommandWithContext(context.Background(), &LaunchRequest{},
				profile, tt.agent, agentruntime.RuntimePluginRemote, false)
			if err == nil {
				t.Fatal("launch accepted an unusable override instead of failing closed")
			}
			if got := validCommandOverride(profile, tt.agent); got != "" {
				t.Fatalf("preflight override = %q, want empty", got)
			}
		})
	}
}

func TestCommandOverrideBeatsNativeBinaryPreference(t *testing.T) {
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	profile := &AgentProfileInfo{ProfileID: "p1", CommandOverride: "/opt/pinned/bin/copilot"}
	cmds, err := manager.buildAgentCommandWithContext(context.Background(), &LaunchRequest{},
		profile, agents.NewCopilotACP(), agentruntime.RuntimeStandalone, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cmds.args[0] != "/opt/pinned/bin/copilot" {
		t.Fatalf("args = %q, want the override first", cmds.args)
	}
}

// The override changes only argv. The launch environment is the existing
// profile env_vars path, so CODEX_HOME and friends reach the agent unchanged.
func TestProfileEnvFlowsAlongsideCommandOverride(t *testing.T) {
	manager := &Manager{logger: newTestLogger()}
	info := &AgentProfileInfo{ProfileID: "p1", CommandOverride: "/opt/pinned/bin/codex",
		EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "CODEX_HOME", Value: "/work/codex-home"},
			{Key: "OPENAI_API_KEY", Value: "from-profile"},
		}}
	env := map[string]string{"OPENAI_API_KEY": "already-set"}
	if err := manager.mergeAgentProfileEnvFromInfoWithPartial(context.Background(), info, env, false); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if env["CODEX_HOME"] != "/work/codex-home" || env["OPENAI_API_KEY"] != "already-set" {
		t.Fatalf("env = %v, want CODEX_HOME merged and existing keys kept", env)
	}
	plain := pluginPlainProfileEnv(info.EnvVars)
	if plain["CODEX_HOME"] != "/work/codex-home" {
		t.Fatalf("plugin agent_env = %v, want CODEX_HOME forwarded", plain)
	}
}
