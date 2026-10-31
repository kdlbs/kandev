package hostutility

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
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
	stdout   []byte
	root     string
	calls    int
	argv     []string
	env      map[string]string
	workDir  string
	argvs    [][]string
	envs     []map[string]string
	workDirs []string
}

func (r *copilotUtilityRunner) Run(_ context.Context, argv []string, env map[string]string, workDir string) (copilotplugins.CommandResult, error) {
	r.calls++
	r.argv = append([]string(nil), argv...)
	r.env = cloneStringMap(env)
	r.workDir = workDir
	r.argvs = append(r.argvs, append([]string(nil), argv...))
	r.envs = append(r.envs, cloneStringMap(env))
	r.workDirs = append(r.workDirs, workDir)
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
		root:   cacheRoot,
	}
}

func canonicalCopilotPluginRootForTest(t *testing.T, root string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve plugin root %q: %v", root, err)
	}
	return canonical
}

func utilityInferenceCommand(version string) []string {
	return agents.NewCopilotACP().ManagedNPMRuntime().ACPCommand(version).Args()
}

func TestCopilotPluginProfileUtility(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	workDir := t.TempDir()
	flags, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, workDir, utilityInferenceCommand("1.0.104"))
	if err != nil {
		t.Fatalf("profileCopilotPluginFlags: %v", err)
	}
	if !containsSequence(flags, "--plugin-dir", canonicalCopilotPluginRootForTest(t, runner.root)) {
		t.Fatalf("utility flags = %#v, want selected plugin mount", flags)
	}
	assertCopilotUtilityInventory(t, runner, 0, "1.0.104", workDir, configDir)
}

