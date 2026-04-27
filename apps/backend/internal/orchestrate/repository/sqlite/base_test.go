package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
)

// newTestRepo creates an in-memory SQLite repo for testing.
func newTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func TestInitSchema_AllTablesExist(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	expectedTables := []string{
		"orchestrate_agent_runtime",
		"orchestrate_cost_events",
		"orchestrate_budget_policies",
		"orchestrate_wakeup_queue",
		"orchestrate_routines",
		"orchestrate_routine_triggers",
		"orchestrate_routine_runs",
		"orchestrate_approvals",
		"orchestrate_activity_log",
		"orchestrate_agent_memory",
		"orchestrate_channels",
		"task_blockers",
		"task_comments",
	}

	for _, table := range expectedTables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&count)
		if err != nil {
			t.Errorf("query table %s: %v", table, err)
			continue
		}
		if count != 1 {
			t.Errorf("table %s not found (count=%d)", table, count)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create repo twice - should not error on second call
	_, err = sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	_, err = sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
}
