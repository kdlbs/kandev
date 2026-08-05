package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestAgentProfileEnabled_RoundTrip verifies the enabled flag persists:
// new profiles default to enabled, an update can disable them, and the
// value survives a re-read.
func TestAgentProfileEnabled_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "Default",
		AgentDisplayName: "Test Agent",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}
	// The model zero value is false, but a fresh profile must be enabled.
	if !profile.Enabled {
		t.Fatal("expected new profile to default to enabled")
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile after create: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true after create round-trip")
	}

	got.Enabled = false
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("UpdateAgentProfile: %v", err)
	}
	reRead, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile after update: %v", err)
	}
	if reRead.Enabled {
		t.Fatal("expected enabled=false after update round-trip")
	}

	updatedAt, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, true)
	if err != nil {
		t.Fatalf("UpdateAgentProfileEnabled: %v", err)
	}
	if updatedAt.IsZero() {
		t.Fatal("expected enabled-only update to return its persisted timestamp")
	}
	reEnabled, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile after enabled-only update: %v", err)
	}
	if !reEnabled.Enabled {
		t.Fatal("expected enabled=true after enabled-only update")
	}
}

// TestMigration_LegacyDB_BackfillsEnabled verifies that a database created
// before the enabled column existed keeps its rows enabled after migration.
func TestMigration_LegacyDB_BackfillsEnabled(t *testing.T) {
	db := newLegacyDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'test-agent', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p1', 'a1', 'Legacy Profile', 'Test', 'some-model', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed legacy profile: %v", err)
	}

	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository on legacy DB: %v", err)
	}

	profile, err := repo.GetAgentProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("get legacy profile after migration: %v", err)
	}
	if !profile.Enabled {
		t.Fatal("expected legacy row to backfill to enabled")
	}
}
