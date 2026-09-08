package sqlite_test

import (
	"context"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresListStuckRuns is the PostgreSQL twin of the ListStuckRuns cases
// in loop_health_stuck_runs_test.go. ListStuckRuns' stuck_since column is
// computed by a UNION ALL over two different source columns, which loses its
// declared type on read — the original fix normalized it with SQLite-only
// strftime(), a function Postgres does not have, so this exercised the same
// query shape (a claimed-stuck and a queued-stuck row, one list, ordered) end
// to end against a real Postgres backend. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckRuns(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	// Two distinct agent profiles: sharing one would put the queued run
	// behind a claimed sibling on the same agent_profile_id, excluding
	// it under AC-OFFICE-LOOP-LIVENESS-004.17 (a busy agent's steady
	// state, not a stall) — this test wants both rows reported.
	// agent_profiles.agent_id carries a foreign key to agents(id),
	// enforced on Postgres (not on SQLite's default connection).
	for _, agentID := range []string{"agent-a", "agent-b"} {
		if _, err := db.ExecContext(ctx, db.Rebind(
			`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`), agentID, agentID, agentID, agentID, "ws-pg-stuck", "developer", now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}

	claimed := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, claimed); err != nil {
		t.Fatalf("create claimed run: %v", err)
	}
	longAgo := now.Add(-72 * time.Hour)
	if _, err := db.ExecContext(ctx, db.Rebind(
		`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
	), longAgo, claimed.ID); err != nil {
		t.Fatalf("seed claimed state: %v", err)
	}

	queued := &models.Run{AgentProfileID: "agent-b", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, queued); err != nil {
		t.Fatalf("create queued run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	if _, err := db.ExecContext(ctx, db.Rebind(
		`UPDATE runs SET requested_at = ? WHERE id = ?`,
	), old, queued.ID); err != nil {
		t.Fatalf("backdate requested_at: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-pg-stuck", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("ListStuckRuns: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("got %d rows, total %d, want 2/2", len(rows), total)
	}
	// Claimed-stuck outranks queued-stuck regardless of relative age
	// (loop_health.go's ORDER BY (status = 'claimed') DESC ...).
	if rows[0].RunID != claimed.ID || rows[0].Condition != "claimed_stuck" {
		t.Errorf("rows[0] = %+v, want claimed_stuck for %s", rows[0], claimed.ID)
	}
	if rows[0].StuckSince == nil || !rows[0].StuckSince.Equal(longAgo) {
		t.Errorf("rows[0].StuckSince = %v, want %v", rows[0].StuckSince, longAgo)
	}
	if rows[1].RunID != queued.ID || rows[1].Condition != "queued_stuck" {
		t.Errorf("rows[1] = %+v, want queued_stuck for %s", rows[1], queued.ID)
	}
	if rows[1].StuckSince == nil || !rows[1].StuckSince.Equal(old) {
		t.Errorf("rows[1].StuckSince = %v, want %v", rows[1].StuckSince, old)
	}
}
