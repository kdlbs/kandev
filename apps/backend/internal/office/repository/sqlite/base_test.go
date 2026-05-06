package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestRepo creates an in-memory SQLite repo for testing.
//
// Office agent CRUD now reads/writes the unified agent_profiles table
// (ADR 0005 Wave C), so tests must also bring up the settings store
// schema. Both stores share the same writer/reader connections.
func newTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

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
		"office_agent_runtime",
		"office_cost_events",
		"office_budget_policies",
		"runs",
		"office_routines",
		"office_routine_triggers",
		"office_routine_runs",
		"office_approvals",
		"office_activity_log",
		"office_agent_memory",
		"office_channels",
		"task_blockers",
		"task_comments",
		"office_onboarding",
		"office_agent_instructions",
		"office_labels",
		"office_task_labels",
		"task_workspace_groups",
		"task_workspace_group_members",
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
