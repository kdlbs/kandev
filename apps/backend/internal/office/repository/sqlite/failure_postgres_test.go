package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresHasPriorTasklessFailedRun is the PostgreSQL twin of
// TestHasPriorTasklessFailedRun (WO-35, PR Fixup round 1): HasPriorTasklessFailedRun
// used a raw SQLite-only json_extract(...) expression, which is a syntax error
// on Postgres (payload->>'task_id' is the Postgres form) — see
// internal/db/dialect.JSONExtract. This exercises the same
// prior-taskless-failure/task-scoped/non-failed cases against a real Postgres
// backend so a dialect regression fails loudly instead of only on SQLite CI.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresHasPriorTasklessFailedRun(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// runs is created by the task repository's schema init, mirroring
	// production boot order (see workflow_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	seedPostgresRun(t, ctx, repo, "pg-tl-1", "agent-a", "failed", time.Now().Add(-time.Hour), "{}")
	has, err := repo.HasPriorTasklessFailedRun(ctx, "agent-a", "agent:agent-a", "pg-tl-1")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun: %v", err)
	}
	if has {
		t.Error("has = true, want false — pg-tl-1 is its own only taskless failure")
	}

	seedPostgresRun(t, ctx, repo, "pg-tl-2", "agent-a", "failed", time.Now(), "{}")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-a", "agent:agent-a", "pg-tl-2")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (second): %v", err)
	}
	if !has {
		t.Error("has = false, want true — pg-tl-1 is a prior taskless failure")
	}

	seedPostgresRun(t, ctx, repo, "pg-task", "agent-b", "failed", time.Now().Add(-time.Hour), `{"task_id":"t-1"}`)
	seedPostgresRun(t, ctx, repo, "pg-b", "agent-b", "failed", time.Now(), "{}")
	has, err = repo.HasPriorTasklessFailedRun(ctx, "agent-b", "agent:agent-b", "pg-b")
	if err != nil {
		t.Fatalf("HasPriorTasklessFailedRun (task-scoped prior excluded): %v", err)
	}
	if has {
		t.Error("has = true, want false — the only other failed run for agent-b carries a task_id")
	}
}

// seedPostgresRun inserts a minimal runs row against a real Postgres
// connection. Uses driver-native placeholders since repo.ExecRaw goes
// through the reader/writer sqlx handles that already rebind for the
// connected driver.
func seedPostgresRun(
	t *testing.T, ctx context.Context, repo *sqlite.Repository,
	id, agentID, status string, requestedAt time.Time, payload string,
) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, error_message, requested_at)
		VALUES (?, ?, 'test', ?, ?, '', ?)
	`, id, agentID, payload, status, requestedAt); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}
