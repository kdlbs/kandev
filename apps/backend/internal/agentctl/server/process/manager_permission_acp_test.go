package process

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/kandev/kandev/pkg/agent"
)

const (
	permissionACPPort       = 43210
	injectedKandevToolName  = "mcp__kandev__update_task_plan_kandev"
	thirdPartyToolName      = "mcp__other__search"
	permissionFlowToolCall  = "tool-1"
	permissionFlowSessionID = "session-acp"
)

type permissionACPCase struct {
	name                            string
	taskID                          string
	resume                          bool
	toolName                        string
	title                           string
	toolKind                        acpsdk.ToolKind
	toolMeta                        map[string]any
	autoSelect                      string
	wantEvent                       bool
	agentType                       string
	authPolicy                      *acpcompat.AuthPolicy
	additionalMCPServers            []config.McpServerConfig
	nativeMCPServerName             string
	nativePluginFolder              string
	nativePluginName                string
	nativePluginServerName          string
	conflictingNotificationToolName string
	permissionNameOmitted           bool
	notificationSessionID           string
	notificationToolCallID          string
	permissionToolName              string
	permissionOptions               []acpsdk.PermissionOption
	legacyCoreCatalog               bool
}

func TestInjectedKandevPermissionACPFlow(t *testing.T) {
	tests := []permissionACPCase{
		{
			name:       "fresh task auto approves injected tool",
			taskID:     "task-1",
			toolName:   injectedKandevToolName,
			title:      "Update task plan",
			toolKind:   acpsdk.ToolKindOther,
			toolMeta:   claudePermissionMeta(injectedKandevToolName),
			autoSelect: "allow-once",
		},
		{
			name:       "resumed quick chat auto approves injected tool",
			resume:     true,
			toolName:   injectedKandevToolName,
			title:      "Update task plan",
			toolKind:   acpsdk.ToolKindOther,
			toolMeta:   claudePermissionMeta(injectedKandevToolName),
			autoSelect: "allow-once",
		},
		{
			name:      "fresh task keeps Bash pending",
			taskID:    "task-2",
			toolName:  "bash",
			title:     "Run bash command",
			toolKind:  acpsdk.ToolKind("execute"),
			wantEvent: true,
		},
		{
			name:      "resumed quick chat keeps third party MCP pending",
			resume:    true,
			toolName:  thirdPartyToolName,
			title:     "Search third-party service",
			toolKind:  acpsdk.ToolKindOther,
			wantEvent: true,
		},
		{
			name:      "programmatic colliding server stays pending",
			taskID:    "task-3",
			toolName:  "mcp__kandev__external__execute",
			title:     "Execute external MCP tool",
			toolKind:  acpsdk.ToolKindOther,
			wantEvent: true,
		},
		{
			name:      "legacy Claude colliding server stays pending",
			resume:    true,
			title:     "mcp__kandev__external__execute",
			toolKind:  acpsdk.ToolKindOther,
			wantEvent: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runPermissionACPCase(t, test) })
	}
}

