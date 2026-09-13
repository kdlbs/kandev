package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresBudgetClaims is the PostgreSQL twin of the office_budget_claims
// coverage in budgets_test.go (REQ-OFFICE-COSTS-002, ADR 0027): office_budget_claims
// carries a forward foreign key to office_budget_policies, which SQLite tolerates at
// CREATE TABLE time but PostgreSQL rejects, so the two tables' creation order is a
// dialect-sensitive contract; Claim's INSERT ... ON CONFLICT DO NOTHING and the
// db.IsForeignKeyViolation classifier it relies on are dialect-sensitive too. This
// proves fresh-boot, boot replay, claim win/lose, the foreign-key-violation miss, and
// cascade delete all hold on a real PostgreSQL backend. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresBudgetClaims(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// office_budget_policies (and thus office_budget_claims' FK target) has no
	// dependency of its own on the task repository, but office repo init as a
	// whole does, mirroring production boot order (see cost_event_contract_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// First boot: office_budget_claims must be created after office_budget_policies
	// so its forward FK reference resolves on Postgres.
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	// Second boot: the schema must replay without error.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}

	policy := &models.BudgetPolicy{
		WorkspaceID:       "pg-budget-ws",
		ScopeType:         "workspace",
		ScopeID:           "pg-budget-ws",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create budget policy: %v", err)
	}

	claimed, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert", policy.Revision)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim should win")
	}

	claimed, err = repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert", policy.Revision)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("second claim on the same (policy, period, level) must not win")
	}

	// A claim for a nonexistent policy must be an ordinary fenced miss, not a
	// store error. The INSERT ... SELECT fence prevents an FK violation here.
	claimed, err = repo.Claim(ctx, uuid.NewString(), "lifetime", "alert", 1)
	if err != nil {
		t.Fatalf("claim against a nonexistent policy must not be a store error, got: %v", err)
	}
	if claimed {
		t.Fatal("claim against a nonexistent policy must not win")
	}

	if err := repo.DeleteBudgetPolicy(ctx, policy.ID); err != nil {
		t.Fatalf("delete budget policy: %v", err)
	}
	var claimCount int
	if err := db.GetContext(ctx, &claimCount,
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = $1`, policy.ID,
	); err != nil {
		t.Fatalf("count claims after delete: %v", err)
	}
	if claimCount != 0 {
		t.Fatalf("claim count after policy delete = %d, want 0 (FK ON DELETE CASCADE)", claimCount)
	}
}

// TestPostgresBudgetClaims_RevisionFencingAndFourColumnKey is the PostgreSQL
// twin of the revision-fencing coverage in budget_revision_fencing_test.go:
// under READ COMMITTED a claim's WHERE EXISTS fence check and the row it
// inserts both need proving on a real PostgreSQL backend, not just SQLite's
// single-writer connection. It covers both halves called out by the fence
// design: the fence itself refuses an insert against a superseded revision
// (a stale evaluation cannot emit an obsolete claim), and revision being
// part of the primary key means a claim row already recorded for an old
// revision does not collide with — and so cannot suppress — a fresh
// evaluation's claim at the new revision. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresBudgetClaims_RevisionFencingAndFourColumnKey(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	policy := &models.BudgetPolicy{
		WorkspaceID:       "pg-budget-fencing-ws",
		ScopeType:         "workspace",
		ScopeID:           "pg-budget-fencing-ws",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create budget policy: %v", err)
	}
	staleRevision := policy.Revision

	// A claim recorded at the current revision, before any update.
	claimed, err := repo.Claim(ctx, policy.ID, "lifetime", "alert", staleRevision)
	if err != nil {
		t.Fatalf("claim at revision %d: %v", staleRevision, err)
	}
	if !claimed {
		t.Fatal("claim at the current revision should win")
	}

	if err := repo.UpdateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if policy.Revision != staleRevision+1 {
		t.Fatalf("revision after update = %d, want %d", policy.Revision, staleRevision+1)
	}

	// The fence: a claim presented with the now-superseded revision must be
	// refused, even though it targets a different period so it collides
	// with nothing already in the table.
	claimed, err = repo.Claim(ctx, policy.ID, "stale-period", "alert", staleRevision)
	if err != nil {
		t.Fatalf("stale-revision claim must be an ordinary miss, got error: %v", err)
	}
	if claimed {
		t.Fatal("a claim presented with a superseded revision must not win")
	}

	// A surviving old-revision row must not suppress a fresh claim. The
	// direct insert models a claim that was already committed by an older
	// evaluation while the policy update moved the current revision.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO office_budget_claims (policy_id, period_key, level, revision, claimed_at)
		VALUES ($1, 'lifetime', 'alert', $2, NOW())
	`, policy.ID, staleRevision); err != nil {
		t.Fatalf("seed surviving old-revision claim: %v", err)
	}

	// The four-column key lets the current revision claim the same period and
	// level without colliding with that old-revision row.
	claimed, err = repo.Claim(ctx, policy.ID, "lifetime", "alert", policy.Revision)
	if err != nil {
		t.Fatalf("claim at the new revision: %v", err)
	}
	if !claimed {
		t.Fatal("claim at the current (post-update) revision should win")
	}

	var claimCount int
	if err := db.GetContext(ctx, &claimCount,
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = $1`, policy.ID,
	); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claimCount != 2 {
		t.Fatalf("claim rows for policy = %d, want 2 (old and current revisions must coexist)", claimCount)
	}
}

// TestPostgresBudgetClaims_LegacyTableRecreated verifies the destructive
// legacy-table cutover on a real PostgreSQL backend. The old three-column
// table is replaced in one transaction, and a later boot must replay without
// replacing the new table again. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresBudgetClaims_LegacyTableRecreated(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initial boot: %v", err)
	}

	policy := &models.BudgetPolicy{
		WorkspaceID:       "pg-budget-legacy-ws",
		ScopeType:         "workspace",
		ScopeID:           "pg-budget-legacy-ws",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create budget policy: %v", err)
	}

	// Replace the current table with the pre-revision three-column shape and
	// leave a row in it. The next office boot must take the recreate path.
	if _, err := db.ExecContext(ctx, `DROP TABLE office_budget_claims`); err != nil {
		t.Fatalf("drop current claims table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE office_budget_claims (
			policy_id TEXT NOT NULL,
			period_key TEXT NOT NULL,
			level TEXT NOT NULL,
			claimed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (policy_id, period_key, level),
			FOREIGN KEY (policy_id) REFERENCES office_budget_policies(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create legacy claims table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO office_budget_claims (policy_id, period_key, level, claimed_at)
		VALUES ($1, 'legacy', 'alert', NOW())
	`, policy.ID); err != nil {
		t.Fatalf("seed legacy claim: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("boot that recreates legacy claims table: %v", err)
	}

	var revisionColumns int
	if err := db.GetContext(ctx, &revisionColumns, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'office_budget_claims'
		  AND column_name = 'revision'
	`); err != nil {
		t.Fatalf("probe recreated claims table: %v", err)
	}
	if revisionColumns != 1 {
		t.Fatalf("office_budget_claims.revision columns = %d, want 1", revisionColumns)
	}

	claimed, err := repo.Claim(ctx, policy.ID, "replayed", "alert", policy.Revision)
	if err != nil {
		t.Fatalf("claim after legacy recreate: %v", err)
	}
	if !claimed {
		t.Fatal("claim after legacy recreate should win")
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay boot after recreate: %v", err)
	}
	var claimCount int
	if err := db.GetContext(ctx, &claimCount,
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = $1`, policy.ID,
	); err != nil {
		t.Fatalf("count claims after replay: %v", err)
	}
	if claimCount != 1 {
		t.Fatalf("claim rows after replay = %d, want 1", claimCount)
	}
}
