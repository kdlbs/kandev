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

// budgetClaimsHasRevisionColumn reports whether office_budget_claims
// currently declares a revision column, distinguishing the old three-column
// shape from the final four-column one.
func budgetClaimsHasRevisionColumn(t *testing.T, sqlDB *sqlx.DB) bool {
	t.Helper()
	var count int
	if err := sqlDB.Get(&count,
		`SELECT COUNT(*) FROM pragma_table_info('office_budget_claims') WHERE name = 'revision'`,
	); err != nil {
		t.Fatalf("probe office_budget_claims columns: %v", err)
	}
	return count == 1
}

// TestRecreateBudgetClaimsForRevision_AtomicAcrossDropAndCreate covers the
// crash-safety half of AC-OFFICE-COSTS-003.3a: DROP and CREATE must commit
// or fail together, not as two independent statements. A boot interrupted
// between them must not leave office_budget_claims absent, because a
// missing table and an old-shape table both probe as "revision absent" and
// an unconditional DROP TABLE (no IF EXISTS) on an already-missing table
// would error forever on every later boot. failBudgetClaimsRecreateAfterDropErr
// (see base.go) fires after DROP has run but before CREATE, inside the same
// transaction, standing in for a process crash at that point.
func TestRecreateBudgetClaimsForRevision_AtomicAcrossDropAndCreate(t *testing.T) {
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
		Repository: runssqlite.NewWithDB(sqlDB, sqlDB),
		db:         sqlDB,
		ro:         sqlDB,
		migrate:    db.NewMigrateLogger(sqlDB, nil),
	}

	repo.failBudgetClaimsRecreateAfterDropErr = errors.New("injected crash between drop and create")
	if err := repo.initSchema(); err == nil {
		t.Fatal("expected initSchema to surface the injected failure, got nil error")
	}
	if budgetClaimsHasRevisionColumn(t, sqlDB) {
		t.Fatal("a failure between DROP and CREATE must not leave the new shape partially applied")
	}
	var count int
	if err := sqlDB.Get(&count,
		`SELECT COUNT(*) FROM pragma_table_info('office_budget_claims')`,
	); err != nil {
		t.Fatalf("count office_budget_claims columns: %v", err)
	}
	if count == 0 {
		t.Fatal("a failure between DROP and CREATE must not leave office_budget_claims absent; the DROP must roll back too")
	}

	repo.failBudgetClaimsRecreateAfterDropErr = nil
	if err := repo.initSchema(); err != nil {
		t.Fatalf("retry boot after the injected failure must self-heal, got: %v", err)
	}
	if !budgetClaimsHasRevisionColumn(t, sqlDB) {
		t.Fatal("retry boot must complete the recreate and leave the revision column present")
	}
}
