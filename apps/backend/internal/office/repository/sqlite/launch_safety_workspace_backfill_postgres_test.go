package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresLaunchSafetyWorkspaceBackfill is the PostgreSQL twin of
// TestLaunchSafetyWorkspaceBackfill_RepairsLegacyActiveRuns (RR16-F4). It
// proves the correlated-subquery UPDATE used to repair runs.workspace_id
// (internal/db/dialect independence aside, this statement's syntax must
// itself be valid PostgreSQL) both backfills active rows and replays
// cleanly on a second boot. It skips unless a test DSN is set.
func TestPostgresLaunchSafetyWorkspaceBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init current office schema: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES ('agent-known', 'name', now(), now())
	`); err != nil {
		t.Fatalf("seed agents: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agent_profiles (
			id, agent_id, name, agent_display_name, workspace_id, created_at, updated_at
		) VALUES ('agent-known', 'agent-known', 'name', 'display', 'ws-known', now(), now())
	`); err != nil {
		t.Fatalf("seed agent_profiles: %v", err)
	}

	// Simulate a legacy row: workspace_id already defaulted to '' by the
	// ADD COLUMN migration, as every pre-existing row would be.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, status, workspace_id, requested_at
		) VALUES
			('legacy-queued-known', 'agent-known', 'self', 'queued', '', CURRENT_TIMESTAMP),
			('legacy-succeeded-known', 'agent-known', 'self', 'succeeded', '', CURRENT_TIMESTAMP),
			('legacy-queued-unknown', 'agent-missing', 'self', 'queued', '', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed legacy runs: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	assertRunWorkspaceIDPostgres(t, db, "legacy-queued-known", "ws-known")
	assertRunWorkspaceIDPostgres(t, db, "legacy-succeeded-known", "")
	assertRunWorkspaceIDPostgres(t, db, "legacy-queued-unknown", "")

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	assertRunWorkspaceIDPostgres(t, db, "legacy-queued-known", "ws-known")
	assertRunWorkspaceIDPostgres(t, db, "legacy-succeeded-known", "")
	assertRunWorkspaceIDPostgres(t, db, "legacy-queued-unknown", "")
}

func assertRunWorkspaceIDPostgres(t *testing.T, db *sqlx.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(),
		`SELECT workspace_id FROM runs WHERE id = $1`, runID).Scan(&got); err != nil {
		t.Fatalf("read workspace_id for %q: %v", runID, err)
	}
	if got != want {
		t.Errorf("workspace_id for %q = %q, want %q", runID, got, want)
	}
}
