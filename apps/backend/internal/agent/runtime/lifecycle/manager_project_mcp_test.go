package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func TestMaterializeRuntimeProjectMCPForPiWritesProjectFile(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		metadata:       map[string]interface{}{},
		standalonePort: 45678,
	}
	agentConfig, ok := mgr.registry.Get("pi-acp")
	if !ok {
		t.Fatal("pi-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json"))
	if err != nil {
		t.Fatalf("pi mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			Transport string `json:"transport"`
			URL       string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("pi mcp.json not valid JSON: %v\n%s", err, data)
	}
	kandev := payload.MCPServers[kandevMCPServerName]
	if kandev.Transport != "streamable-http" {
		t.Fatalf("kandev transport = %q, want streamable-http", kandev.Transport)
	}
	if kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev URL = %q", kandev.URL)
	}
}

func TestMaterializeRuntimeProjectMCPForCursorWritesProjectFile(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		ExecutorType:   "local",
		metadata:       map[string]interface{}{},
		standalonePort: 45678,
	}
	agentConfig, ok := mgr.registry.Get("cursor-acp")
	if !ok {
		t.Fatal("cursor-acp agent missing from test registry")
	}

	profileInfo := &AgentProfileInfo{ProfileID: "profile-1", CursorMCPAuthEnabled: true}
	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, profileInfo, "local"); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("cursor mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("cursor mcp.json not valid JSON: %v\n%s", err, data)
	}
	kandev := payload.MCPServers[kandevMCPServerName]
	if kandev.URL != "http://localhost:45678/mcp" {
		t.Fatalf("kandev URL = %q", kandev.URL)
	}
}

func TestMaterializeRuntimeProjectMCPSkipsWhenPortUnavailable(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		metadata:       map[string]interface{}{},
	}
	agentConfig, ok := mgr.registry.Get("pi-acp")
	if !ok {
		t.Fatal("pi-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	if _, err := os.Stat(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no pi mcp.json when port unavailable, got err=%v", err)
	}
}

func TestPromoteWorkspaceExecutionResetsCommandWhenProjectMCPFails(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: "pi-acp"}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".pi"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{
		ID:                   "exec-1",
		TaskID:               "task-1",
		SessionID:            "session-1",
		AgentProfileID:       "previous-candidate",
		OfficeAgentProfileID: "assignee",
		WorkspacePath:        workspace,
		metadata:             map[string]interface{}{},
		standalonePort:       45678,
	}
	writer := &captureExecutorRunningWriter{}
	mgr.SetExecutorRunningWriter(writer)
	req := &LaunchRequest{
		TaskID:             "task-1",
		SessionID:          "session-1",
		AgentProfileID:     "reviewer",
		ExecutionProfileID: "selected-candidate",
	}

	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	err := mgr.promoteWorkspaceExecution(context.Background(), execution, req)
	if err == nil {
		t.Fatal("expected promotion to fail when .pi/mcp.json cannot be written")
	}
	if execution.AgentProfileID != "previous-candidate" {
		t.Fatalf("AgentProfileID = %q, want previous-candidate after failed promotion", execution.AgentProfileID)
	}
	if execution.OfficeAgentProfileID != "assignee" {
		t.Fatalf("OfficeAgentProfileID = %q, want assignee after failed promotion", execution.OfficeAgentProfileID)
	}
	if writer.running != nil {
		t.Fatalf("failed promotion persisted executor-running state: %#v", writer.running)
	}
	if execution.AgentCommand != "" {
		t.Fatalf("AgentCommand = %q, want reset to empty", execution.AgentCommand)
	}
	if execution.ContinueCommand != "" {
		t.Fatalf("ContinueCommand = %q, want reset to empty", execution.ContinueCommand)
	}
	if execution.AgentArgs != nil {
		t.Fatalf("AgentArgs = %#v, want nil after failed promotion", execution.AgentArgs)
	}
	if execution.ContinueArgs != nil {
		t.Fatalf("ContinueArgs = %#v, want nil after failed promotion", execution.ContinueArgs)
	}
	if execution.IsPassthrough {
		t.Fatal("IsPassthrough should be reset after failed promotion")
	}
}