func TestCopilotPluginExecuteProfilePromptUsesSelectedInventory(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	agentConfig := agents.NewCopilotACP()
	if err := reg.Register(agentConfig); err != nil {
		t.Fatal(err)
	}

	var received agentctlutil.PromptRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"capabilities":["launch-wrapper.v1"]}`))
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/prompt":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decode profile prompt request: %v", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(agentctlutil.PromptResponse{Success: true, Response: "ok"}); err != nil {
				t.Errorf("encode profile prompt response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	workDir := t.TempDir()
	manager := NewManager(reg, host, port, nil, log)
	manager.SetProfileResolver(profilePromptResolver{profile: &settingsmodels.AgentProfile{
		ID: "profile-1", AgentID: agents.CopilotACPAgentID, Model: "model-a",
		EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "COPILOT_HOME", Value: configDir},
			{Key: "PATH", Value: os.Getenv("PATH")},
		},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}})
	manager.SetManagedRuntimeSelectionStore(&copilotUtilitySelection{version: "1.0.104"})
	manager.SetCopilotPluginRunner(runner)
	manager.instances[agents.CopilotACPAgentID] = &instance{
		agentType: agents.CopilotACPAgentID, workDir: workDir,
		client: agentctlclient.NewClient(host, port, log),
	}

	result, err := manager.ExecuteProfilePrompt(context.Background(), "profile-1", "enhance this prompt")
	if err != nil {
		t.Fatalf("ExecuteProfilePrompt: %v", err)
	}
	if result == nil || result.Response != "ok" {
		t.Fatalf("profile prompt result = %#v, want successful response", result)
	}
	if received.InferenceConfig == nil {
		t.Fatal("profile prompt did not carry inference configuration")
	}
	if version, ok := copilotplugins.RuntimeVersionFromCommand(received.InferenceConfig.Command); !ok || version != "1.0.104" {
		t.Fatalf("profile prompt command = %#v, want selected ACP runtime 1.0.104", received.InferenceConfig.Command)
	}
	if received.InferenceConfig.WorkDir != workDir {
		t.Fatalf("profile prompt workdir = %q, want final host utility workdir %q", received.InferenceConfig.WorkDir, workDir)
	}
	if !containsSequence(
		received.InferenceConfig.CLIFlags, "--plugin-dir", canonicalCopilotPluginRootForTest(t, runner.root),
	) {
		t.Fatalf("profile prompt flags = %#v, want selected native plugin mount", received.InferenceConfig.CLIFlags)
	}
	if received.InferenceConfig.Env["COPILOT_HOME"] != configDir {
		t.Fatalf("profile prompt COPILOT_HOME = %q, want %q", received.InferenceConfig.Env["COPILOT_HOME"], configDir)
	}
	assertCopilotUtilityInventory(t, runner, 0, "1.0.104", workDir, configDir)
}

func assertCopilotUtilityInventory(t *testing.T, runner *copilotUtilityRunner, index int, version, workDir, configDir string) {
	t.Helper()
	if index >= len(runner.argvs) || index >= len(runner.envs) || index >= len(runner.workDirs) {
		t.Fatalf("inventory index %d missing from %d calls", index, runner.calls)
	}
	want := agents.NewCopilotACP().ManagedNPMRuntime().InteractiveCommand(version).Args()
	got := runner.argvs[index]
	if len(got) < len(want)+2 || !reflect.DeepEqual(got[:4], want[:4]) ||
		got[5] != want[5] || !containsSequence(got, "@github/copilot@"+version, "plugin", "list") {
		t.Errorf("inventory %d argv = %#v, want selected interactive command %#v plus inventory arguments", index, got, want)
	}
	if containsSequence(got, "--acp") {
		t.Errorf("inventory %d reused ACP inference arguments: %#v", index, got)
	}
	if runner.workDirs[index] != workDir {
		t.Errorf("inventory %d workdir = %q, want final profile workdir %q", index, runner.workDirs[index], workDir)
	}
	if runner.envs[index]["COPILOT_HOME"] != configDir {
		t.Errorf("inventory %d COPILOT_HOME = %q, want %q", index, runner.envs[index]["COPILOT_HOME"], configDir)
	}
}

func containsSequence(args []string, values ...string) bool {
	for start := 0; start+len(values) <= len(args); start++ {
		matches := true
		for offset, value := range values {
			if args[start+offset] != value {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func TestCopilotPluginProfileProbe(t *testing.T) {
	// A probe without a selection never runs inventory; an agent-wide probe
	// therefore cannot inherit a concrete profile's plugin mounts.
	runner := &copilotUtilityRunner{}
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	flags, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env: map[string]string{"COPILOT_HOME": t.TempDir()},
	}, "", utilityInferenceCommand("1.0.88"))
	if err != nil || len(flags) != 0 {
		t.Fatalf("empty selection flags = %#v, err = %v", flags, err)
	}
	if runner.calls != 0 {
		t.Fatalf("empty selection ran %d inventory commands", runner.calls)
	}
	flags, err = mgr.profileCopilotPluginFlags(context.Background(), "claude-acp", ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": t.TempDir()},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, "", utilityInferenceCommand("1.0.88"))
	if err != nil || len(flags) != 0 {
		t.Fatalf("foreign provider flags = %#v, err = %v", flags, err)
	}
}

func TestCopilotPluginSupportedProfileProcesses(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	var nativeProbes atomic.Int32
	var probeRequests []agentctlutil.ProbeRequest
	manager, _ := newProfileProbeTestManager(t, agents.NewCopilotACP(), nil, func(w http.ResponseWriter, request agentctlutil.ProbeRequest) {
		nativeProbes.Add(1)
		probeRequests = append(probeRequests, request)
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"model-a","name":"Model A"}]}`))
	})
	manager.SetManagedRuntimeSelectionStore(&copilotUtilitySelection{version: "1.0.104"})
	manager.SetCopilotPluginRunner(runner)
	profileContext := ProfileProbeContext{
		Scope:                  "user-1:profile-1",
		Env:                    map[string]string{"COPILOT_HOME": configDir, "TOKEN": "secret-value"},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}

	capability, err := manager.ProbeProfileCapabilities(context.Background(), agents.CopilotACPAgentID, ProfileCapabilityRequest{
		Context: profileContext, Refresh: true,
	})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities: %v", err)
	}
	if capability.Capabilities.Status != StatusOK {
		t.Fatalf("capability status = %q, want ok", capability.Capabilities.Status)
	}
	if capability.ReasonCode != "" || capability.ContextRevision == "" {
		t.Fatalf("capability feedback = %#v, want a successful revision without a rejection reason", capability)
	}
	if strings.Contains(capability.Capabilities.Error, "secret-value") ||
		strings.Contains(capability.Capabilities.Error, profileContext.Env["COPILOT_HOME"]) {
		t.Fatalf("capability preflight exposed profile context: %#v", capability)
	}

	resolution, err := manager.ResolveModelConfig(context.Background(), agents.CopilotACPAgentID, ModelConfigResolutionRequest{
		Model: "model-a", Refresh: true, ProfileContext: &profileContext,
	})
	if err != nil {
		t.Fatalf("ResolveModelConfig: %v", err)
	}
	if resolution.Status != StatusOK {
		t.Fatalf("model option status = %q, want ok", resolution.Status)
	}
	if resolution.ReasonCode != "" || resolution.ContextRevision == "" {
		t.Fatalf("model option feedback = %#v, want a successful revision without a rejection reason", resolution)
	}
	if strings.Contains(resolution.Error, "secret-value") ||
		strings.Contains(resolution.Error, profileContext.Env["COPILOT_HOME"]) {
		t.Fatalf("model option preflight exposed profile context: %#v", resolution)
	}
	if got := nativeProbes.Load(); got != 2 {
		t.Fatalf("native probe received %d requests, want capability plus model-option probes", got)
	}
	if runner.calls != 2 {
		t.Fatalf("plugin inventory ran %d commands, want one per profile probe", runner.calls)
	}
	wantRoot := canonicalCopilotPluginRootForTest(t, runner.root)
	workDir := manager.instances[agents.CopilotACPAgentID].workDir
	for index := 0; index < runner.calls; index++ {
		assertCopilotUtilityInventory(t, runner, index, "1.0.104", workDir, configDir)
	}
	if len(probeRequests) != 2 {
		t.Fatalf("probe requests = %d, want two", len(probeRequests))
	}
	for index, request := range probeRequests {
		if request.InferenceConfig == nil || request.InferenceConfig.WorkDir != workDir {
			t.Errorf("probe %d inference context = %#v, want workdir %q", index, request.InferenceConfig, workDir)
			continue
		}
		if !containsSequence(request.InferenceConfig.CLIFlags, "--plugin-dir", wantRoot) {
			t.Errorf("probe %d profile flags = %#v, want selected plugin mount", index, request.InferenceConfig.CLIFlags)
		}
		if !containsSequence(request.InferenceConfig.Command, "@github/copilot@1.0.104", "--acp") {
			t.Errorf("probe %d command = %#v, want the selected ACP inference command", index, request.InferenceConfig.Command)
		}
	}
}

