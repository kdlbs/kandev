package process

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
	name       string
	taskID     string
	resume     bool
	toolName   string
	title      string
	toolKind   acpsdk.ToolKind
	toolMeta   map[string]any
	autoSelect string
	wantEvent  bool
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
	conn          *acpsdk.AgentSideConnection
	toolName      string
	title         string
	toolKind      acpsdk.ToolKind
	toolMeta      map[string]any
	responseReady chan struct{}

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
	status := acpsdk.ToolCallStatus("pending")
	if err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: req.SessionId,
		Update: acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
			ToolCallId: permissionFlowToolCall,
			Title:      title,
			Kind:       kind,
			Name:       a.toolNamePtr(),
			Meta:       a.toolMeta,
			Status:     status,
		}},
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}

	response, err := a.conn.RequestPermission(ctx, acpsdk.RequestPermissionRequest{
		SessionId: req.SessionId,
		ToolCall: acpsdk.ToolCallUpdate{
			ToolCallId: permissionFlowToolCall,
			Title:      &title,
			Kind:       &kind,
			Name:       a.toolNamePtr(),
			Meta:       a.toolMeta,
		},
		Options: []acpsdk.PermissionOption{
			{OptionId: "reject-once", Kind: acpsdk.PermissionOptionKindRejectOnce, Name: "Reject"},
			{OptionId: "allow-once", Kind: acpsdk.PermissionOptionKindAllowOnce, Name: "Allow once"},
		},
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
	cfg := (&config.Config{
		Defaults: config.InstanceDefaults{
			Protocol: agent.ProtocolACP,
			WorkDir:  workDir,
		},
	}).NewInstanceConfig(permissionACPPort, &config.InstanceOverrides{
		AgentType:  "claude-acp",
		WorkDir:    workDir,
		TaskID:     test.taskID,
		SessionID:  permissionFlowSessionID,
		McpServers: permissionFlowConfigMCPServers(),
	})
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
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig: %v", err)
	}

	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	if err := manager.adapter.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("adapter Connect: %v", err)
	}
	fake := &permissionFlowAgent{
		toolName:      test.toolName,
		title:         test.title,
		toolKind:      test.toolKind,
		toolMeta:      test.toolMeta,
		responseReady: make(chan struct{}),
	}
	fake.conn = acpsdk.NewAgentSideConnection(fake, agentToClientWriter, clientToAgentReader)

	cleanup := func() {
		_ = manager.adapter.Close()
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
