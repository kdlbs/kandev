package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/mcp/profile"
)

type permissionCallResult struct {
	response *adapter.PermissionResponse
	err      error
}

func antigravityPermissionManager(t *testing.T, cfg *config.InstanceConfig) *Manager {
	t.Helper()
	return &Manager{
		cfg:                cfg,
		logger:             newTestLogger(t),
		updatesCh:          make(chan adapter.AgentEvent, 4),
		pendingPermissions: make(map[string]*PendingPermission),
	}
}

func startPermissionCall(m *Manager, ctx context.Context, req *adapter.PermissionRequest) <-chan permissionCallResult {
	resultCh := make(chan permissionCallResult, 1)
	go func() {
		response, err := m.handlePermissionRequest(ctx, req)
		resultCh <- permissionCallResult{response: response, err: err}
	}()
	return resultCh
}

func waitPermissionEvent(t *testing.T, m *Manager) adapter.AgentEvent {
	t.Helper()
	select {
	case event := <-m.updatesCh:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the permission event")
		return adapter.AgentEvent{}
	}
}

func waitPermissionCall(t *testing.T, resultCh <-chan permissionCallResult) permissionCallResult {
	t.Helper()
	select {
	case result := <-resultCh:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the permission response")
		return permissionCallResult{}
	}
}

func antigravityQuestionOptions() []adapter.PermissionOption {
	return []adapter.PermissionOption{
		{OptionID: "option-a", Name: "Option A", Kind: streams.PermissionOptionKindAllowOnce},
		{OptionID: "option-b", Name: "Option B", Kind: streams.PermissionOptionKindAllowOnce},
	}
}

// TestAntigravityHumanInputNeverAutoApproves pins that a native Antigravity
// question is classified before the coordinator, managed-MCP, and blanket
// auto-approval branches: no automatic policy may select an offered option,
// because every one of those options is the user's answer.
func TestAntigravityHumanInputNeverAutoApproves(t *testing.T) {
	coordinatorConfig := func(cfg *config.InstanceConfig) {
		cfg.Port = 43210
		cfg.InjectedKandevMCP = true
		cfg.McpServers = injectedKandevMCPServers(43210)
		cfg.McpMode = mcpmode.Coordinator
		coordinatorProfile := profile.NewCoordinator()
		cfg.McpProfile = &coordinatorProfile
	}
	managedConfig := func(cfg *config.InstanceConfig) {
		cfg.Port = 43210
		cfg.InjectedKandevMCP = true
		cfg.McpServers = injectedKandevMCPServers(43210)
		policy := profile.ManagedToolPolicy{
			PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
			InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
			ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			AgentToolNames: []string{"read_task"},
		}
		managedProfile := profile.New(profile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
		cfg.McpProfile = &managedProfile
	}

	tests := []struct {
		name      string
		configure func(*config.InstanceConfig)
	}{
		{name: "blanket auto approve", configure: func(*config.InstanceConfig) {}},
		{name: "coordinator allowlist", configure: coordinatorConfig},
		{name: "managed tool policy", configure: managedConfig},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &config.InstanceConfig{
				AgentType:              acpcompat.AntigravityAgentID,
				AutoApprovePermissions: true,
			}
			test.configure(cfg)
			m := antigravityPermissionManager(t, cfg)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			toolName := "mcp__kandev__list_tasks_kandev"
			resultCh := startPermissionCall(m, ctx, &adapter.PermissionRequest{
				SessionID:  "session-1",
				ToolCallID: acpcompat.AntigravityHumanInputPrefix + "question-1",
				PendingID:  "pending-human",
				Title:      "Which approach?",
				ToolName:   &toolName,
				ActionType: string(streams.ActionTypeOther),
				Options:    antigravityQuestionOptions(),
			})

			event := waitPermissionEvent(t, m)
			if event.Type != adapter.EventTypePermissionRequest {
				t.Fatalf("event type = %q, want %q", event.Type, adapter.EventTypePermissionRequest)
			}
			if !event.PermissionHumanInput {
				t.Fatal("human-input classification was not carried on the permission event")
			}
			if event.AutoApprovedOptionID != "" || event.AutoApprovalPending {
				t.Fatalf("human question was auto-answered: option=%q pending=%v",
					event.AutoApprovedOptionID, event.AutoApprovalPending)
			}
			if len(event.PermissionOptions) != 2 ||
				event.PermissionOptions[0].OptionID != "option-a" || event.PermissionOptions[0].Name != "Option A" ||
				event.PermissionOptions[1].OptionID != "option-b" || event.PermissionOptions[1].Name != "Option B" {
				t.Fatalf("options = %+v, want the exact native options preserved", event.PermissionOptions)
			}

			// A live negative observer: the request must still be waiting.
			select {
			case result := <-resultCh:
				t.Fatalf("question was answered without the user: response=%+v err=%v", result.response, result.err)
			default:
			}

			if err := m.RespondToPermission(event.PendingID, "option-b", false); err != nil {
				t.Fatalf("RespondToPermission: %v", err)
			}
			result := waitPermissionCall(t, resultCh)
			if result.err != nil {
				t.Fatalf("handlePermissionRequest returned error: %v", result.err)
			}
			if result.response == nil || result.response.Cancelled || result.response.OptionID != "option-b" {
				t.Fatalf("response = %+v, want the exact option the user selected", result.response)
			}
		})
	}

	// Control cases: the guard is limited to the exact Antigravity identity
	// and the native prefix, so ordinary approvals keep their tested policy.
	t.Run("another agent with the same prefix still auto-approves", func(t *testing.T) {
		m := antigravityPermissionManager(t, &config.InstanceConfig{
			AgentType:              "other-agent",
			AutoApprovePermissions: true,
		})
		assertAutoApprovalRetained(t, m, acpcompat.AntigravityHumanInputPrefix+"question-1")
	})

	t.Run("ordinary antigravity tool approval retains policy", func(t *testing.T) {
		m := antigravityPermissionManager(t, &config.InstanceConfig{
			AgentType:              acpcompat.AntigravityAgentID,
			AutoApprovePermissions: true,
		})
		assertAutoApprovalRetained(t, m, "tool-call-1")
	})
}

// assertAutoApprovalRetained proves an ordinary approval still takes the
// blanket auto-approval policy after the human-input guard.
func assertAutoApprovalRetained(t *testing.T, m *Manager, toolCallID string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := startPermissionCall(m, ctx, &adapter.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: toolCallID,
		PendingID:  "pending-control",
		Title:      "Run git status",
		Options: []adapter.PermissionOption{
			{OptionID: "allow-once", Name: "Allow", Kind: streams.PermissionOptionKindAllowOnce},
		},
	})
	event := waitPermissionEvent(t, m)
	if event.PermissionHumanInput {
		t.Fatal("an ordinary approval was classified as human input")
	}
	if event.AutoApprovedOptionID != "allow-once" || !event.AutoApprovalPending {
		t.Fatalf("auto approval = (%q, %v), want the blanket policy retained",
			event.AutoApprovedOptionID, event.AutoApprovalPending)
	}
	if _, err := m.ResolvePermission(event.RequestID, event.PendingID, "allow-once"); err != nil {
		t.Fatalf("ResolvePermission: %v", err)
	}
	result := waitPermissionCall(t, resultCh)
	if result.err != nil || result.response == nil || result.response.OptionID != "allow-once" {
		t.Fatalf("response = %+v, err = %v", result.response, result.err)
	}
}