func TestCopilotPluginAgentWideProbe(t *testing.T) {
	_, runner := copilotUtilityFixture(t)
	var received agentctlutil.ProbeRequest
	manager, _ := newProfileProbeTestManager(
		t, agents.NewCopilotACP(), &copilotUtilitySelection{version: "1.0.104"},
		func(w http.ResponseWriter, request agentctlutil.ProbeRequest) {
			received = request
			_, _ = w.Write([]byte(`{"success":true,"models":[]}`))
		},
	)
	manager.SetCopilotPluginRunner(runner)
	capabilities, err := manager.Refresh(context.Background(), agents.CopilotACPAgentID)
	if err != nil {
		t.Fatalf("agent-wide Refresh: %v", err)
	}
	if capabilities.Status != StatusOK || received.InferenceConfig == nil {
		t.Fatalf("agent-wide probe result = %#v request=%#v, want a successful native probe", capabilities, received)
	}
	if version, ok := copilotplugins.RuntimeVersionFromCommand(received.InferenceConfig.Command); !ok || version != "1.0.104" {
		t.Fatalf("agent-wide probe command = %#v, want selected runtime 1.0.104", received.InferenceConfig.Command)
	}
	if containsSequence(received.InferenceConfig.CLIFlags, "--plugin-dir") {
		t.Fatalf("agent-wide probe unexpectedly received profile plugin flags: %#v", received.InferenceConfig.CLIFlags)
	}
	if runner.calls != 0 {
		t.Fatalf("agent-wide probe ran %d plugin inventories, want none", runner.calls)
	}
}

func TestCopilotPluginProfileUtilityManualFlags(t *testing.T) {
	configDir, runner := copilotUtilityFixture(t)
	mgr := &Manager{}
	mgr.SetCopilotPluginRunner(runner)
	flags, err := mgr.profileCopilotPluginFlags(context.Background(), agents.CopilotACPAgentID, ProfileProbeContext{
		Env:                    map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		CLIFlags:               []string{"--model", "auto"},
		CopilotSelectedPlugins: []string{"installed:m/spark"},
	}, t.TempDir(), utilityInferenceCommand("1.0.88"))
	if err != nil {
		t.Fatalf("utility flags with manual profile option: %v", err)
	}
	if !containsSequence(
		flags, "--model", "auto", "--plugin-dir", canonicalCopilotPluginRootForTest(t, runner.root),
	) {
		t.Fatalf("manual and selected plugin flags = %#v", flags)
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
	}, "", utilityInferenceCommand("1.0.88"))
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
	}, t.TempDir(), utilityInferenceCommand("1.0.88"))
	if err != nil {
		t.Fatalf("utility profile flags: %v", err)
	}
	if selection.calls != 0 {
		t.Fatalf("utility resolved plugins through %d later runtime reads", selection.calls)
	}
	assertCopilotUtilityInventory(t, runner, 0, "1.0.88", runner.workDir, configDir)
}

func TestCopilotPluginLegacyReasonFeedback(t *testing.T) {
	err := &copilotplugins.Error{Reason: copilotplugins.ReasonPolicyUnverified}
	if got := profileCopilotPluginStatus(err); got != StatusUnsupported {
		t.Fatalf("legacy policy status = %q, want unsupported", got)
	}
	if got := profileCopilotPluginReasonCode(agents.CopilotACPAgentID, StatusUnsupported, err); got != string(copilotplugins.ReasonPolicyUnverified) {
		t.Fatalf("legacy policy reason = %q, want %q", got, copilotplugins.ReasonPolicyUnverified)
	}
}
