package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestExecutionAgentProfileIDRoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "dynamic"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:                 agent.ID,
		Name:                    "CEO",
		ExecutionAgentProfileID: "profile-dynamic",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.ExecutionAgentProfileID != "profile-dynamic" {
		t.Fatalf("execution_agent_profile_id = %q, want profile-dynamic", got.ExecutionAgentProfileID)
	}

	profile.ExecutionAgentProfileID = ""
	if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("clear binding: %v", err)
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if got.ExecutionAgentProfileID != "" {
		t.Fatalf("execution_agent_profile_id = %q, want empty", got.ExecutionAgentProfileID)
	}
}

func TestMigration_LegacyProfileDefaultsExecutionAgentProfileIDEmpty(t *testing.T) {
	db := newLegacyDB(t)
	if _, err := db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'dynamic', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p1', 'a1', 'CEO', 'CEO', 'legacy-model', datetime('now'), datetime('now'))`); err != nil {
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
	if profile.ExecutionAgentProfileID != "" {
		t.Fatalf("legacy execution_agent_profile_id = %q, want empty", profile.ExecutionAgentProfileID)
	}
}
