package sqlite

// AC-28 coverage: task_sessions.cost_subcents, .tokens_in and .tokens_out
// must be BIGINT on Postgres, mirroring the same 64-bit rule as
// task_usage_events and the already-BIGINT tokens_cached_in. SQLite's
// INTEGER is already 64-bit, so this dialect-sensitive widening is only
// observable against a real Postgres instance.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func postgresColumnDataType(t *testing.T, repo *Repository, table, column string) string {
	t.Helper()
	var dataType string
	err := repo.db.Get(&dataType, `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = $1
		  AND column_name = $2
	`, table, column)
	if err != nil {
		t.Fatalf("inspect %s.%s type: %v", table, column, err)
	}
	return dataType
}

// TestPostgresTaskSessionsRollupColumns_WidenedToBigint proves a fresh
// Postgres database ends up with all three columns as BIGINT after schema
// init runs migrateSessionsAddCostColumns (creates them as INTEGER) followed
// by migrateTaskSessionsRollupColumnsToBigint (widens them).
func TestPostgresTaskSessionsRollupColumns_WidenedToBigint(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	for _, column := range []string{"cost_subcents", "tokens_in", "tokens_out"} {
		if got := postgresColumnDataType(t, repo, "task_sessions", column); got != "bigint" {
			t.Errorf("task_sessions.%s data type = %q, want bigint", column, got)
		}
	}
}

// TestPostgresTaskSessionsRollupColumns_LegacyIntegerSurvivesWidening
// simulates a pre-AC-28 database where these columns are still INTEGER
// (int4), and proves the migration widens them in place without erroring and
// without losing existing data - the same table-rebuild-avoidance shape as
// the priority-to-text migration.
func TestPostgresTaskSessionsRollupColumns_LegacyIntegerSurvivesWidening(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	// Rewind to a pre-AC-28 schema: narrow the columns back to INTEGER, as a
	// legacy database would have them before this migration ever ran.
	for _, column := range []string{"cost_subcents", "tokens_in", "tokens_out"} {
		if _, err := repo.db.Exec(`ALTER TABLE task_sessions ALTER COLUMN ` + column + ` TYPE INTEGER`); err != nil {
			t.Fatalf("rewind %s to integer: %v", column, err)
		}
	}

	seedPostgresTaskSession(t, repo, "task-legacy-widen-pg", "session-legacy-widen-pg")
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_sessions SET cost_subcents = ?, tokens_in = ?, tokens_out = ?
		WHERE id = ?
	`), int64(123_456), int64(789), int64(321), "session-legacy-widen-pg"); err != nil {
		t.Fatalf("seed legacy values: %v", err)
	}

	// Re-run the migration as a new-binary boot would.
	repo.migrateTaskSessionsRollupColumnsToBigint()

	for _, column := range []string{"cost_subcents", "tokens_in", "tokens_out"} {
		if got := postgresColumnDataType(t, repo, "task_sessions", column); got != "bigint" {
			t.Errorf("task_sessions.%s data type after re-migration = %q, want bigint", column, got)
		}
	}

	var costSubcents, tokensIn, tokensOut int64
	if err := repo.db.QueryRowx(repo.db.Rebind(
		`SELECT cost_subcents, tokens_in, tokens_out FROM task_sessions WHERE id = ?`), "session-legacy-widen-pg",
	).Scan(&costSubcents, &tokensIn, &tokensOut); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if costSubcents != 123_456 || tokensIn != 789 || tokensOut != 321 {
		t.Errorf("values after widening = (%d,%d,%d), want (123456,789,321) - existing data must survive",
			costSubcents, tokensIn, tokensOut)
	}

	// The scenario this covers: a value beyond int4's ceiling must now commit
	// where a pre-widening column would reject it with "integer out of range".
	const beyondInt32 = int64(9_000_000_000)
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE task_sessions SET cost_subcents = ? WHERE id = ?`), beyondInt32, "session-legacy-widen-pg",
	); err != nil {
		t.Fatalf("update with a value beyond int32 range must commit on a BIGINT column: %v", err)
	}
}
