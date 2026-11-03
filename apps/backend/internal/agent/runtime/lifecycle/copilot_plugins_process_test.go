package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/settings/models"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type copilotConfiguredProcess struct {
	command string
	args    []string
	env     map[string]string
}

type copilotStartCapture struct {
	mu             sync.Mutex
	configured     []copilotConfiguredProcess
	startCalls     int
	failFirstStart bool
}

func newCopilotConfigureStartClient(t *testing.T, capture *copilotStartCapture) *agentctlclient.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/configure":
			var request struct {
				Command   string            `json:"command"`
				AgentArgs []string          `json:"agent_args"`
				Env       map[string]string `json:"env"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode configure request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			capture.mu.Lock()
			capture.configured = append(capture.configured, copilotConfiguredProcess{
				command: request.Command,
				args:    append([]string(nil), request.AgentArgs...),
				env:     cloneStringMap(request.Env),
			})
			capture.mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			capture.mu.Lock()
			capture.startCalls++
			fail := capture.failFirstStart && capture.startCalls == 1
			capture.mu.Unlock()
			if fail {
				_, _ = w.Write([]byte(`{"success":false,"error":"retry start"}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"command":"copilot"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return newTestAgentctlClient(t, server.URL, newTestLogger())
}

func (c *copilotStartCapture) snapshot() ([]copilotConfiguredProcess, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	configured := make([]copilotConfiguredProcess, len(c.configured))
	for index, process := range c.configured {
		configured[index] = copilotConfiguredProcess{
			command: process.command,
			args:    append([]string(nil), process.args...),
			env:     cloneStringMap(process.env),
		}
	}
	return configured, c.startCalls
}

func copilotProcessFixture(t *testing.T, pluginNames ...string) (map[string]string, map[string]string, *copilotPluginRunner) {
	t.Helper()
	root := t.TempDir()
	configRoots := make(map[string]string, len(pluginNames))
	configDirs := make(map[string]string, len(pluginNames))
	installed := make([]map[string]string, 0, len(pluginNames))
	rows := make([]map[string]any, 0, len(pluginNames))
	for _, name := range pluginNames {
		pluginRoot := filepath.Join(root, "plugin root;$(not-a-command)", name)
		if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest, err := json.Marshal(map[string]string{"name": name, "version": "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pluginRoot, "plugin.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		configDir := filepath.Join(root, "copilot config", name)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		configRoots[name] = pluginRoot
		configDirs[name] = configDir
		installed = append(installed, map[string]string{
			"name": name, "marketplace": "copilot-plugins", "version": "1.0.0", "cache_path": pluginRoot,
		})
		rows = append(rows, map[string]any{
			"name": name, "marketplace": "copilot-plugins", "version": "1.0.0",
			"enabled": true, "source": "installed",
		})
	}
	configJSON, err := json.Marshal(map[string]any{"installedPlugins": installed})
	if err != nil {
		t.Fatal(err)
	}
	for _, configDir := range configDirs {
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), configJSON, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return configRoots, configDirs, &copilotPluginRunner{stdout: inventory}
}

func copilotProcessProfile(profileID, revision, configDir string, selected ...string) *AgentProfileInfo {
	return &AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: revision,
		AgentName:   agents.CopilotACPAgentID,
		EnvVars: []models.ProfileEnvVar{
			{Key: "COPILOT_HOME", Value: configDir},
			{Key: "COPILOT_PROFILE_REV", Value: revision},
			{Key: "PATH", Value: os.Getenv("PATH")},
		},
		CopilotSelectedPlugins: append([]string{}, selected...),
	}
}

func copilotProcessEnvironment(t *testing.T, manager *Manager, profile *AgentProfileInfo, execution *AgentExecution) map[string]string {
	t.Helper()
	req := &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID, AgentProfileID: profile.ProfileID,
		EnvironmentResolutionRequired: true,
		Env:                           map[string]string{"CALLER_ENV": profile.ProfileName},
	}
	env, err := manager.buildEnvForExecution(context.Background(), execution.ID, req, agents.NewCopilotACP(), profile)
	if err != nil {
		t.Fatalf("build profile environment: %v", err)
	}
	return env
}

