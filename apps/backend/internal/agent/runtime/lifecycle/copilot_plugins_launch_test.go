package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

type copilotProfileResolver struct {
	profile *AgentProfileInfo
	err     error
}

func (r copilotProfileResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	return r.profile, r.err
}

type copilotSelectionReader struct {
	version string
	calls   int
}

func (r *copilotSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	r.calls++
	return managedruntime.Selection{Package: "@github/copilot", Version: r.version}, true, nil
}

type copilotPluginRunner struct {
	stdout  []byte
	err     error
	calls   [][]string
	envs    []map[string]string
	dirs    []string
	started chan struct{}
	release chan struct{}
	onRun   func()
	once    sync.Once
}

func (r *copilotPluginRunner) Run(_ context.Context, argv []string, env map[string]string, workDir string) (copilotplugins.CommandResult, error) {
	r.calls = append(r.calls, append([]string{}, argv...))
	r.envs = append(r.envs, cloneStringMap(env))
	r.dirs = append(r.dirs, workDir)
	if r.onRun != nil {
		r.onRun()
	}
	if r.started != nil {
		r.once.Do(func() {
			close(r.started)
			<-r.release
		})
	}
	return copilotplugins.CommandResult{Stdout: r.stdout}, r.err
}

func copilotPluginFixture(t *testing.T) (cacheRoot, configDir string, runner *copilotPluginRunner) {
	t.Helper()
	cacheRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheRoot, "plugin.json"), []byte(`{"name":"spark","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir = t.TempDir()
	encoded := `{"installedPlugins":[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","cache_path":` + quoteJSON(cacheRoot) + `}]}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(encoded), 0o644); err != nil {
		t.Fatal(err)
	}
	runner = &copilotPluginRunner{stdout: []byte(`[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","enabled":true,"source":"installed"}]`)}
	return cacheRoot, configDir, runner
}

func canonicalCopilotPluginRootForTest(t *testing.T, root string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve plugin root %q: %v", root, err)
	}
	return canonical
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

func newCopilotLifecycleManager(t *testing.T, profile *AgentProfileInfo, selection managedruntime.SelectionReader) *Manager {
	t.Helper()
	mgr := newTestManager(t)
	mgr.profileResolver = copilotProfileResolver{profile: profile}
	if selection != nil {
		mgr.managedRuntimeSelections = selection
	}
	return mgr
}

func copilotExecution(t *testing.T, profileID string) *AgentExecution {
	t.Helper()
	return &AgentExecution{
		ID: "exec-copilot", SessionID: "session-1", TaskID: "task-1",
		AgentID: agents.CopilotACPAgentID, AgentProfileID: profileID, ExecutionProfileID: profileID,
		ExecutorType: "local", RuntimeName: agentruntime.RuntimeStandalone, WorkspacePath: t.TempDir(),
	}
}

func copilotLaunchProfile() *AgentProfileInfo {
	return &AgentProfileInfo{
		ProfileID: "profile-1", AgentID: "agent-1", AgentName: agents.CopilotACPAgentID,
		CopilotSelectedPlugins: []string{"installed:copilot-plugins/spark"},
	}
}

func copilotWrapperForTest() launchwrapper.Spec {
	return launchwrapper.Profile([]string{"sandbox", "--"})
}

func copilotBaseArgs() []string {
	return []string{"npx", "--yes", "--prefer-offline", "@github/copilot@1.0.88", "--acp"}
}

func TestCopilotPluginSupportedPreparation(t *testing.T) {
	root, configDir, runner := copilotPluginFixture(t)
	mgr := newCopilotLifecycleManager(t, copilotLaunchProfile(), nil)
	mgr.SetCopilotPluginRunner(runner)
	args, err := mgr.prepareCopilotPluginArgs(
		context.Background(), copilotExecution(t, "profile-1"),
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		copilotBaseArgs(),
	)
	if err != nil {
		t.Fatalf("prepareCopilotPluginArgs: %v", err)
	}
	if !containsSequence(args, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) {
		t.Fatalf("prepared argv = %#v, want a separate plugin directory argument", args)
	}
	if len(runner.calls) != 1 || runner.dirs[0] == "" {
		t.Fatalf("inventory calls=%d workdirs=%#v, want one call in a trusted working directory", len(runner.calls), runner.dirs)
	}
}

func TestCopilotPluginEmptySelectionSkipsInventory(t *testing.T) {
	runner := &copilotPluginRunner{}
	profile := &AgentProfileInfo{ProfileID: "profile-1", AgentName: agents.CopilotACPAgentID}
	mgr := newCopilotLifecycleManager(t, profile, nil)
	mgr.SetCopilotPluginRunner(runner)
	args, err := mgr.prepareCopilotPluginArgs(context.Background(), copilotExecution(t, "profile-1"), map[string]string{}, copilotBaseArgs())
	if err != nil {
		t.Fatalf("prepareCopilotPluginArgs: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("empty selection ran %d inventory commands", len(runner.calls))
	}
	if strings.Join(args, "\x00") != strings.Join(copilotBaseArgs(), "\x00") {
		t.Fatalf("empty selection changed argv: %#v", args)
	}
}

func TestCopilotPluginExecutorAdmission(t *testing.T) {
	_, configDir, runner := copilotPluginFixture(t)
	mgr := newCopilotLifecycleManager(t, copilotLaunchProfile(), nil)
	mgr.SetCopilotPluginRunner(runner)
	execution := copilotExecution(t, "profile-1")
	execution.ExecutorType = "local_docker"
	_, err := mgr.prepareCopilotPluginArgs(
		context.Background(), execution,
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}, copilotBaseArgs(),
	)
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
		t.Fatalf("executor error = %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("isolated executor ran %d host inventory commands", len(runner.calls))
	}

	unknown := copilotExecution(t, "profile-1")
	unknown.ExecutorType = ""
	unknown.RuntimeName = "docker"
	_, err = mgr.prepareCopilotPluginArgs(
		context.Background(), unknown,
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}, copilotBaseArgs(),
	)
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
		t.Fatalf("unknown executor error = %#v", err)
	}
}

