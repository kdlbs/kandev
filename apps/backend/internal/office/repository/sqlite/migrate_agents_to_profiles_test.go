package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// legacyOfficeAgentInstancesSchema is the pre-Wave-C schema for the
// office_agent_instances table. The migration tests pre-create this table
// directly so the copy-into-agent_profiles + drop migrations have something
// to act on. Production code never recreates this table.
const legacyOfficeAgentInstancesSchema = `
CREATE TABLE IF NOT EXISTS office_agent_instances (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	name TEXT NOT NULL,
	agent_profile_id TEXT DEFAULT '',
	role TEXT NOT NULL DEFAULT 'worker',
	icon TEXT DEFAULT '',
	status TEXT NOT NULL DEFAULT 'idle',
	reports_to TEXT DEFAULT '',
	permissions TEXT DEFAULT '{}',
	budget_monthly_cents INTEGER DEFAULT 0,
	max_concurrent_sessions INTEGER DEFAULT 1,
	cooldown_sec INTEGER DEFAULT 10,
	skip_idle_runs INTEGER NOT NULL DEFAULT 0,
	last_run_finished_at DATETIME,
	desired_skills TEXT DEFAULT '[]',
	executor_preference TEXT DEFAULT '{}',
	pause_reason TEXT DEFAULT '',
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	failure_threshold INTEGER,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(workspace_id, name)
);
`

// TestMigrateOfficeAgentsToProfiles verifies that the Wave A migration copies
// each office_agent_instances row into the merged agent_profiles table,
// preserving the instance id and combining the office enrichment with the
// CLI fields from the linked legacy profile. It also verifies the Wave-C
// drop migration removes the legacy table afterwards.
func TestMigrateOfficeAgentsToProfiles(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Pin to a single connection so the in-memory schema is shared by
	// every operation. Without this, sqlx's default connection pool will
	// hand out a fresh, empty :memory: DB on later queries.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Initialize the agent_profiles + agents tables via the real settings
	// store so the schema matches production exactly.
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}

	// Seed a "shallow" CLI profile + the parent agent.
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `INSERT INTO agents
		(id, name, supports_mcp, mcp_config_path, created_at, updated_at)
		VALUES ('claude-agent', 'claude', 1, '', datetime('now'), datetime('now'))`)
	_, err = db.ExecContext(ctx, `INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, mode,
		 auto_approve, dangerously_skip_permissions, allow_indexing,
		 cli_passthrough, user_modified, plan, cli_flags, created_at, updated_at)
		VALUES ('cli-profile', 'claude-agent', 'Claude Default', 'Claude',
		        'claude-sonnet-4-6', NULL,
		        0, 0, 1, 0, 0, '', '[]', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed shallow profile: %v", err)
	}

	// Pre-create the legacy office_agent_instances table BEFORE the office
	// repo runs its migrations. Production tables created in older
	// installs would already exist before the binary upgrades.
	if _, err := db.ExecContext(ctx, legacyOfficeAgentInstancesSchema); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	// Seed an office_agent_instances row pointing at the shallow profile.
	_, err = db.ExecContext(ctx, `INSERT INTO office_agent_instances (
		id, workspace_id, name, agent_profile_id,
		role, icon, status, reports_to, permissions,
		budget_monthly_cents, max_concurrent_sessions, cooldown_sec,
		skip_idle_runs, last_run_finished_at, desired_skills,
		executor_preference, pause_reason, consecutive_failures, failure_threshold,
		created_at, updated_at
	) VALUES (
		'inst-ceo', 'ws-1', 'CEO Alice', 'cli-profile',
		'ceo', ':crown:', 'idle', '', '{}',
		5000, 3, 12,
		1, NULL, '["kandev-protocol"]',
		'local_pc', '', 0, 5,
		datetime('now'), datetime('now')
	)`)
	if err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	// Build the office repo. NewWithDB runs runMigrations which calls
	// migrateOfficeAgentsToProfiles (copies row in) THEN
	// dropLegacyOfficeAgentInstances (removes the legacy table).
	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("office repo: %v", err)
	}

	// Idempotency: a second NewWithDB on the post-drop schema must be a
	// no-op (the migration short-circuits when the table is gone).
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("idempotent re-init: %v", err)
	}

	// Verify the legacy table was dropped.
	var legacy int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='office_agent_instances'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if legacy != 0 {
		t.Errorf("legacy office_agent_instances table still present after migration")
	}

	// Verify: agent_profiles now has a row with id == inst-ceo, carrying
	// both the office enrichment AND the CLI fields from cli-profile.
	var count int
	if err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM agent_profiles WHERE id = 'inst-ceo'`); err != nil {
		t.Fatalf("count merged row: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 merged row, got %d", count)
	}

	type row struct {
		AgentID            string `db:"agent_id"`
		Model              string `db:"model"`
		WorkspaceID        string `db:"workspace_id"`
		Role               string `db:"role"`
		Icon               string `db:"icon"`
		Status             string `db:"status"`
		Cooldown           int    `db:"cooldown_sec"`
		MaxSessions        int    `db:"max_concurrent_sessions"`
		Budget             int    `db:"budget_monthly_cents"`
		FailureThreshold   int    `db:"failure_threshold"`
		ExecutorPreference string `db:"executor_preference"`
		DesiredSkills      string `db:"desired_skills"`
		SkillIDs           string `db:"skill_ids"`
		Settings           string `db:"settings"`
		SkipIdleRuns       int    `db:"skip_idle_runs"`
	}
	var got row
	if err := db.GetContext(ctx, &got, `SELECT
		agent_id, model, workspace_id, role, icon, status,
		cooldown_sec, max_concurrent_sessions, budget_monthly_cents,
		failure_threshold, executor_preference, desired_skills, skill_ids,
		settings, skip_idle_runs
		FROM agent_profiles WHERE id = 'inst-ceo'`); err != nil {
		t.Fatalf("read merged row: %v", err)
	}

	if got.AgentID != "claude-agent" || got.Model != "claude-sonnet-4-6" {
		t.Errorf("CLI fields not copied: %+v", got)
	}
	if got.WorkspaceID != "ws-1" || got.Role != "ceo" || got.Icon != ":crown:" {
		t.Errorf("identity fields not copied: %+v", got)
	}
	if got.Cooldown != 12 || got.MaxSessions != 3 || got.Budget != 5000 {
		t.Errorf("knob fields not copied: %+v", got)
	}
	if got.FailureThreshold != 5 || got.ExecutorPreference != "local_pc" {
		t.Errorf("threshold/executor not copied: %+v", got)
	}
	if got.DesiredSkills != `["kandev-protocol"]` {
		t.Errorf("desired_skills not copied: %q", got.DesiredSkills)
	}
	if got.SkillIDs != "[]" {
		t.Errorf("skill_ids should default to []: %q", got.SkillIDs)
	}
	if got.Settings != "{}" {
		t.Errorf("settings should default to {}: %q", got.Settings)
	}
	if got.SkipIdleRuns != 1 {
		t.Errorf("skip_idle_runs not preserved: %d", got.SkipIdleRuns)
	}

	// The original shallow CLI profile must remain intact.
	var shallowExists int
	if err := db.GetContext(ctx, &shallowExists,
		`SELECT COUNT(*) FROM agent_profiles WHERE id = 'cli-profile'`); err != nil {
		t.Fatalf("count shallow: %v", err)
	}
	if shallowExists != 1 {
		t.Fatalf("original shallow profile lost: count=%d", shallowExists)
	}

	_ = repo
}