func buildCopilotProcessExecution(
	t *testing.T,
	manager *Manager,
	profile *AgentProfileInfo,
	client *agentctlclient.Client,
	workspace string,
	replacement bool,
	acpSessionID string,
) *AgentExecution {
	t.Helper()
	execution := copilotExecution(t, profile.ProfileID)
	execution.WorkspacePath = workspace
	execution.agentctl = client
	req := &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID,
		AgentProfileID: profile.ProfileID, ExecutorType: "local", ACPSessionID: acpSessionID,
	}
	var commands agentCommands
	var err error
	if replacement {
		commands, err = manager.buildFreshAgentCommandWithProfile(
			context.Background(), execution, agents.NewCopilotACP(), profile,
		)
	} else {
		commands, err = manager.buildAgentCommandWithContext(
			context.Background(), req, profile, agents.NewCopilotACP(), agentruntime.RuntimeStandalone, false,
		)
	}
	if err != nil {
		t.Fatalf("build Copilot command: %v", err)
	}
	execution.AgentCommand = commands.initial
	execution.ContinueCommand = commands.continue_
	execution.AgentArgs = append([]string(nil), commands.args...)
	execution.ContinueArgs = append([]string(nil), commands.continueArgs...)
	execution.LaunchWrapper = commands.wrapper
	execution.CopilotSelectedPlugins = append([]string{}, profile.CopilotSelectedPlugins...)
	execution.setRuntimeEnvironment(copilotProcessEnvironment(t, manager, profile, execution))
	return execution
}

func assertCopilotConfigure(t *testing.T, configured []copilotConfiguredProcess, root, configDir, revision, version string) {
	t.Helper()
	assertCopilotConfigureWithExpectedRoot(
		t, configured, canonicalCopilotPluginRootForTest(t, root), configDir, revision, version,
	)
}

func assertCopilotConfigureWithExpectedRoot(
	t *testing.T,
	configured []copilotConfiguredProcess,
	wantRoot, configDir, revision, version string,
) {
	t.Helper()
	if len(configured) == 0 {
		t.Fatal("agentctl received no configure request")
	}
	for index, process := range configured {
		if !containsSequence(process.args, "--plugin-dir", wantRoot) || countArg(process.args, "--plugin-dir") != 1 {
			t.Errorf("configure %d argv = %#v, want one separate mount for %q", index, process.args, wantRoot)
		}
		command := append([]string{process.command}, process.args...)
		if got, ok := copilotplugins.RuntimeVersionFromCommand(command); !ok {
			t.Errorf("configure %d command has no managed Copilot runtime: %#v", index, command)
		} else if got != version {
			t.Errorf("configure %d runtime = %q, want %q", index, got, version)
		}
		if process.env["COPILOT_HOME"] != configDir || process.env["COPILOT_PROFILE_REV"] != revision {
			t.Errorf("configure %d profile environment = %#v, want config=%q revision=%q",
				index, process.env, configDir, revision)
		}
		if process.env["CALLER_ENV"] != revision {
			t.Errorf("configure %d caller environment = %q, want %q", index, process.env["CALLER_ENV"], revision)
		}
	}
}