func TestPromoteWorkspaceExecutionUsesResolvedExecutionProfileForCursorAuth(t *testing.T) {
	const revision = "1111111111111111111111111111111111111111"
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	cursorHome := filepath.Join(home, ".cursor")
	projects := filepath.Join(cursorHome, "projects")
	sourceRepository := t.TempDir()
	secondaryRepository := t.TempDir()
	writeCursorMCPDisabledForPath(t, cursorHome, sourceRepository, []string{"plugin-atlassian-atlassian"})
	writeCursorNativeCachePlugin(t, cursorHome, revision, `{"mcpServers":{"atlassian":{"url":"https://selected.example/mcp"}}}`)
	if err := os.MkdirAll(filepath.Join(projects, "source-project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projects, "source-project", "mcp-auth.json"), []byte(`{"figma":{"token":"opaque"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mcpconfig.LinkCursorMCPAuth(workspace, cursorHome); err != nil {
		t.Fatalf("create existing bridge link: %v", err)
	}

	resolver := &mockPassthroughProfileResolver{
		profiles: map[string]*AgentProfileInfo{
			"office-stable-profile":    {AgentName: "cursor-acp", CursorMCPAuthEnabled: true},
			"cursor-execution-profile": {AgentName: "cursor-acp", CursorMCPAuthEnabled: false, CursorPluginsMCPEnabled: true},
		},
	}
	mgr := newTestManager(t)
	mgr.profileResolver = resolver
	mgr.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
		return cursorInventoryForRevision(revision), nil
	}
	execution := &AgentExecution{
		ID:                   "exec-1",
		TaskID:               "task-1",
		SessionID:            "session-1",
		WorkspacePath:        workspace,
		WorkspaceSourceRoots: []string{secondaryRepository},
		ExecutorType:         "worktree",
		metadata:             map[string]interface{}{},
		standalonePort:       45678,
	}
	execution.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{AllowHTTP: true})
	req := &LaunchRequest{
		TaskID:             "task-1",
		SessionID:          "session-1",
		AgentProfileID:     "office-stable-profile",
		ExecutionProfileID: "cursor-execution-profile",
		ExecutorType:       "worktree",
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "primary", RepositoryPath: sourceRepository},
			{RepositoryID: "secondary", RepositoryPath: secondaryRepository},
		},
	}

	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.promoteWorkspaceExecution(context.Background(), execution, req); err != nil {
		t.Fatalf("promoteWorkspaceExecution: %v", err)
	}
	if len(resolver.resolvedIDs) != 1 || resolver.resolvedIDs[0] != "cursor-execution-profile" {
		t.Fatalf("resolved profile IDs = %v, want only cursor execution profile", resolver.resolvedIDs)
	}
	destination := cursorMCPAuthDestinationForTest(t, projects, workspace)
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("resolved disabled execution profile kept shared link, lstat err=%v", err)
	}
	materialized := readCursorProjectMCPForTest(t, filepath.Join(workspace, ".cursor", "mcp.json"))
	if _, imported := materialized["plugin-atlassian-atlassian"]; imported {
		t.Fatal("promotion imported the source-disabled native plugin")
	}
	got, available := cursorMCPSourceRepository(execution)
	if !available || got != sourceRepository {
		t.Fatalf("promoted source repository = %q, want primary RepoSpecs path %q", got, sourceRepository)
	}
}

