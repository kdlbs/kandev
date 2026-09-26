package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestSQLiteRepositoryCreatesDynamicProfileSchema(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	ctx := context.Background()
	parent := &models.Agent{ID: "dynamic", Name: "dynamic"}
	if err := repo.CreateAgent(ctx, parent); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          parent.ID,
		Name:             "Balanced",
		AgentDisplayName: "Dynamic",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create dynamic profile: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO dynamic_agent_profiles (profile_id, version, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, profile.ID, 3, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("insert dynamic profile config: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO dynamic_agent_routes
			(dynamic_profile_id, position, execution_profile_id, enabled, rules_json)
		VALUES (?, ?, ?, ?, ?)
	`, profile.ID, 0, "concrete-profile", 1, `{"on_provider_error":"try_next"}`); err != nil {
		t.Fatalf("insert dynamic route: %v", err)
	}

	var version int
	if err := db.GetContext(ctx, &version,
		`SELECT version FROM dynamic_agent_profiles WHERE profile_id = ?`, profile.ID); err != nil {
		t.Fatalf("read dynamic profile version: %v", err)
	}
	if version != 3 {
		t.Fatalf("version = %d, want 3", version)
	}
	var routes int
	if err := db.GetContext(ctx, &routes,
		`SELECT COUNT(*) FROM dynamic_agent_routes WHERE dynamic_profile_id = ?`, profile.ID); err != nil {
		t.Fatalf("count dynamic routes: %v", err)
	}
	if routes != 1 {
		t.Fatalf("dynamic routes = %d, want 1", routes)
	}
}

func TestSQLiteRepositoryDynamicProfileCRUDUsesOptimisticVersions(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "dynamic", Name: "dynamic"}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	for _, id := range []string{"profile-dynamic", "candidate-a", "candidate-b"} {
		if err := repo.CreateAgentProfile(ctx, &models.AgentProfile{
			ID:               id,
			AgentID:          "dynamic",
			Name:             id,
			AgentDisplayName: "Dynamic",
		}); err != nil {
			t.Fatalf("create profile %s: %v", id, err)
		}
	}
	// Move the concrete candidates to a real inference-family parent. The
	// store must preserve the foreign-key relationship without interpreting it.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, "concrete-agent", "concrete-agent", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("create concrete family: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE agent_profiles SET agent_id = ? WHERE id IN (?, ?)
	`, "concrete-agent", "candidate-a", "candidate-b"); err != nil {
		t.Fatalf("move candidates: %v", err)
	}

	created := &models.DynamicAgentProfile{ProfileID: "profile-dynamic", Version: 1}
	routes := []models.DynamicAgentRoute{
		{DynamicProfileID: created.ProfileID, Position: 0, ExecutionProfileID: "candidate-a", Enabled: true, RulesJSON: `{"on_provider_error":"try_next"}`},
		{DynamicProfileID: created.ProfileID, Position: 1, ExecutionProfileID: "candidate-b", Enabled: false, RulesJSON: `{}`},
	}
	if err := repo.CreateDynamicAgentProfile(ctx, created, routes); err != nil {
		t.Fatalf("create dynamic config: %v", err)
	}
	got, gotRoutes, err := repo.GetDynamicAgentProfile(ctx, created.ProfileID)
	if err != nil {
		t.Fatalf("get dynamic config: %v", err)
	}
	if got.Version != 1 || len(gotRoutes) != 2 || gotRoutes[0].ExecutionProfileID != "candidate-a" {
		t.Fatalf("dynamic config = %#v, routes = %#v", got, gotRoutes)
	}

	updated := &models.DynamicAgentProfile{ProfileID: created.ProfileID}
	if err := repo.UpdateDynamicAgentProfile(ctx, updated, 1, []models.DynamicAgentRoute{
		{DynamicProfileID: created.ProfileID, Position: 0, ExecutionProfileID: "candidate-b", Enabled: true, RulesJSON: `{}`},
	}); err != nil {
		t.Fatalf("update dynamic config: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}
	if err := repo.UpdateDynamicAgentProfile(ctx, &models.DynamicAgentProfile{ProfileID: created.ProfileID}, 1, nil); !errors.Is(err, ErrDynamicProfileVersionConflict) {
		t.Fatalf("stale update error = %v, want %v", err, ErrDynamicProfileVersionConflict)
	}
}