func countArg(args []string, value string) int {
	count := 0
	for _, arg := range args {
		if arg == value {
			count++
		}
	}
	return count
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

func TestCopilotPluginSupportedLaunchPaths(t *testing.T) {
	roots, configDirs, runner := copilotProcessFixture(t, "spark")
	profile := copilotProcessProfile("profile-1", "fresh", configDirs["spark"], "installed:copilot-plugins/spark")
	profile.CLIFlags = []models.CLIFlag{{Flag: "--plugin-dir '" + roots["spark"] + "'", Enabled: true}}

	manager := newCopilotLifecycleManager(t, profile, &copilotSelectionReader{version: "1.0.104"})
	manager.SetCopilotPluginRunner(runner)
	capture := &copilotStartCapture{failFirstStart: true}
	client := newCopilotConfigureStartClient(t, capture)
	execution := buildCopilotProcessExecution(t, manager, profile, client, t.TempDir(), false, "")

	if _, err := manager.configureAndStartAgent(context.Background(), execution); err == nil {
		t.Fatal("first start should fail to exercise the retry boundary")
	}
	if _, err := manager.configureAndStartAgent(context.Background(), execution); err != nil {
		t.Fatalf("retry configureAndStartAgent: %v", err)
	}
	configured, starts := capture.snapshot()
	if starts != 2 || len(configured) != 2 {
		t.Fatalf("configure/start counts = %d/%d, want two attempts", len(configured), starts)
	}
	assertCopilotConfigureWithExpectedRoot(
		t, configured, roots["spark"], configDirs["spark"], "fresh", "1.0.104",
	)
	if len(runner.calls) != 2 || len(runner.dirs) != 2 {
		t.Fatalf("native inventories = %d workdirs=%#v, want one per new-process attempt", len(runner.calls), runner.dirs)
	}
	for index, argv := range runner.calls {
		if !containsSequence(argv, "@github/copilot@1.0.104", "plugin", "list") {
			t.Errorf("inventory %d used another runtime: %#v", index, argv)
		}
		if runner.dirs[index] != execution.WorkspacePath {
			t.Errorf("inventory %d workdir = %q, want %q", index, runner.dirs[index], execution.WorkspacePath)
		}
		if runner.envs[index]["COPILOT_HOME"] != configDirs["spark"] ||
			runner.envs[index]["COPILOT_PROFILE_REV"] != "fresh" {
			t.Errorf("inventory %d environment = %#v", index, runner.envs[index])
		}
	}

	t.Run("conflicting manual root prevents any launch attempt", func(t *testing.T) {
		_, configDirs, runner := copilotProcessFixture(t, "spark")
		manualRoot := filepath.Join(t.TempDir(), "manual root;$(not-a-command)")
		if err := os.MkdirAll(manualRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(manualRoot, "plugin.json"),
			[]byte(`{"name":"spark","version":"1.0.0"}`), 0o644,
		); err != nil {
			t.Fatal(err)
		}
		conflictingProfile := copilotProcessProfile(
			"profile-1", "conflict", configDirs["spark"], "installed:copilot-plugins/spark",
		)
		conflictingProfile.CLIFlags = []models.CLIFlag{{
			Flag: "--plugin-dir '" + manualRoot + "'", Enabled: true,
		}}
		manager := newCopilotLifecycleManager(t, conflictingProfile, &copilotSelectionReader{version: "1.0.104"})
		manager.SetCopilotPluginRunner(runner)
		capture := &copilotStartCapture{}
		client := newCopilotConfigureStartClient(t, capture)
		execution := buildCopilotProcessExecution(
			t, manager, conflictingProfile, client, t.TempDir(), false, "",
		)

		_, err := manager.configureAndStartAgent(context.Background(), execution)
		var pluginErr *copilotplugins.Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPluginMountConflict {
			t.Fatalf("conflicting profile launch error = %#v, want typed mount-conflict error", err)
		}
		configured, starts := capture.snapshot()
		if starts != 0 || len(configured) != 0 {
			t.Fatalf("conflict launch attempted configure/start: configured=%d starts=%d", len(configured), starts)
		}
		if len(runner.calls) != 1 ||
			!containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") {
			t.Fatalf("conflict inventory commands = %#v, want only selected version 1.0.104", runner.calls)
		}
		if runner.dirs[0] != execution.WorkspacePath {
			t.Fatalf("conflict inventory workdir = %q, want %q", runner.dirs[0], execution.WorkspacePath)
		}
	})

	t.Run("replacement", func(t *testing.T) {
		roots, configDirs, runner := copilotProcessFixture(t, "spark")
		replacementProfile := copilotProcessProfile("profile-1", "replacement", configDirs["spark"], "installed:copilot-plugins/spark")
		manager := newCopilotLifecycleManager(t, replacementProfile, &copilotSelectionReader{version: "1.0.88"})
		manager.SetCopilotPluginRunner(runner)
		capture := &copilotStartCapture{}
		client := newCopilotConfigureStartClient(t, capture)
		execution := buildCopilotProcessExecution(t, manager, replacementProfile, client, t.TempDir(), true, "")
		if _, err := manager.configureAndStartAgent(context.Background(), execution); err != nil {
			t.Fatalf("replacement configureAndStartAgent: %v", err)
		}
		configured, starts := capture.snapshot()
		if starts != 1 {
			t.Fatalf("replacement starts = %d, want one", starts)
		}
		assertCopilotConfigure(t, configured, roots["spark"], configDirs["spark"], "replacement", "1.0.88")
	})

	t.Run("new-process resume recovery", func(t *testing.T) {
		roots, configDirs, runner := copilotProcessFixture(t, "spark")
		resumeProfile := copilotProcessProfile("profile-1", "resume", configDirs["spark"], "installed:copilot-plugins/spark")
		manager := newCopilotLifecycleManager(t, resumeProfile, &copilotSelectionReader{version: "1.0.104"})
		manager.SetCopilotPluginRunner(runner)
		mock := newMockAgentServer(t)
		t.Cleanup(mock.Close)
		client := createTestClient(t, mock.server.URL)
		t.Cleanup(client.Close)
		var mcpMu sync.Mutex
		var receivedMCP []agentctltypes.McpServer
		var sessionLoadCalls int
		mock.handler = func(message ws.Message) *ws.Message {
			if message.Action == "agent.session.load" {
				var request struct {
					MCPServers []agentctltypes.McpServer `json:"mcp_servers"`
				}
				if err := message.ParsePayload(&request); err != nil {
					t.Errorf("decode resumed session MCP servers: %v", err)
				} else {
					mcpMu.Lock()
					sessionLoadCalls++
					receivedMCP = append([]agentctltypes.McpServer(nil), request.MCPServers...)
					mcpMu.Unlock()
				}
			}
			return mock.defaultHandler(message)
		}
		workspace := t.TempDir()
		resumeRequest := &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1",
			AgentProfileID: resumeProfile.ProfileID, ACPSessionID: "native-session-1",
		}
		executorRequest := &ExecutorCreateRequest{
			InstanceID: "resume-recovery-execution", TaskID: resumeRequest.TaskID,
			SessionID: resumeRequest.SessionID, AgentProfileID: resumeProfile.ProfileID,
			ExecutorType: "local", WorkspacePath: workspace, AgentConfig: agents.NewCopilotACP(),
			Env: map[string]string{
				"COPILOT_HOME": configDirs["spark"], "COPILOT_PROFILE_REV": "resume",
				"CALLER_ENV": "resume", "PATH": os.Getenv("PATH"),
			},
			Metadata: map[string]interface{}{},
		}
		executorInstance := &ExecutorInstance{
			InstanceID: executorRequest.InstanceID, TaskID: resumeRequest.TaskID,
			SessionID: resumeRequest.SessionID, RuntimeName: agentruntime.RuntimeStandalone,
			WorkspacePath: workspace, Client: client,
		}
		execution, err := manager.buildExecutionFromInstance(
			context.Background(), resumeRequest, executorRequest, executorInstance,
			&MockExecutor{name: agentruntime.RuntimeStandalone}, resumeProfile,
			agents.NewCopilotACP(), nil,
		)
		if err != nil {
			t.Fatalf("build resumed execution: %v", err)
		}
		if execution.ACPSessionID != resumeRequest.ACPSessionID {
			t.Fatalf("resumed execution ACP identity = %q, want %q", execution.ACPSessionID, resumeRequest.ACPSessionID)
		}
		manager.sessionManager = NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
		if _, err := manager.configureAndStartAgent(context.Background(), execution); err != nil {
			t.Fatalf("resume configureAndStartAgent: %v", err)
		}
		configured := mock.getConfiguredAgents()
		if len(configured) != 1 {
			t.Fatalf("resume configure calls = %d, want one", len(configured))
		}
		assertCopilotConfigure(t, []copilotConfiguredProcess{{
			command: configured[0].Command, args: configured[0].AgentArgs, env: configured[0].Env,
		}}, roots["spark"], configDirs["spark"], "resume", "1.0.104")

		connectAgentStream(t, mock, client)
		mcpServers := []agentctltypes.McpServer{{
			Name: "profile-tool", Command: "node", Args: []string{"mcp.js"},
			Env: map[string]string{"PROFILE_CONTEXT": "preserved"},
		}}
		dispatched := make(chan struct{}, 1)
		execution.setInitialPromptDispatchCallbacks(nil, func() {
			dispatched <- struct{}{}
		}, nil)
		if err := manager.initializeACPSession(
			context.Background(), execution, agents.NewCopilotACP(), "resume this task", nil, mcpServers,
		); err != nil {
			t.Fatalf("initialize resumed ACP session: %v", err)
		}
		select {
		case <-dispatched:
		case <-time.After(5 * time.Second):
			t.Fatal("resumed task prompt did not dispatch")
		}
		mcpMu.Lock()
		gotMCP := append([]agentctltypes.McpServer(nil), receivedMCP...)
		loadCalls := sessionLoadCalls
		mcpMu.Unlock()
		if loadCalls != 1 || !reflect.DeepEqual(gotMCP, mcpServers) {
			t.Fatalf("ACP session/load MCP servers = %#v calls=%d, want %#v once",
				gotMCP, loadCalls, mcpServers)
		}
		actions := mock.getActionLog()
		if !containsSequence(actions, "agent.session.load") || containsSequence(actions, "agent.session.new") {
			t.Fatalf("resumed ACP actions = %#v, want native session/load without session/new", actions)
		}
	})
}