func TestCopilotPluginWrapperRejected(t *testing.T) {
	_, configDir, runner := copilotPluginFixture(t)
	mgr := newCopilotLifecycleManager(t, copilotLaunchProfile(), nil)
	mgr.SetCopilotPluginRunner(runner)
	execution := copilotExecution(t, "profile-1")
	execution.LaunchWrapper = copilotWrapperForTest()
	_, err := mgr.prepareCopilotPluginArgs(
		context.Background(), execution,
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}, copilotBaseArgs(),
	)
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
		t.Fatalf("wrapper error = %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("wrapped launch ran %d native inventory commands", len(runner.calls))
	}
}

func TestCopilotPluginRuntimeSnapshotNotReread(t *testing.T) {
	root, configDir, runner := copilotPluginFixture(t)
	selection := &copilotSelectionReader{version: "1.0.104"}
	mgr := newCopilotLifecycleManager(t, copilotLaunchProfile(), selection)
	mgr.SetCopilotPluginRunner(runner)
	args, err := mgr.prepareCopilotPluginArgs(
		context.Background(), copilotExecution(t, "profile-1"),
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")}, copilotBaseArgs(),
	)
	if err != nil {
		t.Fatalf("prepare supported runtime snapshot: %v", err)
	}
	if !containsSequence(args, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) {
		t.Fatalf("prepared args = %#v, want selected plugin mount", args)
	}
	if len(runner.calls) != 1 || !containsSequence(runner.calls[0], "@github/copilot@1.0.88", "plugin", "list") {
		t.Fatalf("inventory command = %#v, want the version captured in launch argv", runner.calls)
	}
	if selection.calls != 0 {
		t.Fatalf("plugin preparation reread the active runtime %d times after argv construction", selection.calls)
	}
}

func TestCopilotPluginSelectionUsesExecutionSnapshotAndRefreshesLegacy(t *testing.T) {
	profile := copilotLaunchProfile()
	mgr := newCopilotLifecycleManager(t, profile, nil)
	execution := copilotExecution(t, "profile-1")
	execution.CopilotSelectedPlugins = []string{"installed:stale/stale"}

	got, err := mgr.copilotPluginSelection(context.Background(), execution)
	if err != nil {
		t.Fatalf("copilotPluginSelection: %v", err)
	}
	if len(got) != 1 || got[0] != "installed:stale/stale" {
		t.Fatalf("selection = %#v, want the captured execution snapshot", got)
	}
	if execution.CopilotSelectedPlugins[0] != got[0] {
		t.Fatalf("execution snapshot = %#v", execution.CopilotSelectedPlugins)
	}

	execution.CopilotSelectedPlugins = nil
	resolved, err := mgr.copilotPluginSelection(context.Background(), execution)
	if err != nil {
		t.Fatalf("copilotPluginSelection for legacy execution: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "installed:copilot-plugins/spark" {
		t.Fatalf("legacy selection = %#v, want the current profile snapshot", resolved)
	}

	profile.CopilotSelectedPlugins = nil
	execution.CopilotSelectedPlugins = nil
	cleared, err := mgr.copilotPluginSelection(context.Background(), execution)
	if err != nil {
		t.Fatalf("copilotPluginSelection after profile clear: %v", err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared selection = %#v", cleared)
	}
}
