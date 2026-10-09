package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestRetentionIndexes_CreatedFreshAndReplaySafe proves the run-history and
// assignment-wake indexes exist after a fresh boot and that re-running schema
// init against the same database (the upgrade-path replay) is a no-op.
func TestRetentionIndexes_CreatedFreshAndReplaySafe(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := sqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("fresh NewWithDB: %v", err)
	}
	assertIndexExists(t, conn, "idx_office_routine_runs_retention")
	assertIndexExists(t, conn, "idx_runs_retention")
	assertIndexExists(t, conn, "idx_office_agent_pause_recoveries_failed_run")
	assertIndexExists(t, conn, "idx_runs_assignment_rate_reason_requested")

	// Replay: schema init against the same, already-initialized database.
	if _, err := sqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("replay NewWithDB: %v", err)
	}
	assertIndexExists(t, conn, "idx_office_routine_runs_retention")
	assertIndexExists(t, conn, "idx_runs_retention")
	assertIndexExists(t, conn, "idx_office_agent_pause_recoveries_failed_run")
	assertIndexExists(t, conn, "idx_runs_assignment_rate_reason_requested")
}

func assertIndexExists(t *testing.T, conn *sqlx.DB, name string) {
	t.Helper()
	var count int
	if err := conn.Get(&count,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name,
	); err != nil {
		t.Fatalf("query sqlite_master for %s: %v", name, err)
	}
	if count != 1 {
		t.Fatalf("index %s: found %d, want 1", name, count)
	}
}