func TestCopilotPluginNativeInventoryFailurePreventsAlternateRuntimeOrProfileRetry(t *testing.T) {
	tests := []struct {
		name         string
		inventoryErr error
	}{
		{name: "native inventory rejection", inventoryErr: errors.New("native inventory rejected")},
		{name: "successful inventory control"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots, configDirs, runner := copilotProcessFixture(t, "spark")
			runner.err = test.inventoryErr
			profile := copilotProcessProfile(
				"profile-1", "native-inventory", configDirs["spark"],
				"installed:copilot-plugins/spark",
			)
			selection := &copilotSelectionReader{version: "1.0.104"}
			manager := newCopilotLifecycleManager(t, profile, selection)
			manager.SetCopilotPluginRunner(runner)
			capture := &copilotStartCapture{}
			client := newCopilotConfigureStartClient(t, capture)
			execution := buildCopilotProcessExecution(
				t, manager, profile, client, t.TempDir(), false, "",
			)
			selectionReadsBeforeInventory := selection.calls
			agentProfileID := execution.AgentProfileID
			executionProfileID := execution.ExecutionProfileID

			_, err := manager.configureAndStartAgent(context.Background(), execution)
			configured, starts := capture.snapshot()
			if test.inventoryErr != nil {
				assertCopilotInventoryRejection(t, err, configured, starts)
			} else {
				assertCopilotInventorySuccess(t, err, configured, starts, roots["spark"], configDirs["spark"])
			}

			if len(runner.calls) != 1 ||
				!containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") {
				t.Fatalf("inventory attempts = %#v, want one attempt with selected runtime 1.0.104", runner.calls)
			}
			if runner.dirs[0] != execution.WorkspacePath ||
				runner.envs[0]["COPILOT_HOME"] != configDirs["spark"] ||
				runner.envs[0]["COPILOT_PROFILE_REV"] != "native-inventory" ||
				runner.envs[0]["CALLER_ENV"] != "native-inventory" {
				t.Fatalf("inventory context = dir %q env %#v, want selected profile and workspace",
					runner.dirs[0], runner.envs[0])
			}
			if selection.calls != selectionReadsBeforeInventory {
				t.Fatalf("inventory boundary reread runtime selection %d times, want no alternate runtime lookup",
					selection.calls-selectionReadsBeforeInventory)
			}
			if execution.AgentProfileID != agentProfileID || execution.ExecutionProfileID != executionProfileID {
				t.Fatalf("inventory boundary changed profile identity: agent=%q execution=%q",
					execution.AgentProfileID, execution.ExecutionProfileID)
			}
		})
	}
}

