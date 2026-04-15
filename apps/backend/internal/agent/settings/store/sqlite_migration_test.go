package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// legacySchema is the CREATE TABLE DDL that existing databases have, including
// the CHECK(model != ") constraint that the ACP-first migration must remove.
const legacySchema = `
CREATE TABLE IF NOT EXISTS agents (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	workspace_id TEXT DEFAULT NULL,
	supports_mcp INTEGER NOT NULL DEFAULT 0,
	mcp_config_path TEXT DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_profiles (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL,
	name TEXT NOT NULL,
	agent_display_name TEXT NOT NULL,
	model TEXT NOT NULL CHECK(model != ''),
	auto_approve INTEGER NOT NULL DEFAULT 0,
	dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
	allow_indexing INTEGER NOT NULL DEFAULT 1,
	cli_passthrough INTEGER NOT NULL DEFAULT 0,
	user_modified INTEGER NOT NULL DEFAULT 0,
	plan TEXT DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	deleted_at TIMESTAMP,
	FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_profile_mcp_configs (
	profile_id TEXT PRIMARY KEY,
	enabled INTEGER NOT NULL DEFAULT 0,
	servers_json TEXT NOT NULL DEFAULT '{}',
	meta_json TEXT NOT NULL DEFAULT '{}',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
CREATE INDEX IF NOT EXISTS idx_agent_profiles_agent_id ON agent_profiles(agent_id);
`

// newLegacyDB creates a SQLite database with the pre-ACP-first schema
// (including the CHECK constraint) and seeds it with test data.
func newLegacyDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}

	// Also add the tui_config column (this ALTER existed before the PR).
	if _, err := db.Exec(`ALTER TABLE agents ADD COLUMN tui_config TEXT DEFAULT NULL`); err != nil {
		t.Fatalf("failed to add tui_config: %v", err)
	}

	return db
}

// TestMigration_LegacyDB_DropCheckConstraint verifies that opening the store
// on a database with the old CHECK(model != ") constraint succeeds and that
// empty models can be inserted afterwards.
func TestMigration_LegacyDB_DropCheckConstraint(t *testing.T) {
	db := newLegacyDB(t)
	ctx := context.Background()

	// Seed a profile with a non-empty model under the legacy schema.
	_, err := db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'claude-acp', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	_, err = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p1', 'a1', 'Claude Sonnet', 'Claude', 'claude-sonnet-4-6', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	// Verify the legacy schema rejects empty model.
	_, err = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p_fail', 'a1', 'Empty', 'Claude', '', datetime('now'), datetime('now'))`)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject empty model on legacy schema")
	}

	// Now open the store — this runs initSchema which should migrate the table.
	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository on legacy DB: %v", err)
	}

	// Existing profile should survive the migration.
	profile, err := repo.GetAgentProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("get existing profile after migration: %v", err)
	}
	if profile.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model %q, got %q", "claude-sonnet-4-6", profile.Model)
	}
	if profile.Name != "Claude Sonnet" {
		t.Errorf("expected name %q, got %q", "Claude Sonnet", profile.Name)
	}

	// Empty model should now be allowed.
	emptyModelProfile := &models.AgentProfile{
		AgentID:          "a1",
		Name:             "Default",
		AgentDisplayName: "Claude",
		Model:            "",
	}
	if err := repo.CreateAgentProfile(ctx, emptyModelProfile); err != nil {
		t.Fatalf("create profile with empty model after migration: %v", err)
	}

	// Verify it round-trips.
	got, err := repo.GetAgentProfile(ctx, emptyModelProfile.ID)
	if err != nil {
		t.Fatalf("get empty-model profile: %v", err)
	}
	if got.Model != "" {
		t.Errorf("expected empty model, got %q", got.Model)
	}
}

// TestMigration_LegacyDB_PreservesAllColumns verifies that mode and
// migrated_from columns (added by ALTERs before the table recreation)
// survive the migration and are readable.
func TestMigration_LegacyDB_PreservesAllColumns(t *testing.T) {
	db := newLegacyDB(t)
	ctx := context.Background()

	_, _ = db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'test-agent', datetime('now'), datetime('now'))`)
	_, _ = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, auto_approve, allow_indexing, cli_passthrough, created_at, updated_at)
		VALUES ('p1', 'a1', 'Test Profile', 'Test', 'some-model', 1, 0, 1, datetime('now'), datetime('now'))`)

	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}

	profile, err := repo.GetAgentProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}

	if profile.Model != "some-model" {
		t.Errorf("model: got %q, want %q", profile.Model, "some-model")
	}
	if !profile.AutoApprove {
		t.Error("auto_approve: got false, want true")
	}
	if profile.AllowIndexing {
		t.Error("allow_indexing: got true, want false")
	}
	if !profile.CLIPassthrough {
		t.Error("cli_passthrough: got false, want true")
	}

	// Update the profile to set mode (new column).
	profile.Mode = "plan"
	if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("update profile with mode: %v", err)
	}
	updated, err := repo.GetAgentProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if updated.Mode != "plan" {
		t.Errorf("mode: got %q, want %q", updated.Mode, "plan")
	}
}

// TestMigration_LegacyDB_MCPConfigSurvives verifies that agent_profile_mcp_configs
// rows (which FK-reference agent_profiles) survive the table recreation.
func TestMigration_LegacyDB_MCPConfigSurvives(t *testing.T) {
	db := newLegacyDB(t)
	ctx := context.Background()

	_, _ = db.Exec(`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1', 'claude-acp', datetime('now'), datetime('now'))`)
	_, _ = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, created_at, updated_at)
		VALUES ('p1', 'a1', 'Claude', 'Claude', 'claude-sonnet-4-6', datetime('now'), datetime('now'))`)
	_, err := db.Exec(`INSERT INTO agent_profile_mcp_configs (profile_id, enabled, servers_json, meta_json, created_at, updated_at)
		VALUES ('p1', 1, '{"test-server":{}}', '{}', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed mcp config: %v", err)
	}

	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}

	cfg, err := repo.GetAgentProfileMcpConfig(ctx, "p1")
	if err != nil {
		t.Fatalf("mcp config missing after migration: %v", err)
	}
	if !cfg.Enabled {
		t.Error("expected mcp config to be enabled after migration")
	}
}

// TestMigration_FreshDB_NoOp verifies that initSchema on a fresh database
// (no legacy CHECK constraint) doesn't error or corrupt the table.
func TestMigration_FreshDB_NoOp(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository on fresh DB: %v", err)
	}

	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agents, err := repo.ListAgents(ctx)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}

// TestMigration_Idempotent verifies that running initSchema twice (simulating
// two backend restarts) doesn't error.
func TestMigration_Idempotent(t *testing.T) {
	db := newLegacyDB(t)

	// First init — runs the migration.
	repo1, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	_ = repo1

	// Second init — migration should detect no CHECK and skip.
	repo2, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("second init (idempotent): %v", err)
	}
	_ = repo2
}
