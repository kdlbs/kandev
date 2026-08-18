package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
)

// TestProvideRepositoriesBackfillsSessionCachedTokens is the composition-level
// regression test for moving task_sessions.tokens_cached_in's backfill
// trigger out of the task repository's own runMigrations() and into this
// package (see the "PR Fixup" plan section on carlosflorencio's PR #2521
// architecture comment). Neither repository's own package tests can see the
// other's construction order, so only a test that boots the real repository
// graph through provideRepositories can prove the backfill still actually
// runs — a regression here would silently resurrect the original card's bug
// (task_sessions.tokens_cached_in drifting from the office_cost_events
// ledger) without any single-package test failing.
func TestProvideRepositoriesBackfillsSessionCachedTokens(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	log := newTestLogger()

	// First boot: creates the schema (both repositories' tables, including
	// office_cost_events) against a real on-disk SQLite file. Then seed a
	// drifted task_sessions row directly - bypassing IncrementTaskSessionUsage
	// - the same way a missed live increment would leave one, plus a ledger
	// row establishing the true value the next boot's backfill must recover.
	pool, _, cleanups, err := provideRepositories(ctx, cfg, log, "test")
	if err != nil {
		t.Fatalf("provideRepositories (first boot): %v", err)
	}
	writer := pool.Writer()
	now := time.Now().UTC()
	if _, err := writer.Exec(writer.Rebind(
		`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, 'test workspace', ?, ?)`),
		"ws-cached-boot", now, now); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := writer.Exec(writer.Rebind(
		`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, 'test', ?, ?)`),
		"task-cached-boot", "ws-cached-boot", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := writer.Exec(writer.Rebind(
		`INSERT INTO task_sessions (id, task_id, started_at, updated_at, tokens_cached_in) VALUES (?, ?, ?, ?, ?)`),
		"sess-cached-boot", "task-cached-boot", now, now, 2_000); err != nil {
		t.Fatalf("seed drifted session: %v", err)
	}
	if _, err := writer.Exec(writer.Rebind(
		`INSERT INTO office_cost_events (id, session_id, tokens_cached_in, occurred_at, created_at) VALUES (?, ?, ?, ?, ?)`),
		"cost-cached-boot", "sess-cached-boot", 98_805_109, now, now); err != nil {
		t.Fatalf("seed ledger row: %v", err)
	}
	for i := len(cleanups) - 1; i >= 0; i-- {
		if cleanups[i] != nil {
			_ = cleanups[i]()
		}
	}

	// Second boot against the same on-disk DB (same cfg.HomeDir, so the same
	// kandev.db file) simulates a restart. The composition-level backfill
	// call in provideRepositories must reconcile the drifted row above.
	pool2, _, cleanups2, err := provideRepositories(ctx, cfg, log, "test")
	if err != nil {
		t.Fatalf("provideRepositories (second boot): %v", err)
	}
	t.Cleanup(func() {
		for i := len(cleanups2) - 1; i >= 0; i-- {
			if cleanups2[i] != nil {
				_ = cleanups2[i]()
			}
		}
	})

	var got int64
	writer2 := pool2.Writer()
	if err := writer2.QueryRowx(writer2.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-cached-boot",
	).Scan(&got); err != nil {
		t.Fatalf("read row after second boot: %v", err)
	}
	if got != 98_805_109 {
		t.Errorf("tokens_cached_in after reboot = %d, want 98805109 (the drifted 2000 must be "+
			"reconciled against the office_cost_events ledger by the composition-level backfill "+
			"call in provideRepositories, after office.Provide succeeds)", got)
	}
}