func assertCopilotInventoryRejection(
	t *testing.T,
	err error,
	configured []copilotConfiguredProcess,
	starts int,
) {
	t.Helper()
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonCLIUnavailable {
		t.Fatalf("inventory rejection error = %#v, want typed CLI-unavailable error", err)
	}
	if len(configured) != 0 || starts != 0 {
		t.Fatalf("rejected inventory reached Configure/Start: configured=%d starts=%d", len(configured), starts)
	}
}

func assertCopilotInventorySuccess(
	t *testing.T,
	err error,
	configured []copilotConfiguredProcess,
	starts int,
	root, configDir string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("configureAndStartAgent success control: %v", err)
	}
	if len(configured) != 1 || starts != 1 {
		t.Fatalf("success control Configure/Start counts = %d/%d, want one each", len(configured), starts)
	}
	assertCopilotConfigure(t, configured, root, configDir, "native-inventory", "1.0.104")
}

type mutableCopilotProfileResolver struct {
	mu      sync.RWMutex
	profile *AgentProfileInfo
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *mutableCopilotProfileResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	if r.started != nil {
		r.once.Do(func() {
			close(r.started)
			<-r.release
		})
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneCopilotProfileInfo(r.profile), nil
}

func (r *mutableCopilotProfileResolver) set(profile *AgentProfileInfo) {
	r.mu.Lock()
	r.profile = cloneCopilotProfileInfo(profile)
	r.mu.Unlock()
}

func cloneCopilotProfileInfo(profile *AgentProfileInfo) *AgentProfileInfo {
	if profile == nil {
		return nil
	}
	clone := *profile
	clone.CopilotSelectedPlugins = append([]string{}, profile.CopilotSelectedPlugins...)
	clone.EnvVars = append([]models.ProfileEnvVar(nil), profile.EnvVars...)
	clone.CLIFlags = append([]models.CLIFlag(nil), profile.CLIFlags...)
	return &clone
}

func TestCopilotPluginProfileSnapshotCoherence(t *testing.T) {
	roots, configDirs, runner := copilotProcessFixture(t, "spark", "orbit")
	runner.started = make(chan struct{})
	runner.release = make(chan struct{})
	oldProfile := copilotProcessProfile("profile-1", "old", configDirs["spark"], "installed:copilot-plugins/spark")
	newProfile := copilotProcessProfile("profile-1", "new", configDirs["orbit"], "installed:copilot-plugins/orbit")
	reader := &copilotSelectionReader{version: "1.0.88"}
	manager := newCopilotLifecycleManager(t, oldProfile, reader)
	manager.SetCopilotPluginRunner(runner)
	resolver := &mutableCopilotProfileResolver{
		profile: cloneCopilotProfileInfo(oldProfile), started: make(chan struct{}), release: make(chan struct{}),
	}
	manager.profileResolver = resolver
	capture := &copilotStartCapture{}
	client := newCopilotConfigureStartClient(t, capture)
	execution := buildCopilotProcessExecution(t, manager, oldProfile, client, t.TempDir(), false, "")

	configureReady := make(chan struct{})
	configureRelease := make(chan struct{})
	configureResult := make(chan error, 1)
	go func() {
		close(configureReady)
		<-configureRelease
		_, err := manager.configureAndStartAgent(context.Background(), execution)
		configureResult <- err
	}()
	<-configureReady
	close(configureRelease)

	select {
	case <-resolver.started:
		resolver.set(newProfile)
		close(resolver.release)
		<-runner.started
		close(runner.release)
	case <-runner.started:
		resolver.set(newProfile)
		close(resolver.release)
		close(runner.release)
	}
	if err := <-configureResult; err != nil {
		t.Fatalf("configure old snapshot: %v", err)
	}

	configured, starts := capture.snapshot()
	if starts != 1 || len(configured) != 1 {
		t.Fatalf("initial configure/start counts = %d/%d, want one", len(configured), starts)
	}
	assertCopilotConfigure(t, configured, roots["spark"], configDirs["spark"], "old", "1.0.88")
	if len(execution.CopilotSelectedPlugins) != 1 || execution.CopilotSelectedPlugins[0] != "installed:copilot-plugins/spark" {
		t.Fatalf("in-flight execution snapshot changed: %#v", execution.CopilotSelectedPlugins)
	}

	reader.version = "1.0.104"
	nextExecution := copilotExecution(t, "profile-1")
	nextExecution.agentctl = client
	nextAgent, nextProfile, err := manager.getAgentConfigAndProfileForExecution(context.Background(), nextExecution)
	if err != nil {
		t.Fatalf("resolve next profile snapshot: %v", err)
	}
	if nextAgent.ID() != agents.CopilotACPAgentID {
		t.Fatalf("next provider = %q, want Copilot", nextAgent.ID())
	}
	nextExecution = buildCopilotProcessExecution(t, manager, nextProfile, client, t.TempDir(), false, "")
	if _, err := manager.configureAndStartAgent(context.Background(), nextExecution); err != nil {
		t.Fatalf("configure next profile snapshot: %v", err)
	}
	configured, starts = capture.snapshot()
	if starts != 2 || len(configured) != 2 {
		t.Fatalf("next configure/start counts = %d/%d, want two total", len(configured), starts)
	}
	assertCopilotConfigure(t, configured[1:], roots["orbit"], configDirs["orbit"], "new", "1.0.104")
}

func TestCopilotPluginSupportedPromotion(t *testing.T) {
	roots, configDirs, runner := copilotProcessFixture(t, "spark")
	profile := copilotProcessProfile("profile-1", "promotion", configDirs["spark"], "installed:copilot-plugins/spark")
	manager := newCopilotLifecycleManager(t, profile, &copilotSelectionReader{version: "1.0.104"})
	manager.SetCopilotPluginRunner(runner)
	capture := &copilotStartCapture{}
	client := newCopilotConfigureStartClient(t, capture)
	execution := copilotExecution(t, profile.ProfileID)
	execution.agentctl = client
	execution.AgentCommand = ""
	execution.AgentArgs = nil
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add workspace execution: %v", err)
	}
	req := &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID,
		AgentProfileID: profile.ProfileID, ExecutorType: "local",
	}
	if err := manager.promoteWorkspaceExecution(context.Background(), execution, req); err != nil {
		t.Fatalf("promote workspace execution: %v", err)
	}
	execution.setRuntimeEnvironment(copilotProcessEnvironment(t, manager, profile, execution))
	if _, err := manager.configureAndStartAgent(context.Background(), execution); err != nil {
		t.Fatalf("configure promoted process: %v", err)
	}
	configured, starts := capture.snapshot()
	if starts != 1 {
		t.Fatalf("promotion starts = %d, want one", starts)
	}
	assertCopilotConfigure(t, configured, roots["spark"], configDirs["spark"], "promotion", "1.0.104")
	if len(execution.CopilotSelectedPlugins) != 1 || execution.CopilotSelectedPlugins[0] != "installed:copilot-plugins/spark" {
		t.Fatalf("promotion did not retain the authorized selection snapshot: %#v", execution.CopilotSelectedPlugins)
	}
}

