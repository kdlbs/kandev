package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func TestCursorMCPDisableInheritanceUsesTrustedPrimaryRepository(t *testing.T) {
	for _, test := range []struct {
		name              string
		repositoryEnabled bool
		primaryDisabled   bool
		secondaryDisabled bool
		folderDisabled    bool
		missingPrimary    bool
		wantImported      bool
	}{
		{name: "primary repository disable", repositoryEnabled: true, primaryDisabled: true, wantImported: false},
		{name: "secondary repository disable is unrelated", repositoryEnabled: true, secondaryDisabled: true, wantImported: true},
		{name: "repository-free task ignores attached folders", folderDisabled: true, wantImported: true},
		{name: "configured repository with missing primary path fails closed", repositoryEnabled: true, missingPrimary: true, wantImported: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			cursorHome := filepath.Join(home, ".cursor")
			pluginDir := filepath.Join(cursorHome, "plugins", "local", "harness")
			require.NoError(t, os.MkdirAll(pluginDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"),
				[]byte(`{"mcpServers":{"figma":{"url":"https://fixture.example/mcp"}}}`), 0o600))
			primary := t.TempDir()
			secondary := t.TempDir()
			attachedFolder := t.TempDir()
			if test.primaryDisabled {
				writeCursorMCPDisabledForPath(t, cursorHome, primary, []string{"plugin-harness-figma"})
			}
			if test.secondaryDisabled {
				writeCursorMCPDisabledForPath(t, cursorHome, secondary, []string{"plugin-harness-figma"})
			}
			if test.folderDisabled {
				writeCursorMCPDisabledForPath(t, cursorHome, attachedFolder, []string{"plugin-harness-figma"})
			}

			metadata := map[string]interface{}{MetadataKeyRepositoryConfigured: test.repositoryEnabled}
			if test.repositoryEnabled && !test.missingPrimary {
				metadata[MetadataKeyRepositoryPath] = primary
			}
			execution := &AgentExecution{
				WorkspacePath:        t.TempDir(),
				standalonePort:       1234,
				WorkspaceSourceRoots: []string{attachedFolder, secondary},
				metadata:             metadata,
			}
			execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
			mgr := newTestManager(t)
			agent := agents.NewCursorACP()
			profile := &AgentProfileInfo{CursorPluginsMCPEnabled: true}
			require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
				agent, profile, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy))

			data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
			require.NoError(t, err)
			var config struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
			}
			require.NoError(t, json.Unmarshal(data, &config))
			if test.wantImported {
				require.Contains(t, config.MCPServers, "plugin-harness-figma")
			} else {
				require.NotContains(t, config.MCPServers, "plugin-harness-figma")
			}
		})
	}
}

func writeCursorMCPDisabledForPath(t *testing.T, cursorHome, path string, identifiers []string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(absolute)
	require.NoError(t, err)
	projectDir := filepath.Join(cursorHome, "projects", mcpconfig.DeriveCursorProjectSlug(canonical))
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	data, err := json.Marshal(identifiers)
	require.NoError(t, err)
	disabledPath := filepath.Join(projectDir, "mcp-disabled.json")
	require.NoError(t, os.WriteFile(disabledPath, data, 0o600))
	return disabledPath
}

