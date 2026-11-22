package automation

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/testutil"
)

func TestAutomationPluginToolsPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("first schema init: %v", err)
	}
	if _, err := NewStore(database, database); err != nil {
		t.Fatalf("replayed schema init: %v", err)
	}

	ctx := context.Background()
	automation := &Automation{
		ID: "automation-plugin-tools", WorkspaceID: "ws-1", Name: "Plugin tools",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1", Enabled: true, MaxConcurrentRuns: 1,
		PluginTools: []plugintools.Selection{{
			PluginID: "kandev-security", InstallationID: "installation-1",
			PolicyVersion: "policy-1", ToolNames: []string{"list_findings", "complete_report"},
		}},
	}
	if err := store.CreateAutomation(ctx, automation); err != nil {
		t.Fatalf("create automation: %v", err)
	}
	stored, err := store.GetAutomation(ctx, automation.ID)
	if err != nil || stored == nil {
		t.Fatalf("get automation: %v", err)
	}
	if len(stored.PluginTools) != 1 || len(stored.PluginTools[0].ToolNames) != 2 {
		t.Fatalf("stored plugin tools = %#v", stored.PluginTools)
	}

	original := stored.PluginTools[0]
	if original.InstallationID != "installation-1" || original.PolicyVersion != "policy-1" {
		t.Fatalf("stored selection identity = %#v", original)
	}

	if err := store.UpdateAutomation(ctx, automation.ID, &UpdateAutomationRequest{}); err != nil {
		t.Fatalf("omitted update: %v", err)
	}
	unchanged, err := store.GetAutomation(ctx, automation.ID)
	if err != nil || unchanged == nil {
		t.Fatalf("get unchanged automation: %v", err)
	}
	if len(unchanged.PluginTools) != 1 ||
		unchanged.PluginTools[0].InstallationID != original.InstallationID ||
		unchanged.PluginTools[0].PolicyVersion != original.PolicyVersion ||
		len(unchanged.PluginTools[0].ToolNames) != 2 {
		t.Fatalf("omitted update changed plugin tools: %#v", unchanged.PluginTools)
	}

	empty := []plugintools.Selection{}
	if err := store.UpdateAutomation(ctx, automation.ID, &UpdateAutomationRequest{PluginTools: &empty}); err != nil {
		t.Fatalf("clear plugin tools: %v", err)
	}
	cleared, err := store.GetAutomation(ctx, automation.ID)
	if err != nil || cleared == nil {
		t.Fatalf("get cleared automation: %v", err)
	}
	if len(cleared.PluginTools) != 0 {
		t.Fatalf("cleared plugin tools = %#v", cleared.PluginTools)
	}
}