func newCopilotRestartFixture(t *testing.T, version string) (*Manager, *AgentExecution, *restartMockAgentctlServer, *copilotPluginRunner, string, string) {
	t.Helper()
	roots, configDirs, runner := copilotProcessFixture(t, "spark")
	profile := copilotProcessProfile("profile-1", "reset", configDirs["spark"], "installed:copilot-plugins/spark")
	manager := newTestManager(t)
	manager.profileResolver = &restartProfileResolver{profile: profile}
	manager.managedRuntimeSelections = &copilotSelectionReader{version: version}
	manager.SetCopilotPluginRunner(runner)

	mock := newRestartMockAgentctlServer(t, false, false)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	execution := copilotExecution(t, profile.ProfileID)
	execution.AgentID = agents.CopilotACPAgentID
	execution.ExecutionProfileID = profile.ProfileID
	execution.AgentCommand = "old-copilot"
	execution.AgentArgs = []string{"old-args"}
	execution.ACPSessionID = "old-session"
	execution.Status = v1.AgentStatusRunning
	execution.setRuntimeEnvironment(map[string]string{
		"COPILOT_HOME":        configDirs["spark"],
		"COPILOT_PROFILE_REV": "reset",
		"PATH":                os.Getenv("PATH"),
	})
	execution.agentctl = client
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	return manager, execution, mock, runner, roots["spark"], configDirs["spark"]
}