func TestCursorMCPImportsInjectedNativeInventoryForACPAndTerminal(t *testing.T) {
	const selectedRevision = "1111111111111111111111111111111111111111"
	const disabledRevision = "3333333333333333333333333333333333333333"
	const staleRevision = "2222222222222222222222222222222222222222"
	for _, mode := range []string{"acp", "terminal"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			cursorHome := filepath.Join(home, ".cursor")
			writeCursorNativeCachePlugin(t, cursorHome, selectedRevision, `{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"}}}`)
			disabledRoot := filepath.Join(cursorHome, "plugins", "cache", "cursor-public", "harness", disabledRevision)
			require.NoError(t, os.MkdirAll(disabledRoot, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(disabledRoot, "mcp.json"),
				[]byte(`{"mcpServers":{"figma":{"url":"https://disabled.example/mcp"}}}`), 0o600))
			writeCursorNativeCachePlugin(t, cursorHome, staleRevision, `{"mcpServers":{"stale":{"url":"https://stale.example/mcp"}}}`)
			sourceRepository := t.TempDir()
			workspace := t.TempDir()
			writeCursorMCPDisabledForPath(t, cursorHome, sourceRepository, []string{"plugin-harness-figma"})
			writeCursorMCPDisabledForPath(t, cursorHome, workspace, []string{"plugin-disabled-task-server"})
			require.NoError(t, os.WriteFile(filepath.Join(cursorHome, "mcp.json"),
				[]byte(`{"mcpServers":{"plugin-disabled-task-server":{"url":"https://task-disabled.example/mcp"}}}`), 0o600))
			credentialSource := filepath.Join(cursorHome, "projects", "credential-source")
			require.NoError(t, os.MkdirAll(credentialSource, 0o700))
			credentialObject := []byte(`{"plugin-atlassian-atlassian":{"tokens":{"access_token":"synthetic-fixture-token"},"clientInfo":{"client_id":"synthetic-fixture-client"},"opaque":{"kept":true}}}`)
			require.NoError(t, os.WriteFile(filepath.Join(credentialSource, "mcp-auth.json"), credentialObject, 0o600))
			mgr := newTestManager(t)
			var inventoryCalls atomic.Int32
			mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
				inventoryCalls.Add(1)
				return mcpconfig.CursorNativeInventory{Plugins: []mcpconfig.CursorNativePlugin{
					{Name: "atlassian", Marketplace: "cursor-public", Revision: selectedRevision},
					{Name: "harness", Marketplace: "cursor-public", Revision: disabledRevision},
				}}, nil
			}
			execution := &AgentExecution{
				WorkspacePath:  workspace,
				standalonePort: 1234,
				ExecutorType:   "local",
				metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: true,
					MetadataKeyRepositoryPath: sourceRepository},
			}
			execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
			agent := agents.NewCursorACP()
			profile := &AgentProfileInfo{CursorPluginsMCPEnabled: true, CursorMCPAuthEnabled: true}
			if mode == "acp" {
				require.NoError(t, mgr.materializeRuntimeProjectMCP(context.Background(), execution, agent, profile, "local"))
			} else {
				_, err := mgr.applyPassthroughMCP(context.Background(), execution, agent.PassthroughConfig(), agent, profile)
				require.NoError(t, err)
			}
			require.EqualValues(t, 1, inventoryCalls.Load())
			config := readCursorProjectMCPForTest(t, filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
			require.Contains(t, config, "plugin-atlassian-atlassian")
			require.NotContains(t, config, "plugin-atlassian-stale")
			require.NotContains(t, config, "plugin-harness-figma")
			require.NotContains(t, config, "plugin-disabled-task-server")
			require.Contains(t, string(config["plugin-atlassian-atlassian"]), "selected.example")
			authPath := cursorMCPAuthDestinationForTest(t, filepath.Join(cursorHome, "projects"), workspace)
			sharedAuth, err := os.ReadFile(authPath)
			require.NoError(t, err)
			require.JSONEq(t, string(credentialObject), string(sharedAuth))
		})
	}
}

func TestCursorMCPSelectedProfileImportsOnlyExactNativeIdentities(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	pluginDir := filepath.Join(cursorHome, "plugins", "local", "Harness")
	require.NoError(t, os.MkdirAll(pluginDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(`{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"},"figma":{"url":"https://unselected.example/mcp"}}}`), 0o600))
	workspace := t.TempDir()
	execution := &AgentExecution{WorkspacePath: workspace, standalonePort: 1234, metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	mgr := newTestManager(t)
	profile := &AgentProfileInfo{
		CursorPluginsMCPEnabled: true,
		MCPSelectionMode:        "selected",
		MCPSelectedServers:      []string{"plugin-Harness-atlassian"},
	}

	require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
		agents.NewCursorACP(), profile, string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy))
	config := readCursorProjectMCPForTest(t, filepath.Join(workspace, ".cursor", "mcp.json"))
	require.Contains(t, config, "plugin-Harness-atlassian")
	require.NotContains(t, config, "plugin-Harness-figma")
}

