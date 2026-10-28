package lifecycle

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
)

func TestCopilotPluginPassthrough(t *testing.T) {
	_, configDir, runner := copilotPluginFixture(t)
	profile := copilotLaunchProfile()
	mgr := newCopilotLifecycleManager(t, profile, &copilotSelectionReader{version: "1.0.104"})
	mgr.SetCopilotPluginRunner(runner)
	agentConfig := agents.NewCopilotACP()

	base, err := mgr.copilotPassthroughBaseCommand(context.Background(), agentConfig, profile)
	if err != nil {
		t.Fatalf("copilotPassthroughBaseCommand: %v", err)
	}
	if !strings.Contains(strings.Join(base.Args(), " "), "@github/copilot@1.0.104") {
		t.Fatalf("base command = %#v, want selected managed runtime", base.Args())
	}

	empty, err := mgr.copilotPassthroughBaseCommand(context.Background(), agentConfig, &AgentProfileInfo{})
	if err != nil {
		t.Fatalf("empty base command: %v", err)
	}
	if !empty.IsEmpty() {
		t.Fatalf("empty selection changed the passthrough base: %#v", empty.Args())
	}

	cmd := passthroughCommand{Command: base.With().Flag("--continue").Build()}
	env := map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}
	err = mgr.applyPassthroughCopilotPlugins(context.Background(), copilotExecution("profile-1"), profile, &cmd, env)
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPolicyUnverified {
		t.Fatalf("passthrough policy error = %#v", err)
	}
	if strings.Contains(strings.Join(cmd.Args(), " "), "--plugin-dir") {
		t.Fatalf("policy-unqualified passthrough mounted plugins: %#v", cmd.Args())
	}
}

func TestCopilotPluginPassthroughRejectsWrapper(t *testing.T) {
	_, configDir, runner := copilotPluginFixture(t)
	profile := copilotLaunchProfile()
	mgr := newCopilotLifecycleManager(t, profile, &copilotSelectionReader{version: "1.0.88"})
	mgr.SetCopilotPluginRunner(runner)
	base, err := mgr.copilotPassthroughBaseCommand(context.Background(), agents.NewCopilotACP(), profile)
	if err != nil {
		t.Fatalf("copilotPassthroughBaseCommand: %v", err)
	}
	execution := copilotExecution("profile-1")
	execution.LaunchWrapper = copilotWrapperForTest()
	cmd := passthroughCommand{Command: base}
	err = mgr.applyPassthroughCopilotPlugins(context.Background(), execution, profile, &cmd,
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")})
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
		t.Fatalf("terminal wrapper error = %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("wrapped terminal ran %d native inventory commands", len(runner.calls))
	}
}
