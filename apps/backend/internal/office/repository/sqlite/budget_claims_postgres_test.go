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

	claimed, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim should win")
	}

	claimed, err = repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("second claim on the same (policy, period, level) must not win")
	}

	// A foreign-key violation (no such policy) must classify as an ordinary
	// miss, not a store error — db.IsForeignKeyViolation's PostgreSQL branch.
	claimed, err = repo.Claim(ctx, uuid.NewString(), "lifetime", "alert")
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
