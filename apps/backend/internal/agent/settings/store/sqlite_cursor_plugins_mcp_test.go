package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCursorPluginsMCPPreferenceRoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "cursor-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profiles := []*models.AgentProfile{
		{AgentID: agent.ID, Name: "enabled", CursorPluginsMCPEnabled: true},
		{AgentID: agent.ID, Name: "disabled", CursorPluginsMCPEnabled: false},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatalf("create profile %q: %v", profile.Name, err)
		}
		got, err := repo.GetAgentProfile(ctx, profile.ID)
		if err != nil {
			t.Fatalf("get profile %q: %v", profile.Name, err)
		}
		if got.CursorPluginsMCPEnabled != profile.CursorPluginsMCPEnabled {
			t.Errorf("profile %q cursor_plugins_mcp_enabled = %v, want %v", profile.Name, got.CursorPluginsMCPEnabled, profile.CursorPluginsMCPEnabled)
		}
	}

	profiles[0].CursorPluginsMCPEnabled = false
	if err := repo.UpdateAgentProfile(ctx, profiles[0]); err != nil {
		t.Fatalf("disable preference: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profiles[0].ID)
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if got.CursorPluginsMCPEnabled {
		t.Error("cursor_plugins_mcp_enabled = true, want false after update")
	}
}
