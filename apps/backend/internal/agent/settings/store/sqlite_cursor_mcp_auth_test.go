package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCursorMCPAuthPreferenceRoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "cursor-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profiles := []*models.AgentProfile{
		{AgentID: agent.ID, Name: "enabled", CursorMCPAuthEnabled: true},
		{AgentID: agent.ID, Name: "disabled", CursorMCPAuthEnabled: false},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatalf("create profile %q: %v", profile.Name, err)
		}
		got, err := repo.GetAgentProfile(ctx, profile.ID)
		if err != nil {
			t.Fatalf("get profile %q: %v", profile.Name, err)
		}
		if got.CursorMCPAuthEnabled != profile.CursorMCPAuthEnabled {
			t.Errorf("profile %q cursor_mcp_auth_enabled = %v, want %v", profile.Name, got.CursorMCPAuthEnabled, profile.CursorMCPAuthEnabled)
		}
	}

	profiles[0].CursorMCPAuthEnabled = false
	if err := repo.UpdateAgentProfile(ctx, profiles[0]); err != nil {
		t.Fatalf("disable preference: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profiles[0].ID)
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if got.CursorMCPAuthEnabled {
		t.Fatal("explicit false was not persisted")
	}
}

func TestMigration_LegacyProfileDefaultsCursorMCPAuthEnabled(t *testing.T) {
	db := newLegacyDB(t)
	_, err := db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'cursor-acp', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	_, err = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p1', 'a1', 'Cursor', 'Cursor', 'legacy-model', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("migrate legacy db: %v", err)
	}
	profile, err := repo.GetAgentProfile(context.Background(), "p1")
	if err != nil {
		t.Fatalf("get migrated profile: %v", err)
	}
	if !profile.CursorMCPAuthEnabled {
		t.Fatal("legacy profile should default to enabled")
	}
}