func TestMaterializeRuntimeProjectMCP_DiscoversPluginServers(t *testing.T) {
	cursorHome := t.TempDir()
	t.Setenv("HOME", cursorHome)

	pluginDir := filepath.Join(cursorHome, ".cursor", "plugins", "local", "atlassian")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(`{
		"mcpServers": {
			"atlassian": {
				"type": "streamable-http",
				"url": "https://mcp.atlassian.com/v2/mcp"
			}
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		WorkspacePath:  t.TempDir(),
		ExecutorType:   "worktree",
		metadata:       map[string]interface{}{},
		standalonePort: 45678,
	}
	agentConfig, ok := mgr.registry.Get("cursor-acp")
	if !ok {
		t.Fatal("cursor-acp agent missing from test registry")
	}

	profileInfo := &AgentProfileInfo{
		CursorPluginsMCPEnabled: true,
	}

	if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, profileInfo, "worktree"); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("cursor mcp.json not written: %v", err)
	}
	var payload struct {
		MCPServers map[string]struct {
			URL  string `json:"url"`
			Type string `json:"type"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("cursor mcp.json not valid JSON: %v\n%s", err, data)
	}
	if _, ok := payload.MCPServers["plugin-atlassian-atlassian"]; !ok {
		t.Fatalf("atlassian plugin MCP server not found in materialized .cursor/mcp.json: %s", data)
	}
	if payload.MCPServers["plugin-atlassian-atlassian"].URL != "https://mcp.atlassian.com/v2/mcp" {
		t.Errorf("atlassian URL = %q, want https://mcp.atlassian.com/v2/mcp", payload.MCPServers["plugin-atlassian-atlassian"].URL)
	}
	if _, ok := payload.MCPServers["kandev"]; !ok {
		t.Fatalf("kandev MCP server missing: %s", data)
	}
}

func projectMCPDeliveryTestProfile() p.Context {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{
		Version: 1, CatalogVersion: 1, WorkspaceID: "workspace-1", TaskID: "task-1",
		Surface:  p.SurfaceKanbanTask,
		Packages: []p.PackageID{p.PackageCore, p.PackageDiagnostics},
		Origins:  map[p.PackageID]string{p.PackageCore: "required", p.PackageDiagnostics: "task"},
	}
	return p.Normalize(profile)
}

func newProjectMCPDeliveryTestExecution(t *testing.T) *AgentExecution {
	t.Helper()
	encoded, err := p.MarshalBuiltinBinding(projectMCPDeliveryTestProfile())
	if err != nil {
		t.Fatalf("marshal built-in binding: %v", err)
	}
	client := agentctl.NewClient("127.0.0.1", 1, newTestLogger(), agentctl.WithExecutionID("instance-1"))
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		AgentProfileID: "profile-1", WorkspacePath: t.TempDir(), metadata: map[string]interface{}{},
		standalonePort: 45678, agentctl: client,
	}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	return execution
}

