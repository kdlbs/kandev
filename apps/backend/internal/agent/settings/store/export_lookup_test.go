package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// AC-29: the automations YAML export opens one read transaction spanning
// several stores. GetAgentProfileTx is GetAgentProfile's counterpart that
// accepts the caller's *sqlx.Tx instead of opening its own, and reports a
// three-outcome (value, found, err) result so a missing profile is
// distinguishable from a lookup failure without string-matching an error.

// TestGetAgentProfileTx_LegacyBackfillReadsThroughTransaction proves AC-29's
// single-snapshot guarantee holds for the legacy Auggie backfill path too:
// GetAgentProfileTx's nested agent-name lookup (applyLegacyBackfill's
// r.GetAgent call) must read through the caller's *sqlx.Tx, not through
// r.ro directly. If the nested read used r.ro while this tx is still open,
// the connection pool would have to open a second physical connection to a
// bare (non-shared-cache) ":memory:" database to serve it — landing on an
// empty database with no "agents" table at all, since sqlite3's ":memory:"
// DSN gives each connection its own private, independent database. That
// failure mode (either a hard "no such table" error swallowed into an empty
// CLIFlags backfill, or simply the wrong CLIFlags) is exactly what a
// snapshot leak looks like from the caller's side: the profile silently
// loses its legacy --allow-indexing flag depending on which physical
// connection happened to serve the nested read.
func TestGetAgentProfileTx_LegacyBackfillReadsThroughTransaction(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "auggie"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "auggie")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	// A pre-migration row: cli_flags NULL, allow_indexing = 1, matching
	// TestCLIFlags_LegacyBackfill's fixture exactly.
	_, err = repo.db.Exec(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, model, mode, auto_approve,
		 dangerously_skip_permissions, allow_indexing, cli_passthrough,
		 user_modified, plan, cli_flags, created_at, updated_at)
		VALUES ('legacy-tx-1', ?, 'Auggie', 'Auggie', '', NULL, 0, 0, 1, 0, 0, '', NULL,
		        datetime('now'), datetime('now'))`, agent.ID)
	if err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	tx, err := repo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetAgentProfileTx(ctx, tx, "legacy-tx-1")
	if err != nil {
		t.Fatalf("GetAgentProfileTx: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if len(got.CLIFlags) != 1 || got.CLIFlags[0].Flag != "--allow-indexing" || !got.CLIFlags[0].Enabled {
		t.Errorf("CLIFlags = %+v, want the backfilled --allow-indexing flag (nested agent lookup must read through the same tx, not r.ro)", got.CLIFlags)
	}
}

func TestGetAgentProfileTx_FoundAndMissing(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	profile := &models.AgentProfile{AgentID: "agent-1", Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	sqliteRepo, ok := repo.(*sqliteRepository)
	if !ok {
		t.Fatalf("repo is %T, want *sqliteRepository", repo)
	}
	tx, err := sqliteRepo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetAgentProfileTx(ctx, tx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfileTx(%s): %v", profile.ID, err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got == nil || got.ID != profile.ID {
		t.Errorf("profile = %+v, want ID %s", got, profile.ID)
	}

	missing, found, err := repo.GetAgentProfileTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetAgentProfileTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("profile = %+v, want nil", missing)
	}
}
