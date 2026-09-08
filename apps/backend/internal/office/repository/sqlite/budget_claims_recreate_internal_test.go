package sqlite

import (
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// TestRecreateBudgetClaimsForRevision_FailureIsVisible covers the other
// half of AC-OFFICE-COSTS-003.3a: a failed recreate must reach the caller
// that initializes the schema, not be logged and swallowed the way the
// neighbouring runMigrations()/activateRunOutcome() calls are.
// failBudgetClaimsRecreateErr is a test-only failpoint (see base.go)
// standing in for a fault-injecting driver; the repo is built by hand
// (mirroring NewWithDB) so the failpoint can be set before initSchema runs,
// while the database still carries the pre-.3 three-column claims table the
// probe needs to find revision absent in.
func TestRecreateBudgetClaimsForRevision_FailureIsVisible(t *testing.T) {
	sqlDB, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := sqlDB.Exec(`
		CREATE TABLE office_budget_policies (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			scope_type TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			limit_subcents INTEGER NOT NULL,
			period TEXT NOT NULL,
			alert_threshold_pct INTEGER DEFAULT 80,
			action_on_exceed TEXT DEFAULT 'notify_only',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE office_budget_claims (
			policy_id TEXT NOT NULL,
			period_key TEXT NOT NULL,
			level TEXT NOT NULL,
			claimed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (policy_id, period_key, level),
			FOREIGN KEY (policy_id) REFERENCES office_budget_policies(id) ON DELETE CASCADE
		);
	`); err != nil {
		t.Fatalf("seed legacy office_budget_claims: %v", err)
	}

	repo := &Repository{
		Repository:                  runssqlite.NewWithDB(sqlDB, sqlDB),
		db:                          sqlDB,
		ro:                          sqlDB,
		migrate:                     db.NewMigrateLogger(sqlDB, nil),
		failBudgetClaimsRecreateErr: errors.New("injected recreate failure"),
	}

	if err := repo.initSchema(); err == nil {
		t.Fatal("expected initSchema to surface the injected recreate failure, got nil error")
	}
}
