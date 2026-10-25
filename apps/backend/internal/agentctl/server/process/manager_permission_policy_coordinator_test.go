package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/mcp/profile"
)

func coordinatorPermissionManager(t *testing.T) *Manager {
	t.Helper()
	m := injectedKandevPermissionManager(t, injectedKandevMCPServers(43210))
	m.cfg.McpMode = mcpmode.Coordinator
	coordinatorProfile := profile.NewCoordinator()
	m.cfg.McpProfile = &coordinatorProfile
	return m
}

// TestCoordinatorPermissionPolicyApprovesOnlyPhaseOneSevenWithoutBinding pins the exact-name
// allowlist: each of the phase-1 seven tools is auto-approved and an
// unlisted Kandev tool, even one that the generic injected-MCP policy would
// approve, is not (docs/specs/coordinator/system-design/copilot.md#permission-policy).
func TestCoordinatorPermissionPolicyApprovesOnlyPhaseOneSevenWithoutBinding(t *testing.T) {
	m := coordinatorPermissionManager(t)
	allowOption := []adapter.PermissionOption{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}}

	for _, toolName := range []string{
		"mcp__kandev__list_tasks_kandev",
		"mcp__kandev__get_task_conversation_kandev",
		"mcp__kandev__list_workflows_kandev",
		"mcp__kandev__list_workflow_steps_kandev",
		"mcp__kandev__list_repositories_kandev",
		"mcp__kandev__get_coordinator_item_kandev",
		"mcp__kandev__propose_task_kandev",
	} {
		t.Run(toolName, func(t *testing.T) {
			response, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{
				ToolName: permissionStringPtr(toolName),
				Options:  allowOption,
			})
			if !approved || response == nil || response.OptionID != "allow-once" {
				t.Fatalf("tool %q response = %+v, approved = %v", toolName, response, approved)
			}
		})
	}

	for _, toolName := range []string{
		"mcp__kandev__move_task_kandev",
		"mcp__kandev__create_task_kandev",
		"mcp__kandev__update_task_plan_kandev",
		"mcp__kandev__delete_task_kandev",
	} {
		t.Run("refuses_"+toolName, func(t *testing.T) {
			response, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{
				ToolName: permissionStringPtr(toolName),
				Options:  allowOption,
			})
			if approved {
				t.Fatalf("tool %q unexpectedly auto-approved: %+v", toolName, response)
			}
		})
	}
}

// TestCoordinatorPermissionPolicyRequiresCoordinatorMode proves the
// allowlist function only fires when the instance's own McpMode is
// coordinator, so a non-coordinator instance never gets the seven-tool
// shortcut.
func TestCoordinatorPermissionPolicyRequiresCoordinatorMode(t *testing.T) {
	m := injectedKandevPermissionManager(t, injectedKandevMCPServers(43210))
	m.cfg.McpMode = mcpmode.Task
	response, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{
		ToolName: permissionStringPtr("mcp__kandev__list_tasks_kandev"),
		Options:  []adapter.PermissionOption{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}},
	})
	if approved {
		t.Fatalf("non-coordinator instance auto-approved: %+v", response)
	}
}

// TestCoordinatorPermissionPolicyRequiresInjectedProvenance proves a
// coordinator instance never auto-approves a request whose "kandev" server
// entry does not match the genuine host-injected provenance (wrong port,
// stdio command, extra args/headers) — the same guard
// autoApproveInjectedKandevPermission uses, shared rather than duplicated.
func TestCoordinatorPermissionPolicyRequiresInjectedProvenance(t *testing.T) {
	m := injectedKandevPermissionManager(t, []config.McpServerConfig{
		{Name: "kandev", Type: "http", URL: "http://localhost:9999/mcp"}, // wrong port
	})
	m.cfg.McpMode = mcpmode.Coordinator
	coordinatorProfile := profile.NewCoordinator()
	m.cfg.McpProfile = &coordinatorProfile
	response, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{
		ToolName: permissionStringPtr("mcp__kandev__list_tasks_kandev"),
		Options:  []adapter.PermissionOption{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}},
	})
	if approved {
		t.Fatalf("spoofed kandev server entry auto-approved: %+v", response)
	}
}