func TestAntigravityInjectedKandevPermissionACPFlow(t *testing.T) {
	const toolName = "mcp_kandev_step_complete_kandev"
	cliPolicy := func() *acpcompat.AuthPolicy {
		return &acpcompat.AuthPolicy{RuntimeVariant: acpcompat.AntigravityRuntimeCLI}
	}
	cliCase := func(name string) permissionACPCase {
		return permissionACPCase{
			name: name, agentType: acpcompat.AntigravityAgentID, authPolicy: cliPolicy(),
			toolName: toolName, title: "Complete task step", toolKind: acpsdk.ToolKindOther,
			autoSelect: "allow-once",
		}
	}
	tests := []permissionACPCase{
		func() permissionACPCase {
			test := cliCase("fresh CLI request approves a registered legacy Core tool")
			test.taskID = "task-antigravity-legacy"
			test.legacyCoreCatalog = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("resumed CLI request approves a registered legacy Core tool")
			test.resume = true
			test.legacyCoreCatalog = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("fresh CLI request uses typed native name")
			test.taskID = "task-antigravity"
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("resumed CLI request correlates omitted name by session and call ID")
			test.resume = true
			test.permissionNameOmitted = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("ambiguous alias remains pending")
			test.additionalMCPServers = []config.McpServerConfig{{Name: "kandev_step_complete", Type: "http", URL: "https://mcp.example.test/other"}}
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("tool missing from live Core catalog remains pending")
			test.toolName = "mcp_kandev_not_registered_kandev"
			test.title = "Unregistered MCP tool"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("native workspace registration outside injected list keeps alias pending")
			test.nativeMCPServerName = "kandev_step_complete"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("native plugin manifest registration outside injected list keeps alias pending")
			test.nativePluginFolder = "plugin-folder"
			test.nativePluginName = "kandev"
			test.nativePluginServerName = "step_complete"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("mismatched notification session remains pending")
			test.permissionNameOmitted = true
			test.notificationSessionID = "different-session"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("mismatched notification tool-call ID remains pending")
			test.permissionNameOmitted = true
			test.notificationToolCallID = "different-call"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("conflicting typed names remain pending")
			test.conflictingNotificationToolName = "mcp_other_search"
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("non-injected server remains pending")
			test.toolName = "mcp_other_search"
			test.title = "Search third-party service"
			test.additionalMCPServers = []config.McpServerConfig{{Name: "other", Type: "http", URL: "https://mcp.example.test/other"}}
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("shell permission remains pending")
			test.toolName = "bash"
			test.title = "Run bash command"
			test.toolKind = acpsdk.ToolKind("execute")
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("no offered allow choice remains pending")
			test.permissionOptions = []acpsdk.PermissionOption{{OptionId: "reject-once", Kind: acpsdk.PermissionOptionKindRejectOnce, Name: "Deny"}}
			test.wantEvent = true
			return test
		}(),
		func() permissionACPCase {
			test := cliCase("Antigravity without qualified CLI identity keeps ordinary permission flow")
			test.authPolicy = nil
			test.wantEvent = true
			return test
		}(),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runPermissionACPCase(t, test) })
	}
}

func runPermissionACPCase(t *testing.T, test permissionACPCase) {
	t.Helper()
	manager, fake, cleanup := newPermissionACPFlow(t, test)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	preparePermissionACPTestSession(t, manager, ctx, test.resume)
	drainAdapterUpdates(manager.adapter)

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- manager.adapter.Prompt(ctx, "continue", nil, 1)
	}()
	if test.conflictingNotificationToolName != "" {
		waitForAdapterToolTitle(t, manager.adapter.Updates(), "contradictory tool identity")
		close(fake.permissionGate)
	}

	if test.wantEvent {
		resolvePendingPermissionACP(t, manager, test)
	} else {
		awaitAutoApprovedPermission(t, manager, fake, test.autoSelect)
	}
	waitForPermissionPrompt(t, ctx, promptDone)

	if test.wantEvent {
		if got := fake.selectedOption(); got != "reject-once" {
			t.Fatalf("explicitly resolved permission option = %q, want reject-once", got)
		}
		return
	}

	events := collectPromptEvents(manager.adapter.Updates())
	assertPromptEvents(t, events)
	select {
	case event := <-manager.GetUpdates():
		t.Fatalf("injected Kandev request emitted delayed manager event %q", event.Type)
	default:
	}
}

func preparePermissionACPTestSession(t *testing.T, manager *Manager, ctx context.Context, resume bool) {
	t.Helper()
	if resume {
		if err := manager.adapter.LoadSession(ctx, permissionFlowSessionID, nil); err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		return
	}
	if _, err := manager.adapter.NewSession(ctx, permissionFlowMCPServers()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
}

func resolvePendingPermissionACP(t *testing.T, manager *Manager, test permissionACPCase) {
	t.Helper()
	event := waitForPermissionEvent(t, manager.GetUpdates())
	if event.Type != streams.EventTypePermissionRequest {
		t.Fatalf("manager event type = %q, want permission_request", event.Type)
	}
	if event.PermissionTitle != test.title {
		t.Fatalf("permission title = %q, want %q", event.PermissionTitle, test.title)
	}
	if _, err := manager.ResolvePermission(event.RequestID, event.PendingID, "reject-once"); err != nil {
		t.Fatalf("ResolvePermission: %v", err)
	}
}

func awaitAutoApprovedPermission(t *testing.T, manager *Manager, fake *permissionFlowAgent, wantOption string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	select {
	case event := <-manager.GetUpdates():
		t.Fatalf("injected Kandev request emitted manager event %q", event.Type)
	case <-fake.responseReady:
		if fake.selectedOption() != wantOption {
			t.Fatalf("selected permission option = %q, want %q", fake.selectedOption(), wantOption)
		}
	case <-deadline.C:
		t.Fatal("injected Kandev permission did not complete")
	}
}

func waitForPermissionPrompt(t *testing.T, ctx context.Context, promptDone <-chan error) {
	t.Helper()
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("Prompt did not complete: %v", ctx.Err())
	}
}

