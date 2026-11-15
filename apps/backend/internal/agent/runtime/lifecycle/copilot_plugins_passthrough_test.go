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

func TestCopilotPluginSupportedPassthroughPreparation(t *testing.T) {
	root, configDir, runner := copilotPluginFixture(t)
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
	execution := copilotExecution(t, "profile-1")
	err = mgr.applyPassthroughCopilotPlugins(context.Background(), execution, profile, &cmd, env)
	if err != nil {
		t.Fatalf("applyPassthroughCopilotPlugins: %v", err)
	}
	if !containsSequence(cmd.Args(), "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) {
		t.Fatalf("passthrough argv = %#v, want a separate plugin directory argument", cmd.Args())
	}
	if len(runner.calls) != 1 || runner.dirs[0] != execution.WorkspacePath {
		t.Fatalf("inventory calls=%d workdirs=%#v, want one call in %q", len(runner.calls), runner.dirs, execution.WorkspacePath)
	}
	if !containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") {
		t.Fatalf("inventory argv = %#v, want selected managed runtime", runner.calls[0])
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
	execution := copilotExecution(t, "profile-1")
	cmd := passthroughCommand{Command: base, wrapper: copilotWrapperForTest()}
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