func TestCopilotPluginResetPreflightPreservesProcess(t *testing.T) {
	t.Run("unsupported runtime leaves current process untouched", func(t *testing.T) {
		manager, execution, mock, runner, _, _ := newCopilotRestartFixture(t, "1.0.200")
		oldID := execution.ACPSessionID
		oldCommand := execution.AgentCommand
		oldArgs := append([]string(nil), execution.AgentArgs...)
		oldStatus := execution.Status

		err := manager.RestartAgentProcess(context.Background(), execution.ID)
		var pluginErr *copilotplugins.Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonRuntimeUnsupported {
			t.Fatalf("reset error = %#v, want typed runtime-unsupported error", err)
		}
		if actions := mock.getHTTPActions(); len(actions) != 0 {
			t.Fatalf("preflight rejection reached agentctl actions: %#v", actions)
		}
		if execution.ACPSessionID != oldID || execution.AgentCommand != oldCommand ||
			!reflect.DeepEqual(execution.AgentArgs, oldArgs) || execution.Status != oldStatus {
			t.Fatalf("rejected reset changed current process identity or state: session=%q command=%q args=%#v status=%q",
				execution.ACPSessionID, execution.AgentCommand, execution.AgentArgs, execution.Status)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("unsupported runtime ran %d native inventory commands", len(runner.calls))
		}
	})

	t.Run("supported reset preflights then stops and starts once", func(t *testing.T) {
		manager, execution, mock, runner, root, configDir := newCopilotRestartFixture(t, "1.0.104")
		var inventoryAfterStop bool
		runner.onRun = func() {
			inventoryAfterStop = inventoryAfterStop || containsSequence(mock.getHTTPActions(), "stop")
		}
		if err := manager.RestartAgentProcess(context.Background(), execution.ID); err != nil {
			t.Fatalf("RestartAgentProcess: %v", err)
		}
		actions := mock.getHTTPActions()
		if !reflect.DeepEqual(actions, []string{"stop", "configure", "start"}) {
			t.Fatalf("restart HTTP actions = %#v, want one stop/configure/start", actions)
		}
		if inventoryAfterStop {
			t.Fatal("plugin inventory ran after the old process was stopped")
		}
		if len(runner.calls) != 1 || runner.dirs[0] != execution.WorkspacePath {
			t.Fatalf("preflight inventory calls/workdir = %d/%#v", len(runner.calls), runner.dirs)
		}
		command, args, env := mock.getConfiguredProcess()
		commandArgs := append([]string{command}, args...)
		if !containsSequence(args, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) ||
			countArg(args, "--plugin-dir") != 1 {
			t.Fatalf("restarted command args = %#v, want one prepared plugin mount", args)
		}
		if version, ok := copilotplugins.RuntimeVersionFromCommand(commandArgs); !ok || version != "1.0.104" {
			t.Fatalf("restarted command = %#v, want selected runtime 1.0.104", commandArgs)
		}
		if env["COPILOT_HOME"] != configDir || env["COPILOT_PROFILE_REV"] != "reset" {
			t.Fatalf("restarted environment = %#v", env)
		}
	})
}