// @covers AC-AGENTS-MCP-PREP-005.1, AC-AGENTS-MCP-PREP-005.4
func TestProjectMCPDeliveryPreparedOnlyAfterSuccessfulWrite(t *testing.T) {
	mgr := newTestManager(t)
	agentConfig, ok := mgr.registry.Get("pi-acp")
	if !ok {
		t.Fatal("pi-acp agent missing from test registry")
	}

	t.Run("successful write", func(t *testing.T) {
		execution := newProjectMCPDeliveryTestExecution(t)
		if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err != nil {
			t.Fatalf("materializeRuntimeProjectMCP: %v", err)
		}
		delivery := execution.projectMCPDeliverySnapshot()
		if delivery == nil {
			t.Fatal("successful project materialization produced no delivery evidence")
		}
		digest, err := p.BuiltinBindingDigest(projectMCPDeliveryTestProfile())
		if err != nil {
			t.Fatal(err)
		}
		if delivery.Strategy != types.ProjectMCPDeliveryStrategyPi || delivery.Workspace != execution.WorkspacePath ||
			delivery.TaskID != execution.TaskID || delivery.InstanceID != "instance-1" || delivery.BindingDigest != digest {
			t.Fatalf("delivery evidence is not execution-bound: %+v", delivery)
		}
		if len(delivery.Packages) != 2 || delivery.Packages[0] != string(p.PackageCore) || delivery.Packages[1] != string(p.PackageDiagnostics) {
			t.Fatalf("delivery packages = %v", delivery.Packages)
		}
	})

	t.Run("missing port", func(t *testing.T) {
		execution := newProjectMCPDeliveryTestExecution(t)
		execution.standalonePort = 0
		if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err != nil {
			t.Fatalf("materializeRuntimeProjectMCP: %v", err)
		}
		if delivery := execution.projectMCPDeliverySnapshot(); delivery != nil {
			t.Fatalf("missing port produced delivery evidence: %+v", delivery)
		}
	})

	t.Run("failed write clears prior evidence", func(t *testing.T) {
		execution := newProjectMCPDeliveryTestExecution(t)
		if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err != nil {
			t.Fatalf("materializeRuntimeProjectMCP: %v", err)
		}
		if execution.projectMCPDeliverySnapshot() == nil {
			t.Fatal("baseline materialization produced no evidence")
		}
		if err := os.RemoveAll(filepath.Join(execution.WorkspacePath, ".pi")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(execution.WorkspacePath, ".pi"), []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := mgr.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, ""); err == nil {
			t.Fatal("expected materialization to fail")
		}
		if delivery := execution.projectMCPDeliverySnapshot(); delivery != nil {
			t.Fatalf("failed write kept delivery evidence: %+v", delivery)
		}
	})
}

// @covers AC-AGENTS-MCP-PREP-005.1, AC-AGENTS-BUILTIN-MCP-003.6
func TestPromoteWorkspaceExecutionPreparesProjectDelivery(t *testing.T) {
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mcp/builtin-binding" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var profile p.Context
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(profile)
	}))
	t.Cleanup(helper.Close)
	parsed, err := url.Parse(helper.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: "pi-acp"}
	profile := projectMCPDeliveryTestProfile()
	execution := &AgentExecution{
		ID: "exec-1", TaskID: profile.BuiltinPackages.TaskID, SessionID: "session-1",
		WorkspaceID: profile.BuiltinPackages.WorkspaceID, AgentProfileID: "profile-1",
		WorkspacePath: t.TempDir(), metadata: map[string]interface{}{}, standalonePort: port,
		agentctl: agentctl.NewClient(parsed.Hostname(), port, newTestLogger(), agentctl.WithExecutionID("instance-1")),
	}
	req := &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID, WorkspaceID: execution.WorkspaceID,
		AgentProfileID: "profile-1", McpProfile: &profile,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if err := mgr.promoteWorkspaceExecution(context.Background(), execution, req); err != nil {
		t.Fatalf("promoteWorkspaceExecution: %v", err)
	}
	delivery := execution.projectMCPDeliverySnapshot()
	if delivery == nil {
		t.Fatal("promotion did not prepare project delivery evidence")
	}
	if delivery.InstanceID != "instance-1" || delivery.TaskID != execution.TaskID {
		t.Fatalf("promotion evidence is not execution-bound: %+v", delivery)
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5, AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPDeliveryHandoffRejectsReplacedHelper(t *testing.T) {
	execution := newProjectMCPDeliveryTestExecution(t)
	execution.setProjectMCPDelivery(&types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: execution.WorkspacePath, TaskID: execution.TaskID, InstanceID: "instance-1",
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core"},
	})
	current, release := execution.AcquireAgentCtlClient()
	_, err := execution.mcpProjectDeliveryFor(current)
	release()
	if err != nil {
		t.Fatalf("matching helper rejected: %v", err)
	}
	replacement := agentctl.NewClient("127.0.0.1", 2, newTestLogger(), agentctl.WithExecutionID("instance-2"))
	if _, err := execution.mcpProjectDeliveryFor(replacement); err == nil {
		t.Fatal("evidence prepared for a replaced helper configured a successor process")
	}
}
