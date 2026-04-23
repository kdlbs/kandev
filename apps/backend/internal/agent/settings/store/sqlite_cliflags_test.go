package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func newFreshRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := newSQLiteRepository(db, db, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	return repo
}

// TestCLIFlags_RoundTrip verifies CLIFlags survive insert → read → update → read.
func TestCLIFlags_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "copilot-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "copilot-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "default",
		AgentDisplayName: "Copilot",
		CLIFlags: []models.CLIFlag{
			{Description: "allow all tools", Flag: "--allow-all-tools", Enabled: true},
			{Description: "custom dir", Flag: "--add-dir /shared", Enabled: false},
		},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(got.CLIFlags) != 2 {
		t.Fatalf("cli_flags length mismatch: got %d", len(got.CLIFlags))
	}
	if got.CLIFlags[0].Flag != "--allow-all-tools" || !got.CLIFlags[0].Enabled {
		t.Errorf("first flag mismatch: %+v", got.CLIFlags[0])
	}
	if got.CLIFlags[1].Flag != "--add-dir /shared" || got.CLIFlags[1].Enabled {
		t.Errorf("second flag mismatch: %+v", got.CLIFlags[1])
	}

	// Update: replace list entirely
	got.CLIFlags = []models.CLIFlag{{Flag: "--no-ask-user", Enabled: true}}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got2, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("re-get profile: %v", err)
	}
	if len(got2.CLIFlags) != 1 || got2.CLIFlags[0].Flag != "--no-ask-user" {
		t.Errorf("post-update cli_flags mismatch: %+v", got2.CLIFlags)
	}
}

// TestCLIFlags_LegacyBackfill verifies that a profile row without cli_flags
// (i.e. created before the migration) gets a cli_flags list synthesised from
// the legacy allow_indexing column on first read.
func TestCLIFlags_LegacyBackfill(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "auggie"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "auggie")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	// Simulate a pre-migration row: insert directly with cli_flags = NULL,
	// allow_indexing = 1.
	_, err = repo.db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, mode, auto_approve,
		 dangerously_skip_permissions, allow_indexing, cli_passthrough,
		 user_modified, plan, cli_flags, created_at, updated_at)
		VALUES ('legacy-1', ?, 'Auggie', 'Auggie', '', NULL, 0, 0, 1, 0, 0, '', NULL,
		        datetime('now'), datetime('now'))`, agent.ID)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, "legacy-1")
	if err != nil {
		t.Fatalf("get legacy profile: %v", err)
	}
	if len(got.CLIFlags) != 1 {
		t.Fatalf("expected 1 backfilled flag, got %d: %+v", len(got.CLIFlags), got.CLIFlags)
	}
	if got.CLIFlags[0].Flag != "--allow-indexing" || !got.CLIFlags[0].Enabled {
		t.Errorf("backfilled flag mismatch: %+v", got.CLIFlags[0])
	}
}

// TestCLIFlags_LegacyBackfill_NonAuggieAgent ensures that a legacy profile
// for a non-Auggie agent with allow_indexing=1 (possible via direct SQL or
// the DEFAULT 1 column) does NOT synth a --allow-indexing flag, since only
// Auggie has ever supported that CLI flag.
func TestCLIFlags_LegacyBackfill_NonAuggieAgent(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "claude-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	// Deliberately set allow_indexing=1 on a claude-acp profile — this is
	// the scenario CodeRabbit flagged: a legacy row with the column default
	// on a non-Auggie agent would otherwise inject --allow-indexing into
	// Claude's argv and crash the launch.
	_, err = repo.db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, mode, auto_approve,
		 dangerously_skip_permissions, allow_indexing, cli_passthrough,
		 user_modified, plan, cli_flags, created_at, updated_at)
		VALUES ('legacy-claude', ?, 'Claude', 'Claude', '', NULL, 0, 0, 1, 0, 0, '', NULL,
		        datetime('now'), datetime('now'))`, agent.ID)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, "legacy-claude")
	if err != nil {
		t.Fatalf("get legacy profile: %v", err)
	}
	if len(got.CLIFlags) != 0 {
		t.Errorf("expected no backfilled flags for claude-acp, got %+v", got.CLIFlags)
	}
}

// TestCLIFlags_LegacyBackfill_AllowIndexingOff seeds a legacy row with
// allow_indexing=0 and confirms no flag is backfilled (an off toggle would
// not have produced a CLI flag historically either).
func TestCLIFlags_LegacyBackfill_AllowIndexingOff(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "claude-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	_, err = repo.db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, mode, auto_approve,
		 dangerously_skip_permissions, allow_indexing, cli_passthrough,
		 user_modified, plan, cli_flags, created_at, updated_at)
		VALUES ('legacy-2', ?, 'Claude', 'Claude', '', NULL, 0, 0, 0, 0, 0, '', NULL,
		        datetime('now'), datetime('now'))`, agent.ID)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, "legacy-2")
	if err != nil {
		t.Fatalf("get legacy profile: %v", err)
	}
	if len(got.CLIFlags) != 0 {
		t.Errorf("expected no backfilled flags, got %+v", got.CLIFlags)
	}
}

// TestCLIFlags_EmptyListPersists verifies that a profile explicitly saved
// with an empty list stays empty after a round trip (must not confuse empty
// with "never written" and trigger a backfill).
func TestCLIFlags_EmptyListPersists(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "auggie"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "auggie")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "no flags",
		AgentDisplayName: "Auggie",
		AllowIndexing:    true, // legacy column set — backfill would kick in if the shim ran
		CLIFlags:         []models.CLIFlag{},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.CLIFlags) != 0 {
		t.Errorf("expected empty cli_flags, got %+v", got.CLIFlags)
	}
}
