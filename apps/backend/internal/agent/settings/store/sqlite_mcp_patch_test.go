package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestUpdateAgentProfileMcpConfigPatchPreservesUnchangedFields(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "mcp-agent", SupportsMCP: true}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "mcp-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "Default", AgentDisplayName: "MCP Agent"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := repo.UpsertAgentProfileMcpConfig(ctx, &models.AgentProfileMcpConfig{
		ProfileID: profile.ID,
		Enabled:   true,
		Servers:   map[string]interface{}{"before": map[string]interface{}{"command": "one"}},
		Meta:      map[string]interface{}{"source": "preserved"},
	}); err != nil {
		t.Fatalf("seed MCP config: %v", err)
	}

	servers := map[string]interface{}{"after": map[string]interface{}{"command": "two"}}
	updated, err := repo.UpdateAgentProfileMcpConfigPatch(ctx, profile.ID, nil, &servers)
	if err != nil {
		t.Fatalf("patch servers: %v", err)
	}
	if !updated.Enabled || updated.Meta["source"] != "preserved" {
		t.Fatalf("server patch changed unrelated fields: %#v", updated)
	}
	if _, ok := updated.Servers["after"]; !ok {
		t.Fatalf("server patch was not applied: %#v", updated.Servers)
	}

	enabled := false
	updated, err = repo.UpdateAgentProfileMcpConfigPatch(ctx, profile.ID, &enabled, nil)
	if err != nil {
		t.Fatalf("patch enabled: %v", err)
	}
	if updated.Enabled {
		t.Fatal("enabled patch was not applied")
	}
	if _, ok := updated.Servers["after"]; !ok || updated.Meta["source"] != "preserved" {
		t.Fatalf("enabled patch changed unrelated fields: %#v", updated)
	}
}