type permissionFlowAgent struct {
	burstPermissionAgent
	conn                            *acpsdk.AgentSideConnection
	toolName                        string
	title                           string
	toolKind                        acpsdk.ToolKind
	toolMeta                        map[string]any
	permissionNameOmitted           bool
	conflictingNotificationToolName string
	permissionGate                  chan struct{}
	notificationSessionID           string
	notificationToolCallID          string
	permissionToolName              string
	options                         []acpsdk.PermissionOption
	responseReady                   chan struct{}

	selectedMu sync.Mutex
	selected   string
}

// The ACP server sends requests to this fake agent only through the methods
// below. All agent-to-client traffic in the test goes through conn, matching
// the production JSON-RPC boundary.
type burstPermissionAgent struct{}

func (burstPermissionAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (burstPermissionAgent) Initialize(_ context.Context, req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: req.ProtocolVersion,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession: true,
			McpCapabilities: acpsdk.McpCapabilities{
				Http: true,
				Sse:  true,
			},
		},
	}, nil
}

func (burstPermissionAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}

func (burstPermissionAgent) Cancel(context.Context, acpsdk.CancelNotification) error { return nil }

func (burstPermissionAgent) CloseSession(context.Context, acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	return acpsdk.CloseSessionResponse{}, nil
}

func (burstPermissionAgent) DeleteSession(context.Context, acpsdk.DeleteSessionRequest) (acpsdk.DeleteSessionResponse, error) {
	return acpsdk.DeleteSessionResponse{}, errors.New("not implemented")
}

func (burstPermissionAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}

func (burstPermissionAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: permissionFlowSessionID}, nil
}

func (burstPermissionAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}

func (burstPermissionAgent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}

func (burstPermissionAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func (a *permissionFlowAgent) LoadSession(context.Context, acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	return acpsdk.LoadSessionResponse{}, nil
}

func (a *permissionFlowAgent) Prompt(ctx context.Context, req acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	title := a.title
	kind := a.toolKind
	notificationSessionID := req.SessionId
	if a.notificationSessionID != "" {
		notificationSessionID = acpsdk.SessionId(a.notificationSessionID)
	}
	notificationToolCallID := a.notificationToolCallID
	if notificationToolCallID == "" {
		notificationToolCallID = permissionFlowToolCall
	}
	status := acpsdk.ToolCallStatus("pending")
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: notificationSessionID,
		Update: acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
			ToolCallId: acpsdk.ToolCallId(notificationToolCallID),
			Title:      title,
			Kind:       kind,
			Name:       a.toolNamePtr(),
			Meta:       a.toolMeta,
			Status:     status,
		}},
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if a.conflictingNotificationToolName != "" {
		conflictingName := a.conflictingNotificationToolName
		if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
			SessionId: notificationSessionID,
			Update: acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
				ToolCallId: acpsdk.ToolCallId(notificationToolCallID),
				Title:      "contradictory tool identity",
				Kind:       kind,
				Name:       &conflictingName,
				Status:     status,
			}},
		}); err != nil {
			return acpsdk.PromptResponse{}, err
		}
		select {
		case <-a.permissionGate:
		case <-ctx.Done():
			return acpsdk.PromptResponse{}, ctx.Err()
		}
	}

	options := a.options
	if options == nil {
		options = []acpsdk.PermissionOption{
			{OptionId: "reject-once", Kind: acpsdk.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "allow-once", Kind: acpsdk.PermissionOptionKindAllowOnce, Name: "Allow once"},
		}
	}
	permissionName := a.toolNamePtr()
	if a.permissionNameOmitted {
		permissionName = nil
	} else if a.permissionToolName != "" {
		permissionName = &a.permissionToolName
	}
	response, err := a.conn.RequestPermission(ctx, acpsdk.RequestPermissionRequest{
		SessionId: req.SessionId,
		ToolCall: acpsdk.ToolCallUpdate{
			ToolCallId: permissionFlowToolCall,
			Title:      &title,
			Kind:       &kind,
			Name:       permissionName,
			Meta:       a.toolMeta,
		},
		Options: options,
	})
	if err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if response.Outcome.Selected != nil {
		a.selectedMu.Lock()
		a.selected = string(response.Outcome.Selected.OptionId)
		a.selectedMu.Unlock()
	}
	close(a.responseReady)

	if response.Outcome.Cancelled != nil {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
	}

	completed := acpsdk.ToolCallStatus("completed")
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: req.SessionId,
		Update: acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: permissionFlowToolCall,
			Status:     &completed,
			RawOutput:  "permission handled",
		}},
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: req.SessionId,
		Update: acpsdk.SessionUpdate{AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
			Content: acpsdk.TextBlock("permission handled"),
		}},
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *permissionFlowAgent) selectedOption() string {
	a.selectedMu.Lock()
	defer a.selectedMu.Unlock()
	return a.selected
}

