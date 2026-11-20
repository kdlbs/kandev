package profile

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestBoundCoordinatorToolNamesFallsBackToPhaseOneSeven(t *testing.T) {
	names := BoundCoordinatorToolNames(Context{})
	if len(names) != 7 || !slices.Contains(names, "propose_task_kandev") {
		t.Fatalf("phase-1 names = %v", names)
	}
	bound := Context{CoordinatorToolPolicy: &CoordinatorToolPolicy{ToolNames: []string{"list_tasks_kandev"}}}
	if got := BoundCoordinatorToolNames(bound); len(got) != 1 {
		t.Fatalf("bound names = %v", got)
	}
}

func coordinatorBindingFixture(version int) CoordinatorToolPolicy {
	return CoordinatorToolPolicy{
		Version:            version,
		CoordinatorID:      "coordinator-1",
		WorkspaceID:        "workspace-1",
		ConversationTaskID: "conversation-1",
		PolicyRevision:     3,
		ToolNames:          []string{"list_tasks_kandev"},
	}
}

func TestCoordinatorPluginBindingCompatibility(t *testing.T) {
	t.Run("version 1 binding parses with no plugin grant", func(t *testing.T) {
		encoded, err := json.Marshal(coordinatorBindingFixture(CoordinatorToolPolicyVersionV1))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		policy, err := ParseCoordinatorToolPolicyMetadata(string(encoded))
		if err != nil {
			t.Fatalf("parse v1: %v", err)
		}
		if len(policy.PluginTools) != 0 {
			t.Fatalf("v1 PluginTools = %v, want none", policy.PluginTools)
		}
		if policy.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1") {
			t.Fatal("a version-1 binding must never grant a plugin tool")
		}
	})

	t.Run("version 1 ignores a stored plugin list", func(t *testing.T) {
		policy := coordinatorBindingFixture(CoordinatorToolPolicyVersionV1)
		policy.PluginTools = []CoordinatorPluginTool{{
			PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
			InstallationID: "install-1", PolicyVersion: "policy-1",
		}}
		encoded, err := json.Marshal(policy)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		parsed, err := ParseCoordinatorToolPolicyMetadata(string(encoded))
		if err != nil {
			t.Fatalf("parse v1: %v", err)
		}
		if len(parsed.PluginTools) != 0 || parsed.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1") {
			t.Fatalf("a version-1 binding read %v plugin tools", parsed.PluginTools)
		}
	})

	t.Run("version 2 binds exact tool and installation references", func(t *testing.T) {
		policy := coordinatorBindingFixture(CoordinatorToolPolicyVersionV2)
		policy.PluginTools = []CoordinatorPluginTool{{
			PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
			InstallationID: "install-1", PolicyVersion: "policy-1",
		}}
		encoded, err := MarshalCoordinatorToolPolicy(policy)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		parsed, err := ParseCoordinatorToolPolicyMetadata(encoded)
		if err != nil {
			t.Fatalf("parse v2: %v", err)
		}
		if !parsed.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1") {
			t.Fatal("v2 binding must allow the exact reference")
		}
		if parsed.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "install-2", "policy-1") {
			t.Fatal("a changed installation must not match")
		}
		if parsed.AllowsPlugin("kandev-provider-usage", "get_provider_usage", "install-1", "policy-2") {
			t.Fatal("a changed policy version must not match")
		}
		if parsed.AllowsPlugin("kandev-provider-usage", "other_tool", "install-1", "policy-1") {
			t.Fatal("an unbound tool must not match")
		}
		if got := parsed.PluginToolNames(); len(got) != 1 || got[0] != "get_provider_usage" {
			t.Fatalf("PluginToolNames() = %v", got)
		}
	})

	t.Run("invalid versions and incomplete references are rejected", func(t *testing.T) {
		invalidVersion := coordinatorBindingFixture(3)
		if err := invalidVersion.Validate(); err == nil {
			t.Fatal("unsupported version must be rejected")
		}
		incomplete := coordinatorBindingFixture(CoordinatorToolPolicyVersionV2)
		incomplete.PluginTools = []CoordinatorPluginTool{{PluginID: "plugin", LocalName: "tool"}}
		if err := incomplete.Validate(); err == nil {
			t.Fatal("a reference without installation and policy version must be rejected")
		}
		duplicate := coordinatorBindingFixture(CoordinatorToolPolicyVersionV2)
		duplicate.PluginTools = []CoordinatorPluginTool{
			{PluginID: "plugin", LocalName: "tool", InstallationID: "install", PolicyVersion: "policy"},
			{PluginID: "plugin", LocalName: "tool", InstallationID: "install", PolicyVersion: "policy"},
		}
		if err := duplicate.Validate(); err == nil {
			t.Fatal("a duplicated reference must be rejected")
		}
	})
}