func TestCursorMCPPreparationApprovesFinalOwnedIdentityWithRuntimeContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	pluginDir := filepath.Join(cursorHome, "plugins", "local", "Harness")
	require.NoError(t, os.MkdirAll(pluginDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"),
		[]byte(`{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"},"figma":{"url":"https://unselected.example/mcp"}}}`), 0o600))
	workspace := t.TempDir()
	runner := &recordingCursorNativeRunner{}
	mgr := newTestManager(t)
	mgr.SetCursorNativeMCPCommandRunner(runner)
	execution := &AgentExecution{WorkspacePath: workspace, standalonePort: 1234,
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
	execution.setRuntimeEnvironment(map[string]string{"HOME": home, "PATH": "/runtime/bin"})
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	profile := &AgentProfileInfo{CursorPluginsMCPEnabled: true, MCPSelectionMode: "selected",
		MCPSelectedServers: []string{"plugin-Harness-atlassian"}}
	cursor := agents.NewCursorACP()
	runtimeConfig := cursor.Runtime()
	runtimeConfig.Cmd = agents.NewCommand("/opt/cursor-agent", "acp")
	agent := cursorMCPRuntimeOverrideAgent{Agent: cursor, runtime: runtimeConfig}
	require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
		agent, profile, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy))

	if len(runner.calls) != 2 {
		t.Fatalf("native command calls = %#v, want one exact enable and one verification", runner.calls)
	}
	require.Equal(t, []string{"mcp", "enable", "plugin-Harness-atlassian"}, runner.calls[0].args)
	require.Equal(t, []string{"mcp", "list-tools", "plugin-Harness-atlassian"}, runner.calls[1].args)
	for _, call := range runner.calls {
		require.Equal(t, "/opt/cursor-agent", call.executable)
		require.Equal(t, workspace, call.workspace)
		require.Equal(t, home, call.env["HOME"])
		require.Equal(t, "/runtime/bin", call.env["PATH"])
	}
}

type recordingCursorNativeRunner struct {
	calls []recordedCursorNativeCall
}

type recordedCursorNativeCall struct {
	executable string
	args       []string
	workspace  string
	env        map[string]string
}

func (r *recordingCursorNativeRunner) Run(_ context.Context, executable string, args []string, workspace string, env map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	r.calls = append(r.calls, recordedCursorNativeCall{executable: executable, args: append([]string(nil), args...), workspace: workspace, env: cloneLifecycleStringMap(env)})
	if len(args) != 3 || args[0] != "mcp" {
		return mcpconfig.NativeMCPCommandResult{}, mcpconfig.ErrNativeMCPExecutableUnavailable
	}
	if args[1] == "enable" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0}, nil
	}
	if args[1] == "list-tools" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n")}, nil
	}
	return mcpconfig.NativeMCPCommandResult{}, mcpconfig.ErrNativeMCPExecutableUnavailable
}

type cursorMCPRuntimeOverrideAgent struct {
	agents.Agent
	runtime *agents.RuntimeConfig
}

func (a cursorMCPRuntimeOverrideAgent) Runtime() *agents.RuntimeConfig { return a.runtime }

func cloneLifecycleStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func TestCursorMCPInventoryFailureKeepsExplicitLocalAndGlobalDefinitions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	require.NoError(t, os.MkdirAll(filepath.Join(cursorHome, "plugins", "local", "local-plugin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cursorHome, "plugins", "local", "local-plugin", "mcp.json"),
		[]byte(`{"mcpServers":{"local":{"url":"https://local.example/mcp"}}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cursorHome, "mcp.json"),
		[]byte(`{"mcpServers":{"global":{"url":"https://global.example/mcp"}}}`), 0o600))
	const revision = "1111111111111111111111111111111111111111"
	writeCursorNativeCachePlugin(t, cursorHome, revision, `{"mcpServers":{"native":{"url":"https://native.example/mcp"}}}`)

	mgr := newTestManager(t)
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return mcpconfig.CursorNativeInventory{}, errors.New("synthetic service failure with secret")
	}
	mgr.mcpProvider = &mockMcpConfigProvider{configs: map[string]*mcpconfig.ProfileConfig{
		"explicit-profile": {Enabled: true, Servers: map[string]mcpconfig.ServerDef{
			"explicit": {Type: mcpconfig.ServerTypeHTTP, URL: "https://profile.example/mcp"},
		}},
	}}
	execution := &AgentExecution{WorkspacePath: t.TempDir(), standalonePort: 1234}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	profile := &AgentProfileInfo{ProfileID: "explicit-profile", CursorPluginsMCPEnabled: true}
	require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
		agents.NewCursorACP(), profile, string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy))
	config := readCursorProjectMCPForTest(t, filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	require.Contains(t, config, "explicit")
	require.Contains(t, config, "global")
	require.Contains(t, config, "plugin-local-plugin-local")
	require.NotContains(t, config, "plugin-atlassian-native")
}

func TestCursorMCPUnavailableDisableContextPreservesExplicitConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	require.NoError(t, os.MkdirAll(filepath.Join(cursorHome, "plugins", "local", "local-plugin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cursorHome, "plugins", "local", "local-plugin", "mcp.json"),
		[]byte(`{"mcpServers":{"local":{"url":"https://local.example/mcp"}}}`), 0o600))
	source := t.TempDir()
	disabledPath := writeCursorMCPDisabledForPath(t, cursorHome, source, nil)
	require.NoError(t, os.WriteFile(disabledPath, []byte(`{"invalid":true}`), 0o600))
	mgr := newTestManager(t)
	var inventoryCalls atomic.Int32
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		inventoryCalls.Add(1)
		return mcpconfig.CursorNativeInventory{}, nil
	}
	mgr.mcpProvider = &mockMcpConfigProvider{configs: map[string]*mcpconfig.ProfileConfig{
		"profile": {Enabled: true, Servers: map[string]mcpconfig.ServerDef{
			"explicit": {Type: mcpconfig.ServerTypeHTTP, URL: "https://profile.example/mcp"},
		}},
	}}
	execution := &AgentExecution{WorkspacePath: t.TempDir(), standalonePort: 1234,
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: true, MetadataKeyRepositoryPath: source}}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	profile := &AgentProfileInfo{ProfileID: "profile", CursorPluginsMCPEnabled: true}
	require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
		agents.NewCursorACP(), profile, string(executor.NameLocal), agents.NewCursorACP().Runtime().ProjectMCPStrategy))
	require.Zero(t, inventoryCalls.Load(), "unavailable selected preferences stop before account inventory")
	config := readCursorProjectMCPForTest(t, filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	require.Contains(t, config, "explicit")
	require.NotContains(t, config, "plugin-local-plugin-local")
}

func TestCursorMCPNewerPreparationFencesDeferredInventory(t *testing.T) {
	const oldRevision = "1111111111111111111111111111111111111111"
	const newRevision = "2222222222222222222222222222222222222222"
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	writeCursorNativeCachePlugin(t, cursorHome, oldRevision, `{"mcpServers":{"old":{"url":"https://old.example/mcp"}}}`)
	writeCursorNativeCachePlugin(t, cursorHome, newRevision, `{"mcpServers":{"new":{"url":"https://new.example/mcp"}}}`)
	workspace := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	require.NoError(t, os.Symlink(workspace, alias))
	mgr := newTestManager(t)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return cursorInventoryForRevision(oldRevision), nil
		}
		return cursorInventoryForRevision(newRevision), nil
	}
	profile := &AgentProfileInfo{CursorPluginsMCPEnabled: true}
	agent := agents.NewCursorACP()
	makeExecution := func(path string) *AgentExecution {
		execution := &AgentExecution{WorkspacePath: path, standalonePort: 1234,
			metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
		execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
		return execution
	}
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), makeExecution(alias),
			agent, profile, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy)
	}()
	<-firstStarted
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), makeExecution(workspace),
			agent, profile, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy)
	}()
	select {
	case err := <-secondDone:
		require.NoError(t, err, "newer inventory completes while the older service request is pending")
	case <-time.After(3 * time.Second):
		close(releaseFirst)
		require.NoError(t, <-firstDone)
		require.NoError(t, <-secondDone)
		t.Fatal("newer preparation waited for the older inventory request")
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	config := readCursorProjectMCPForTest(t, filepath.Join(workspace, ".cursor", "mcp.json"))
	require.Contains(t, config, "plugin-atlassian-new")
	require.NotContains(t, config, "plugin-atlassian-old")
}

func TestCursorMCPRechecksWorkspacePathAfterInventory(t *testing.T) {
	const revision = "1111111111111111111111111111111111111111"
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	writeCursorNativeCachePlugin(t, cursorHome, revision, `{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"}}}`)
	workspace := t.TempDir()
	outside := t.TempDir()
	mgr := newTestManager(t)
	inventoryStarted := make(chan struct{})
	releaseInventory := make(chan struct{})
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		close(inventoryStarted)
		<-releaseInventory
		return cursorInventoryForRevision(revision), nil
	}
	execution := &AgentExecution{WorkspacePath: workspace, standalonePort: 1234,
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	agent := agents.NewCursorACP()
	done := make(chan error, 1)
	go func() {
		done <- mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution, agent,
			&AgentProfileInfo{CursorPluginsMCPEnabled: true}, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy)
	}()
	<-inventoryStarted
	require.NoError(t, os.Symlink(outside, filepath.Join(workspace, ".cursor")))
	close(releaseInventory)
	require.ErrorContains(t, <-done, "unsafe Cursor MCP config path")
	require.NoFileExists(t, filepath.Join(outside, "mcp.json"))
	require.NoFileExists(t, filepath.Join(outside, ".kandev-mcp-imports.json"))
}

func TestCursorMCPContextCancellationDoesNotPublishInventory(t *testing.T) {
	const revision = "1111111111111111111111111111111111111111"
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorHome := filepath.Join(home, ".cursor")
	writeCursorNativeCachePlugin(t, cursorHome, revision, `{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"}}}`)
	mgr := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		cancel()
		return cursorInventoryForRevision(revision), nil
	}
	execution := &AgentExecution{WorkspacePath: t.TempDir(), standalonePort: 1234,
		metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	agent := agents.NewCursorACP()
	err := mgr.reconcileAndMaterializeCursorProjectMCP(ctx, execution, agent,
		&AgentProfileInfo{CursorPluginsMCPEnabled: true}, string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy)
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
}

func TestCursorInventoryFailureReasonIsSanitized(t *testing.T) {
	require.Equal(t, "response_invalid", cursorInventoryFailureReason(&mcpconfig.CursorInventoryError{Reason: "response_invalid"}))
	require.Equal(t, "unavailable", cursorInventoryFailureReason(errors.New("synthetic bearer secret")))
	require.Equal(t, "unavailable", cursorInventoryFailureReason(&mcpconfig.CursorInventoryError{Reason: "synthetic bearer secret"}))
}

func TestCursorMCPAuthAndImportPreferencesAreIndependent(t *testing.T) {
	const revision = "1111111111111111111111111111111111111111"
	for _, authEnabled := range []bool{false, true} {
		for _, importEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("auth_%v_import_%v", authEnabled, importEnabled), func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				cursorHome := filepath.Join(home, ".cursor")
				writeCursorNativeCachePlugin(t, cursorHome, revision, `{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"}}}`)
				credentialSource := filepath.Join(cursorHome, "projects", "credential-source")
				if err := os.MkdirAll(credentialSource, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(credentialSource, "mcp-auth.json"),
					[]byte(`{"plugin-atlassian-atlassian":{"tokens":{"access_token":"synthetic-token"}}}`), 0o600); err != nil {
					t.Fatal(err)
				}
				mgr := newTestManager(t)
				var calls atomic.Int32
				mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
					calls.Add(1)
					return cursorInventoryForRevision(revision), nil
				}
				workspace := t.TempDir()
				execution := &AgentExecution{WorkspacePath: workspace, standalonePort: 1234,
					metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
				execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
				profile := &AgentProfileInfo{CursorMCPAuthEnabled: authEnabled, CursorPluginsMCPEnabled: importEnabled}
				agent := agents.NewCursorACP()
				require.NoError(t, mgr.materializeRuntimeProjectMCP(context.Background(), execution, agent, profile, "local"))
				wantCalls := int32(0)
				if importEnabled {
					wantCalls = 1
				}
				require.Equal(t, wantCalls, calls.Load())
				config := readCursorProjectMCPForTest(t, filepath.Join(workspace, ".cursor", "mcp.json"))
				require.Equal(t, importEnabled, containsKey(config, "plugin-atlassian-atlassian"))
				authPath := cursorMCPAuthDestinationForTest(t, filepath.Join(cursorHome, "projects"), workspace)
				info, err := os.Lstat(authPath)
				if authEnabled {
					require.NoError(t, err)
					require.True(t, info.Mode()&os.ModeSymlink != 0)
				} else {
					require.True(t, errors.Is(err, os.ErrNotExist))
				}
			})
		}
	}
}