func newPermissionACPFlow(t *testing.T, test permissionACPCase) (*Manager, *permissionFlowAgent, func()) {
	t.Helper()
	workDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	agentType := test.agentType
	if agentType == "" {
		agentType = "claude-acp"
	}
	taskID := test.taskID
	if taskID == "" {
		taskID = "task-test"
	}
	mcpProfile := profile.New(profile.SurfaceKanbanTask, nil, nil)
	if !test.legacyCoreCatalog {
		mcpProfile.BuiltinPackages = &profile.BuiltinManifest{
			Version: 1, CatalogVersion: 1, WorkspaceID: "workspace-test", TaskID: taskID,
			Surface: profile.SurfaceKanbanTask, Packages: []profile.PackageID{profile.PackageCore},
			Origins: map[profile.PackageID]string{profile.PackageCore: "required"},
		}
	}
	servers := append(permissionFlowConfigMCPServers(), test.additionalMCPServers...)
	cfg, err := (&config.Config{
		Defaults: config.InstanceDefaults{
			Protocol: agent.ProtocolACP,
			WorkDir:  workDir,
		},
	}).NewInstanceConfig(permissionACPPort, &config.InstanceOverrides{
		AgentType:  agentType,
		WorkDir:    workDir,
		Env:        []string{"HOME=" + home},
		TaskID:     taskID,
		SessionID:  permissionFlowSessionID,
		McpServers: servers,
		McpProfile: &mcpProfile,
		AuthPolicy: test.authPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if test.nativeMCPServerName != "" {
		writeNativeWorkspaceMCPConfig(t, workDir, test.nativeMCPServerName)
	}
	if test.nativePluginFolder != "" {
		writeNativeWorkspacePluginMCPConfig(t, workDir, test.nativePluginFolder, test.nativePluginName, test.nativePluginServerName)
	}
	if !cfg.InjectedKandevMCP {
		t.Fatal("NewInstanceConfig did not record injected Kandev MCP provenance")
	}
	if !injectedKandevMCPConfigured(cfg) {
		t.Fatal("NewInstanceConfig did not retain the exact injected Kandev MCP entry")
	}
	var hasCollidingServer bool
	for _, server := range cfg.McpServers {
		if server.Name == "kandev__external" {
			hasCollidingServer = true
			break
		}
	}
	if !hasCollidingServer {
		t.Fatal("NewInstanceConfig did not retain the kandev__external test server")
	}
	manager := NewManager(cfg, newTestLogger(t))
	boundMCPServer := mcpserver.NewWithProfile(nil, permissionFlowSessionID, taskID, cfg.Port, newTestLogger(t), "", false, mcpProfile)
	if !slices.Contains(boundMCPServer.BuiltinCatalog()[profile.PackageCore], "step_complete_kandev") {
		t.Fatal("production-bound MCP catalog does not expose the registered Core tool")
	}
	manager.SetBuiltinToolCatalog(boundMCPServer.BuiltinCatalog)
	_, nativeMCPInventoryKnown := manager.antigravityMCPInventorySnapshot()
	if test.authPolicy != nil && !nativeMCPInventoryKnown {
		t.Fatal("Antigravity native MCP inventory was not available for the test workspace")
	}
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig: %v", err)
	}

	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	if err := manager.adapter.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("adapter Connect: %v", err)
	}
	fake := &permissionFlowAgent{
		toolName:                        test.toolName,
		title:                           test.title,
		toolKind:                        test.toolKind,
		toolMeta:                        test.toolMeta,
		permissionNameOmitted:           test.permissionNameOmitted,
		conflictingNotificationToolName: test.conflictingNotificationToolName,
		permissionGate:                  make(chan struct{}),
		notificationSessionID:           test.notificationSessionID,
		notificationToolCallID:          test.notificationToolCallID,
		permissionToolName:              test.permissionToolName,
		options:                         test.permissionOptions,
		responseReady:                   make(chan struct{}),
	}
	fake.conn = acpsdk.NewAgentSideConnection(fake, agentToClientWriter, clientToAgentReader)

	cleanup := func() {
		_ = manager.adapter.Close()
		_ = boundMCPServer.Close(context.Background())
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
		manager.lifetimeCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.adapter.Initialize(ctx); err != nil {
		cleanup()
		t.Fatalf("adapter Initialize: %v", err)
	}
	return manager, fake, cleanup
}

func waitForAdapterToolTitle(t *testing.T, updates <-chan adapter.AgentEvent, title string) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-updates:
			if event.ToolTitle == title {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for ACP tool notification title %q", title)
		}
	}
}

