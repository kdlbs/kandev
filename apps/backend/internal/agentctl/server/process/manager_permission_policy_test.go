package process

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestInjectedKandevPermissionPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolName := "mcp__kandev__update_task_plan_kandev"

	m := &Manager{
		cfg: &config.InstanceConfig{
			Port:              43210,
			InjectedKandevMCP: true,
			McpServers: []config.McpServerConfig{
				{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"},
				{Name: "kandev", Type: "sse", URL: "http://localhost:43210/sse"},
				{Name: "third-party", Type: "http", URL: "https://mcp.example.test/mcp"},
			},
		},
		logger:             newTestLogger(t),
		updatesCh:          make(chan adapter.AgentEvent, 2),
		pendingPermissions: make(map[string]*PendingPermission),
	}

	resultCh := make(chan struct {
		response *adapter.PermissionResponse
		err      error
	}, 1)
	go func() {
		response, err := m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
			SessionID:  "session-1",
			ToolCallID: "tool-1",
			Title:      "Update task plan",
			ToolName:   &toolName,
			ActionType: string(streams.ActionTypeOther),
			Options: []adapter.PermissionOption{
				{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
				{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
			},
		})
		resultCh <- struct {
			response *adapter.PermissionResponse
			err      error
		}{response: response, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("handlePermissionRequest returned error: %v", result.err)
		}
		if result.response == nil || result.response.Cancelled || result.response.OptionID != "allow-once" {
			t.Fatalf("response = %+v, want allow-once without cancellation", result.response)
		}
	case event := <-m.updatesCh:
		cancel()
		select {
		case <-resultCh:
		case <-time.After(time.Second):
			t.Fatal("permission request did not stop after cancellation")
		}
		t.Fatalf("injected Kandev request emitted %q instead of being approved", event.Type)
	}
}