// TestMigrateOfficeAgentsToProfiles_OrphanedInstance verifies that an instance
// pointing at a missing profile id is silently skipped rather than failing
// the whole migration.
func TestMigrateOfficeAgentsToProfiles_OrphanedInstance(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}

	// Pre-create the legacy table before the office repo runs migrations.
	if _, err := db.Exec(legacyOfficeAgentInstancesSchema); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	// Seed an instance whose agent_profile_id doesn't exist.
	_, err = db.Exec(`INSERT INTO office_agent_instances (
		id, workspace_id, name, agent_profile_id,
		role, icon, status, reports_to, permissions,
		budget_monthly_cents, max_concurrent_sessions, cooldown_sec,
		skip_idle_runs, last_run_finished_at, desired_skills,
		executor_preference, pause_reason, consecutive_failures, failure_threshold,
		created_at, updated_at
	) VALUES (
		'inst-orphan', 'ws-1', 'Orphan', 'missing-profile',
		'worker', '', 'idle', '', '{}',
		0, 1, 0, 0, NULL, '[]', '', '', 0, 3,
		datetime('now'), datetime('now')
	)`)
	if err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	// Run migration via NewWithDB; must not fail.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("init with orphan: %v", err)
	}

	// No agent_profiles row should have been written for the orphan.
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM agent_profiles WHERE id = 'inst-orphan'`).Scan(&count); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if count != 0 {
		t.Errorf("orphan instance was inserted; should have been skipped")
	}
}

// TestMigrateOfficeAgentsToProfiles_NoLegacyTable verifies that on a
// post-drop database (the production state after Wave C ships) the
// migration is a clean no-op. Re-running NewWithDB on a fresh DB that
// never had office_agent_instances must not error.
func TestMigrateOfficeAgentsToProfiles_NoLegacyTable(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}
	// No legacy table created. NewWithDB should succeed and the migration
	// should short-circuit on the table-existence check.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("office repo on fresh DB: %v", err)
	}
	// Second init must remain a no-op.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("idempotent re-init: %v", err)
	}
	// Confirm the table is still absent.
	var legacy int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='office_agent_instances'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if legacy != 0 {
		t.Errorf("office_agent_instances unexpectedly exists on fresh install")
	}
}