// TestCoordinatorModeIgnoresBlanketApprovalAndGenericInjectedPolicy proves
// handlePermissionRequest, in coordinator mode, never falls back to the
// blanket AutoApprovePermissions flag or the generic "any kandev tool"
// injected-MCP approval — even when both would approve the request outright
// for a non-coordinator instance. An unlisted Kandev tool must reach the
// normal pending flow instead of being silently approved.
func TestCoordinatorModeIgnoresBlanketApprovalAndGenericInjectedPolicy(t *testing.T) {
	m := coordinatorPermissionManager(t)
	m.cfg.AutoApprovePermissions = true
	m.updatesCh = make(chan adapter.AgentEvent, 2)
	m.pendingPermissions = make(map[string]*PendingPermission)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolName := "mcp__kandev__move_task_kandev"

	resultCh := make(chan struct {
		response *adapter.PermissionResponse
		err      error
	}, 1)
	go func() {
		response, err := m.handlePermissionRequest(ctx, &adapter.PermissionRequest{
			SessionID:  "session-1",
			ToolCallID: "tool-1",
			Title:      "Move task",
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
		t.Fatalf("expected pending permission flow, got response=%+v err=%v", result.response, result.err)
	case <-m.updatesCh:
		cancel()
		select {
		case <-resultCh:
		case <-time.After(time.Second):
			t.Fatal("permission request did not stop after cancellation")
		}
	}
}

// TestCoordinatorModeAutoApprovesThroughHandlePermissionRequest is the
// end-to-end companion: a coordinator-mode instance auto-approves one of the
// seven tools through the full handlePermissionRequest path, even with the
// blanket flag and env-style override both set to true.
func TestCoordinatorModeAutoApprovesThroughHandlePermissionRequest(t *testing.T) {
	m := coordinatorPermissionManager(t)
	m.cfg.AutoApprovePermissions = true
	toolName := "mcp__kandev__list_tasks_kandev"

	response, err := m.handlePermissionRequest(context.Background(), &adapter.PermissionRequest{
		SessionID:  "session-1",
		ToolCallID: "tool-1",
		Title:      "List tasks",
		ToolName:   &toolName,
		ActionType: string(streams.ActionTypeOther),
		Options: []adapter.PermissionOption{
			{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce},
			{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce},
		},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if response == nil || response.Cancelled || response.OptionID != "allow-once" {
		t.Fatalf("response = %+v, want allow-once without cancellation", response)
	}
}

func TestCoordinatorPermissionPolicyApprovesExactlyTheBoundNames(t *testing.T) {
	m := coordinatorPermissionManager(t)
	m.cfg.McpProfile.CoordinatorToolPolicy = &profile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t",
		ToolNames: []string{"list_tasks_kandev", "propose_message_kandev"},
	}
	allow := []adapter.PermissionOption{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}}
	for tool, want := range map[string]bool{
		"mcp__kandev__list_tasks_kandev":        true,
		"mcp__kandev__propose_message_kandev":   true,
		"mcp__kandev__propose_task_kandev":      false, // not bound: denied by policy
		"mcp__kandev__get_coordinator_item_kan": false, // prefix of a name is not the name
		"mcp__kandev__list_tasks_kandev_extra":  false,
	} {
		_, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{ToolName: permissionStringPtr(tool), Options: allow})
		if approved != want {
			t.Errorf("tool %q approved = %v, want %v", tool, approved, want)
		}
	}
}

func TestCoordinatorPermissionPolicyApprovesNothingWithoutCoordinatorProfile(t *testing.T) {
	m := coordinatorPermissionManager(t)
	m.cfg.McpProfile = nil
	_, approved := m.autoApproveCoordinatorPermission(&adapter.PermissionRequest{
		ToolName: permissionStringPtr("mcp__kandev__list_tasks_kandev"),
		Options:  []adapter.PermissionOption{{OptionID: "allow-once", Kind: streams.PermissionOptionKindAllowOnce}},
	})
	if approved {
		t.Fatal("instance with no coordinator profile auto-approved a tool")
	}
}