func TestInjectedKandevPermissionOptions(t *testing.T) {
	m := injectedKandevPermissionManager(t, injectedKandevMCPServers(43210))
	toolName := "mcp__kandev__update_task_plan_kandev"

	tests := []struct {
		name       string
		options    []adapter.PermissionOption
		wantOption string
		wantOK     bool
	}{
		{
			name: "allow once wins regardless of order",
			options: []adapter.PermissionOption{
				{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
				{OptionID: "allow-always", Kind: streams.PermissionOptionKindAllowAlways},
				{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
			},
			wantOption: "allow-once",
			wantOK:     true,
		},
		{
			name: "allow always is fallback",
			options: []adapter.PermissionOption{
				{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
				{OptionID: "allow-always", Kind: streams.PermissionOptionKindAllowAlways},
			},
			wantOption: "allow-always",
			wantOK:     true,
		},
		{
			name: "reject only retains normal flow",
			options: []adapter.PermissionOption{
				{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
			},
			wantOK: false,
		},
		{
			name: "unknown option kind retains normal flow",
			options: []adapter.PermissionOption{
				{OptionID: "unknown", Kind: streams.PermissionOptionKind("maybe")},
			},
			wantOK: false,
		},
		{
			name:    "empty options retain normal flow",
			options: nil,
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, ok := m.autoApproveInjectedKandevPermission(&adapter.PermissionRequest{
				ToolName: &toolName,
				Options:  test.options,
			})
			if ok != test.wantOK {
				t.Fatalf("approved = %v, want %v; response=%+v", ok, test.wantOK, response)
			}
			if !test.wantOK {
				return
			}
			if response == nil || response.Cancelled || response.OptionID != test.wantOption {
				t.Fatalf("response = %+v, want option %q", response, test.wantOption)
			}
		})
	}
}

func TestInjectedKandevPermissionPolicyCoversServerTools(t *testing.T) {
	m := injectedKandevPermissionManager(t, injectedKandevMCPServers(43210))
	for _, toolName := range []string{
		"mcp__kandev__create_task_kandev",
		"mcp__kandev__update_task_plan_kandev",
		"mcp__kandev__delete_task_kandev",
	} {
		t.Run(toolName, func(t *testing.T) {
			response, approved := m.autoApproveInjectedKandevPermission(&adapter.PermissionRequest{
				ToolName: permissionStringPtr(toolName),
				Options: []adapter.PermissionOption{{
					OptionID: "allow-once",
					Kind:     streams.PermissionOptionKindAllowOnce,
				}},
			})
			if !approved || response == nil || response.OptionID != "allow-once" {
				t.Fatalf("tool %q response = %+v, approved = %v", toolName, response, approved)
			}
		})
	}
}

func TestInjectedKandevPolicyRunsAfterBlanketApproval(t *testing.T) {
	m := injectedKandevPermissionManager(t, injectedKandevMCPServers(43210))
	m.cfg.AutoApprovePermissions = true
	m.updatesCh = make(chan adapter.AgentEvent, 1)
	toolName := "mcp__kandev__update_task_plan_kandev"
	response, err := m.handlePermissionRequest(context.Background(), &adapter.PermissionRequest{
		ToolName: &toolName,
		Options: []adapter.PermissionOption{
			{OptionID: "allow-always", Kind: streams.PermissionOptionKindAllowAlways},
			{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if response == nil || response.OptionID != "allow-always" {
		t.Fatalf("blanket response = %+v, want first offered allow-always", response)
	}
	event := <-m.updatesCh
	if event.AutoApprovedOptionID != "allow-always" || event.AutoApprovedOptionKind != string(streams.PermissionOptionKindAllowAlways) || event.AutoApprovalSource != streams.PermissionDecisionSourceAutoApprove {
		t.Fatalf("auto-approved decision = (%q, %q, %q), want allow-always with kind and source metadata", event.AutoApprovedOptionID, event.AutoApprovedOptionKind, event.AutoApprovalSource)
	}
}

func TestInjectedKandevPermissionPolicyRejectsUntrustedRequests(t *testing.T) {
	matchingTitle := "mcp__kandev__update_task_plan_kandev"

	tests := []struct {
		name               string
		toolName           *string
		title              string
		servers            []config.McpServerConfig
		injectedProvenance bool
		wantApproved       bool
	}{
		{name: "every injected tool is eligible", toolName: stringPtr("mcp__kandev__delete_task_kandev"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: true},
		{name: "matching title cannot establish identity", toolName: nil, servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "nonmatching programmatic name wins over title", toolName: permissionStringPtr("mcp__other__tool"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "Bash title resembling MCP name is not identity", toolName: permissionStringPtr("bash"), title: "Run mcp__kandev__update_task_plan_kandev", servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "suffix alone is not qualified", toolName: permissionStringPtr("update_task_plan_kandev"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "empty qualified suffix is rejected", toolName: permissionStringPtr("mcp__kandev__"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "invalid characters are rejected", toolName: permissionStringPtr("mcp__kandev__delete/task"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
		{name: "missing provenance is rejected", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: injectedKandevMCPServers(43210), wantApproved: false},
		{name: "wrong port is rejected", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: injectedKandevMCPServers(43211), injectedProvenance: true, wantApproved: false},
		{name: "stdio server is rejected", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: []config.McpServerConfig{{Name: "kandev", Type: "stdio", Command: "agent"}}, injectedProvenance: true, wantApproved: false},
		{name: "streamable HTTP is not an injected transport", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: []config.McpServerConfig{{Name: "kandev", Type: "streamable_http", URL: "http://localhost:43210/mcp"}}, injectedProvenance: true, wantApproved: false},
		{name: "configured command invalidates HTTP entry", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: []config.McpServerConfig{{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp", Command: "spoof"}}, injectedProvenance: true, wantApproved: false},
		{name: "reserved-name spoof with headers is rejected", toolName: permissionStringPtr("mcp__kandev__update_task_plan_kandev"), servers: []config.McpServerConfig{{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp", Headers: map[string]string{"Authorization": "spoof"}}}, injectedProvenance: true, wantApproved: false},
		{name: "unrelated server does not become eligible", toolName: permissionStringPtr("mcp__third-party__tool"), servers: injectedKandevMCPServers(43210), injectedProvenance: true, wantApproved: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := injectedKandevPermissionManager(t, test.servers)
			m.cfg.InjectedKandevMCP = test.injectedProvenance
			title := test.title
			if title == "" {
				title = matchingTitle
			}
			response, approved := m.autoApproveInjectedKandevPermission(&adapter.PermissionRequest{
				Title:    title,
				ToolName: test.toolName,
				Options: []adapter.PermissionOption{{
					OptionID: "allow-once",
					Kind:     streams.PermissionOptionKindAllowOnce,
				}},
			})
			if approved != test.wantApproved {
				t.Fatalf("approved = %v, response = %+v, want %v", approved, response, test.wantApproved)
			}
		})
	}
}

func injectedKandevPermissionManager(t *testing.T, servers []config.McpServerConfig) *Manager {
	t.Helper()
	return &Manager{
		cfg: &config.InstanceConfig{
			Port:              43210,
			InjectedKandevMCP: true,
			McpServers:        servers,
		},
		logger: newTestLogger(t),
	}
}

func injectedKandevMCPServers(port int) []config.McpServerConfig {
	return []config.McpServerConfig{
		{Name: "kandev", Type: "http", URL: "http://localhost:" + strconv.Itoa(port) + "/mcp"},
		{Name: "kandev", Type: "sse", URL: "http://localhost:" + strconv.Itoa(port) + "/sse"},
		{Name: "third-party", Type: "http", URL: "https://mcp.example.test/mcp"},
	}
}

func permissionStringPtr(value string) *string { return &value }
