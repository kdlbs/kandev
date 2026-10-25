package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/profile"
)

func guardedManager(t *testing.T, guarded, autoApprove bool) *Manager {
	t.Helper()
	return &Manager{
		cfg: &config.InstanceConfig{
			Port:                   43210,
			InjectedKandevMCP:      true,
			SandboxRequired:        guarded,
			AutoApprovePermissions: autoApprove,
			McpServers:             []config.McpServerConfig{{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"}},
		},
		logger: newTestLogger(t),
	}
}

func allowRequest(tool string) *adapter.PermissionRequest {
	return &adapter.PermissionRequest{
		ToolName: &tool,
		Options:  []adapter.PermissionOption{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}},
	}
}

func TestGuardedKandevApproval_OnlyReadAndSelfAreApproved(t *testing.T) {
	m := guardedManager(t, true, true)
	for _, name := range profile.GuardedToolNames() {
		entry, _ := profile.GuardedToolByName(name)
		want := entry.Class == profile.ClassRead || entry.Class == profile.ClassSelf
		opt, response := m.nonCoordinatorAutoApproval(allowRequest("mcp__kandev__" + name + "_kandev"))
		if opt != nil {
			t.Errorf("%s: blanket flag approved a Kandev tool", name)
		}
		if got := response != nil; got != want {
			t.Errorf("%s (%s): approved = %v, want %v", name, entry.Class, got, want)
		}
	}
}

func TestGuardedKandevApproval_NameForms(t *testing.T) {
	m := guardedManager(t, true, true)
	for _, tool := range []string{
		"mcp__kandev__list_workspaces",          // no _kandev suffix
		"mcp__kandev__list_workspaces_kandev_x", // suffix is not terminal
		"mcp__kandev__unlisted_tool_kandev",
		"mcp__kandev__list_workspaces_kandev_kandev",
	} {
		opt, response := m.nonCoordinatorAutoApproval(allowRequest(tool))
		if opt != nil || response != nil {
			t.Errorf("%s was approved", tool)
		}
	}
}

func TestGuardedKandevApproval_OtherServersKeepBlanketFlag(t *testing.T) {
	m := guardedManager(t, true, true)
	opt, _ := m.nonCoordinatorAutoApproval(allowRequest("mcp__other__do_thing"))
	if opt == nil {
		t.Fatal("non-Kandev request lost the blanket approval")
	}
}

func TestUnguardedKandevApprovalUnchanged(t *testing.T) {
	m := guardedManager(t, false, false)
	if _, response := m.nonCoordinatorAutoApproval(allowRequest("mcp__kandev__create_task_kandev")); response == nil {
		t.Fatal("unguarded session lost injected-server approval")
	}
}

func TestAdapterAutoApproveOffWhenGuarded(t *testing.T) {
	if guardedManager(t, true, true).adapterAutoApprove() {
		t.Fatal("adapter auto-approve stayed on for a guarded session")
	}
	if !guardedManager(t, false, true).adapterAutoApprove() {
		t.Fatal("adapter auto-approve changed for an unguarded session")
	}
}

func TestGuardedKandevApproval_ExplicitExpectations(t *testing.T) {
	m := guardedManager(t, true, true)
	cases := map[string]bool{
		"list_workspaces_kandev":          true,
		"create_task_kandev":              false,
		"message_task_kandev":             false,
		"transfer_task_kandev":            false,
		"resolve_agent_permission_kandev": false,
	}
	for tool, want := range cases {
		_, response := m.nonCoordinatorAutoApproval(allowRequest("mcp__kandev__" + tool))
		if got := response != nil; got != want {
			t.Errorf("%s: approved = %v, want %v", tool, got, want)
		}
	}
}

func optionKinds(req *adapter.PermissionRequest) []streams.PermissionOptionKind {
	var kinds []streams.PermissionOptionKind
	for _, option := range req.Options {
		kinds = append(kinds, option.Kind)
	}
	return kinds
}

func promptRequest(tool string) *adapter.PermissionRequest {
	req := allowRequest(tool)
	req.Options = append(req.Options,
		adapter.PermissionOption{OptionID: "always", Kind: streams.PermissionOptionKindAllowAlways},
		adapter.PermissionOption{OptionID: "reject", Kind: streams.PermissionOptionKindRejectOnce})
	return req
}

func TestGuardedKandevPromptOffersNoStandingApproval(t *testing.T) {
	m := guardedManager(t, true, true)
	req := promptRequest("mcp__kandev__create_task_kandev")
	if opt, response := m.nonCoordinatorAutoApproval(req); opt != nil || response != nil {
		t.Fatal("mutating Kandev tool was auto-approved")
	}
	for _, kind := range optionKinds(req) {
		if kind == streams.PermissionOptionKindAllowAlways {
			t.Fatalf("prompt still offers a standing approval: %v", optionKinds(req))
		}
	}
	if len(req.Options) != 2 {
		t.Fatalf("options = %v, want allow_once and reject_once", optionKinds(req))
	}
}

func TestUnguardedKandevPromptKeepsStandingApproval(t *testing.T) {
	m := guardedManager(t, false, false)
	req := promptRequest("mcp__kandev__create_task_kandev")
	m.nonCoordinatorAutoApproval(req)
	if len(req.Options) != 3 {
		t.Fatalf("unguarded options changed: %v", optionKinds(req))
	}
}
