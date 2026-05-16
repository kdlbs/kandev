package store

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestEnrichment_RoundTrip verifies that the office-enrichment fields added in
// ADR 0005 Wave A round-trip through Create / Get / Update / Get cleanly. A
// shallow profile (zero-value enrichment) keeps reading as "" / 0 / nil, and a
// rich profile keeps the values it was written with.
func TestEnrichment_RoundTrip(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}

	ctx := context.Background()
	agent := &models.Agent{Name: "claude"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Shallow profile — enrichment fields default in, do not blow up.
	shallow := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "shallow",
		AgentDisplayName: "Shallow Profile",
		Model:            "claude-sonnet-4-6",
	}
	if err := repo.CreateAgentProfile(ctx, shallow); err != nil {
		t.Fatalf("create shallow: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, shallow.ID)
	if err != nil {
		t.Fatalf("get shallow: %v", err)
	}
	if got.WorkspaceID != "" || got.Role != "" || got.Status != "idle" {
		t.Errorf("shallow defaults wrong: workspace=%q role=%q status=%q",
			got.WorkspaceID, got.Role, got.Status)
	}
	// Wave G: FailureThreshold is *int; nil represents "use workspace
	// default" (matches the office repo's NULLIF semantics).
	if got.MaxConcurrentSessions != 1 || got.FailureThreshold != nil {
		t.Errorf("shallow numeric defaults wrong: max=%d threshold=%v",
			got.MaxConcurrentSessions, got.FailureThreshold)
	}
	// Wave G: SkillIDs / DesiredSkills are JSON-array TEXT columns. Empty
	// values normalise to "[]" so the column's NOT NULL DEFAULT '[]' is
	// satisfied; the JSON tag is omitempty so the kanban API still skips
	// the field for shallow profiles when round-tripping the struct.
	if got.SkillIDs != "[]" || got.DesiredSkills != "[]" {
		t.Errorf("shallow JSON-array defaults wrong: skill_ids=%q desired=%q",
			got.SkillIDs, got.DesiredSkills)
	}
	if got.Settings != "{}" {
		t.Errorf("shallow settings default wrong: %q", got.Settings)
	}
	if got.Permissions != "{}" {
		t.Errorf("shallow permissions default wrong: %q", got.Permissions)
	}

	// Rich profile — every enrichment field set.
	last := time.Now().UTC().Truncate(time.Second)
	threshold := 9
	rich := &models.AgentProfile{
		AgentID:               agent.ID,
		Name:                  "rich",
		AgentDisplayName:      "Rich Profile",
		Model:                 "claude-sonnet-4-6",
		WorkspaceID:           "ws-1",
		Role:                  models.AgentRoleCEO,
		Icon:                  ":crown:",
		ReportsTo:             "boss-id",
		SkillIDs:              `["sk1","sk2"]`,
		DesiredSkills:         `["kandev-protocol"]`,
		CustomPrompt:          "follow the kandev protocol",
		Status:                models.AgentStatusWorking,
		PauseReason:           "",
		LastRunFinishedAt:     &last,
		MaxConcurrentSessions: 5,
		CooldownSec:           42,
		SkipIdleRuns:          true,
		ConsecutiveFailures:   1,
		FailureThreshold:      &threshold,
		ExecutorPreference:    "local_docker",
		BudgetMonthlyCents:    9999,
		Settings:              `{"k":"v"}`,
		Permissions:           `{"can_hire":true}`,
	}
	if err := repo.CreateAgentProfile(ctx, rich); err != nil {
		t.Fatalf("create rich: %v", err)
	}
	got, err = repo.GetAgentProfile(ctx, rich.ID)
	if err != nil {
		t.Fatalf("get rich: %v", err)
	}
	if got.WorkspaceID != "ws-1" || got.Role != models.AgentRoleCEO || got.Icon != ":crown:" {
		t.Errorf("rich identity wrong: %+v", got)
	}
	if got.Status != models.AgentStatusWorking || got.MaxConcurrentSessions != 5 || got.CooldownSec != 42 {
		t.Errorf("rich runtime wrong: status=%q max=%d cooldown=%d",
			got.Status, got.MaxConcurrentSessions, got.CooldownSec)
	}
	if !got.SkipIdleRuns || got.FailureThreshold == nil || *got.FailureThreshold != 9 || got.BudgetMonthlyCents != 9999 {
		t.Errorf("rich knobs wrong: skip_idle=%v threshold=%v budget=%d",
			got.SkipIdleRuns, got.FailureThreshold, got.BudgetMonthlyCents)
	}
	if got.SkillIDs != `["sk1","sk2"]` {
		t.Errorf("skill_ids wrong: %q", got.SkillIDs)
	}
	if got.DesiredSkills != `["kandev-protocol"]` {
		t.Errorf("desired_skills wrong: %q", got.DesiredSkills)
	}
	if got.LastRunFinishedAt == nil || !got.LastRunFinishedAt.Equal(last) {
		t.Errorf("last_run_finished_at lost: got=%v want=%v", got.LastRunFinishedAt, last)
	}
	if got.Settings != `{"k":"v"}` {
		t.Errorf("settings round-trip lost: %q", got.Settings)
	}
	if got.Permissions != `{"can_hire":true}` {
		t.Errorf("permissions round-trip lost: %q", got.Permissions)
	}

	// Update path — flip a few fields and read back.
	got.Status = models.AgentStatusPaused
	got.PauseReason = "auto"
	got.SkillIDs = `["sk-only"]`
	got.DesiredSkills = ""
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	roundtripped, err := repo.GetAgentProfile(ctx, rich.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if roundtripped.Status != models.AgentStatusPaused || roundtripped.PauseReason != "auto" {
		t.Errorf("update lost: %+v", roundtripped)
	}
	if roundtripped.SkillIDs != `["sk-only"]` {
		t.Errorf("update skill_ids wrong: %q", roundtripped.SkillIDs)
	}
	// Empty in → "[]" out (the NOT NULL default at the SQL layer).
	if roundtripped.DesiredSkills != "[]" {
		t.Errorf("update desired_skills should normalise to []: %q", roundtripped.DesiredSkills)
	}
}

// TestEnrichment_Migration_LegacyDB verifies that adding the new enrichment
// columns on an existing database with the legacy CHECK(model != ”)
// constraint does not regress existing reads. The legacy DB has rows that
// pre-date the new columns; after migration, those rows must read with the
// documented defaults.
func TestEnrichment_Migration_LegacyDB(t *testing.T) {
	db := newLegacyDB(t)

	// Seed an agent + profile in the legacy schema.
	_, err := db.Exec(`INSERT INTO agents (id, name, supports_mcp, mcp_config_path, created_at, updated_at)
		VALUES ('a1', 'claude', 0, '', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	_, err = db.Exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, user_modified, plan, created_at, updated_at)
		VALUES ('p1', 'a1', 'Legacy', 'Legacy', 'claude', 0, 0, 1, 0, 0, '', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("init schema: %v", err)
	}

	got, err := repo.GetAgentProfile(context.Background(), "p1")
	if err != nil {
		t.Fatalf("get legacy profile: %v", err)
	}
	if got.WorkspaceID != "" || got.Role != "" {
		t.Errorf("legacy enrichment defaults wrong: %+v", got)
	}
	// Legacy DB rows ALTER-ADDed the failure_threshold column with DEFAULT 3,
	// so they read back as *int(3) rather than nil. New shallow profiles
	// created via the repo write 0 (the "use workspace default" sentinel) and
	// read back as nil — that path is exercised in TestEnrichment_RoundTrip.
	if got.Status != "idle" || got.MaxConcurrentSessions != 1 {
		t.Errorf("legacy default values wrong: status=%q max=%d",
			got.Status, got.MaxConcurrentSessions)
	}
	if got.FailureThreshold == nil || *got.FailureThreshold != 3 {
		t.Errorf("legacy threshold default wrong: %v", got.FailureThreshold)
	}
	if got.Settings != "{}" {
		t.Errorf("legacy settings default wrong: %q", got.Settings)
	}
	if got.Permissions != "{}" {
		t.Errorf("legacy permissions default wrong: %q", got.Permissions)
	}
}