func TestCursorMCPInventoryIsSkippedForIneligibleLaunches(t *testing.T) {
	for _, test := range []struct {
		name          string
		executorType  string
		differentHome bool
		importEnabled bool
	}{
		{name: "preference disabled", executorType: string(executor.NameLocal), importEnabled: false},
		{name: "remote executor", executorType: string(executor.NameDocker), importEnabled: true},
		{name: "different runtime home", executorType: string(executor.NameLocal), differentHome: true, importEnabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			mgr := newTestManager(t)
			var calls atomic.Int32
			mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
				calls.Add(1)
				return mcpconfig.CursorNativeInventory{}, nil
			}
			execution := &AgentExecution{WorkspacePath: t.TempDir(), standalonePort: 1234,
				metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}}
			if test.differentHome {
				execution.setRuntimeEnvironment(map[string]string{"HOME": t.TempDir()})
			}
			profile := &AgentProfileInfo{CursorPluginsMCPEnabled: test.importEnabled}
			agent := agents.NewCursorACP()
			require.NoError(t, mgr.reconcileAndMaterializeCursorProjectMCP(context.Background(), execution,
				agent, profile, test.executorType, agent.Runtime().ProjectMCPStrategy))
			require.Zero(t, calls.Load())
		})
	}
}

func TestCursorMCPSourceRepositorySupportsLegacyResumeMetadata(t *testing.T) {
	ordinaryFolder := t.TempDir()
	primary := t.TempDir()
	tests := []struct {
		name          string
		metadata      map[string]interface{}
		wantPath      string
		wantAvailable bool
	}{
		{name: "repository-free resume with attached folders", metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: false}, wantAvailable: true},
		{name: "primary repository from launch metadata", metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: true, MetadataKeyRepositoryPath: primary}, wantPath: primary, wantAvailable: true},
		{name: "configured repository missing path", metadata: map[string]interface{}{MetadataKeyRepositoryConfigured: true}, wantAvailable: false},
		{name: "legacy resumed primary path", metadata: map[string]interface{}{MetadataKeyRepositoryPath: primary}, wantPath: primary, wantAvailable: true},
		{name: "legacy repository marker without path", metadata: map[string]interface{}{MetadataKeyMainRepoGitDir: filepath.Join(primary, ".git")}, wantAvailable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := &AgentExecution{WorkspaceSourceRoots: []string{ordinaryFolder}, metadata: test.metadata}
			got, available := cursorMCPSourceRepository(execution)
			require.Equal(t, test.wantPath, got)
			require.Equal(t, test.wantAvailable, available)
		})
	}
}

func containsKey(values map[string]json.RawMessage, key string) bool {
	_, ok := values[key]
	return ok
}

func writeCursorNativeCachePlugin(t *testing.T, cursorHome, revision, contents string) {
	t.Helper()
	root := filepath.Join(cursorHome, "plugins", "cache", "cursor-public", "atlassian", revision)
	require.NoError(t, os.MkdirAll(root, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mcp.json"), []byte(contents), 0o600))
}

func cursorInventoryForRevision(revision string) mcpconfig.CursorNativeInventory {
	return mcpconfig.CursorNativeInventory{Plugins: []mcpconfig.CursorNativePlugin{{
		Name: "atlassian", Marketplace: "cursor-public", Revision: revision,
	}}}
}

func readCursorProjectMCPForTest(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var document struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &document))
	return document.MCPServers
}
