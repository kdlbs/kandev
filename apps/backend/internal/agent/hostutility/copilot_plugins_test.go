package hostutility

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type copilotUtilitySelection struct {
	version string
	calls   int
}

func (s *copilotUtilitySelection) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	s.calls++
	return managedruntime.Selection{Package: "@github/copilot", Version: s.version}, true, nil
}

type copilotUtilityRunner struct {
	stdout []byte
	calls  int
}

func (r *copilotUtilityRunner) Run(context.Context, []string, map[string]string, string) (copilotplugins.CommandResult, error) {
	r.calls++
	return copilotplugins.CommandResult{Stdout: r.stdout}, nil
}

func copilotUtilityFixture(t *testing.T) (configDir string, runner *copilotUtilityRunner) {
	t.Helper()
	cacheRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheRoot, "plugin.json"), []byte(`{"name":"spark","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"installedPlugins":[{"name":"spark","marketplace":"m","version":"1.0.0","cache_path":"`+cacheRoot+`"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return configDir, &copilotUtilityRunner{
		stdout: []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"installed"}]`),
	}
}

func utilityRuntimeCommand() []string {
	return agents.NewCopilotACP().ManagedNPMRuntime().InteractiveCommand("1.0.88").Args()
}

func TestCopilotPluginProfileUtility(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	_, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPolicyUnverified {
		t.Fatalf("policy error = %#v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("policy-unqualified utility ran %d inventory commands", runner.calls)
	}
}

func TestCopilotPluginProfileProbe(t *testing.T) {
	// A probe without a selection never runs inventory; an agent-wide probe
	// therefore cannot inherit a concrete profile's plugin mounts.
	runner := &copilotUtilityRunner{}
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	flags, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env: map[string]string{"COPILOT_HOME": t.TempDir()},
	}, "", utilityRuntimeCommand())
	if err != nil || len(flags) != 0 {
		t.Fatalf("empty selection flags = %#v, err = %v", flags, err)
	}
	if runner.calls != 0 {
		t.Fatalf("empty selection ran %d inventory commands", runner.calls)
	}
	flags, err = mgr.profileCopilotPluginFlags(context.Background(), "claude-acp", ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": t.TempDir()},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	if err != nil || len(flags) != 0 {
		t.Fatalf("foreign provider flags = %#v, err = %v", flags, err)
	}
}

func TestCopilotPluginAgentWideProbe(t *testing.T) {
	_, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	_, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": t.TempDir(), "PATH": os.Getenv("PATH")},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPolicyUnverified {
		t.Fatalf("policy error = %#v", err)
	}
}

func TestCopilotPluginProfileUtilityManualFlags(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	_, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CLIFlags:               []string{"--model", "auto"},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPolicyUnverified {
		t.Fatalf("policy error = %#v", err)
	}
}

func TestCopilotPluginUtilityRejectsWrapper(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	_, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CommandPrefix:          []string{"sandbox", "--"},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
		t.Fatalf("wrapper error = %#v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("wrapped utility ran %d inventory commands", runner.calls)
	}
}

func TestCopilotPluginUtilityDoesNotReadRuntimeSelection(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	selection := &copilotUtilitySelection{version: "1.0.104"}
	mgr := &Manager{managedRuntimeSelections: selection}
	mgr.SetCopilotPluginRunner(runner)
	_, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityRuntimeCommand())
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPolicyUnverified {
		t.Fatalf("policy error = %#v", err)
	}
	if selection.calls != 0 {
		t.Fatalf("utility resolved plugins through %d later runtime reads", selection.calls)
	}
}