func writeNativeWorkspaceMCPConfig(t *testing.T, workDir, serverName string) {
	t.Helper()
	path := filepath.Join(workDir, ".agents", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		serverName: map[string]any{"command": "native-mcp"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeNativeWorkspacePluginMCPConfig(t *testing.T, workDir, folder, pluginName, serverName string) {
	t.Helper()
	pluginRoot := filepath.Join(workDir, ".agents", "plugins", folder)
	manifestPath := filepath.Join(pluginRoot, "plugin.json")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]string{"name": pluginName})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		serverName: map[string]any{"command": "native-mcp"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "mcp_config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (a *permissionFlowAgent) toolNamePtr() *string {
	if a.toolName == "" {
		return nil
	}
	return &a.toolName
}

func claudePermissionMeta(toolName string) map[string]any {
	return map[string]any{
		"claudeCode": map[string]any{"toolName": toolName},
	}
}

func permissionFlowMCPServers() []types.McpServer {
	return []types.McpServer{
		{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"},
		{Name: "kandev__external", Type: "http", URL: "https://mcp.example.test/external"},
	}
}

func permissionFlowConfigMCPServers() []config.McpServerConfig {
	return []config.McpServerConfig{
		{Name: "kandev__external", Type: "http", URL: "https://mcp.example.test/external"},
	}
}

func drainAdapterUpdates(agentAdapter adapter.AgentAdapter) {
	for {
		select {
		case <-agentAdapter.Updates():
		default:
			return
		}
	}
}

func waitForPermissionEvent(t *testing.T, events <-chan adapter.AgentEvent) adapter.AgentEvent {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == streams.EventTypePermissionRequest {
				return event
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for permission_request event")
		}
	}
}

func collectPromptEvents(events <-chan adapter.AgentEvent) []adapter.AgentEvent {
	var collected []adapter.AgentEvent
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if event.Type == streams.EventTypeComplete {
				return collected
			}
		case <-deadline.C:
			return collected
		}
	}
}

func assertPromptEvents(t *testing.T, events []adapter.AgentEvent) {
	t.Helper()
	want := []string{
		streams.EventTypeToolCall,
		streams.EventTypeToolUpdate,
		streams.EventTypeMessageChunk,
		streams.EventTypeComplete,
	}
	var got []string
	for _, event := range events {
		for _, wantType := range want {
			if event.Type == wantType {
				got = append(got, event.Type)
				break
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("prompt event types = %v, want at least %v", got, want)
	}
	for i, wantType := range want {
		if got[i] != wantType {
			t.Fatalf("prompt event types = %v, want %v", got, want)
		}
	}
}
