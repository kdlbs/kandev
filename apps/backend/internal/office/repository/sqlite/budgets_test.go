package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newBudgetClaimsRepoWithFK builds an in-memory repo with
// PRAGMA foreign_keys=ON enabled, so the office_budget_claims cascade is
// actually enforced (production always runs with _foreign_keys=on; the
// default newTestRepo does not). Mirrors newRouteAttemptsRepoWithFK.
func newBudgetClaimsRepoWithFK(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("pragma fk: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	// office_task_tree_holds and friends carry an FK to the task-package
	// owned "tasks" table; with foreign_keys=ON that table must exist (even
	// empty) for a workspace-wide DELETE to validate the constraint.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, db
}

func createTestBudgetPolicy(t *testing.T, repo *sqlite.Repository, workspaceID string) *models.BudgetPolicy {
	t.Helper()
	policy := &models.BudgetPolicy{
		WorkspaceID:       workspaceID,
		ScopeType:         "workspace",
		ScopeID:           workspaceID,
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(context.Background(), policy); err != nil {
		t.Fatalf("create budget policy: %v", err)
	}
	return policy
}

func countBudgetClaims(t *testing.T, db *sqlx.DB, policyID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ?`, policyID,
	).Scan(&count); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	return count
}

func TestClaim_FirstCallWinsSecondCallDoesNotReclaim(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-claim")

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
}

func TestClaim_DifferentPeriodOrLevelIsIndependent(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-claim-2")

	if claimed, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert"); err != nil || !claimed {
		t.Fatalf("claim september alert: claimed=%v err=%v", claimed, err)
	}
	// A different period (new window) is unclaimed (AC-OFFICE-COSTS-002.7).
	if claimed, err := repo.Claim(ctx, policy.ID, "2026-10-01T00:00:00Z", "alert"); err != nil || !claimed {
		t.Fatalf("claim october alert: claimed=%v err=%v", claimed, err)
	}
	// A different level in the same period is unclaimed.
	if claimed, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "exceeded"); err != nil || !claimed {
		t.Fatalf("claim september exceeded: claimed=%v err=%v", claimed, err)
	}
}

func TestClaim_ForeignKeyViolationReturnsFalseWithNoError(t *testing.T) {
	repo, _ := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()

	claimed, err := repo.Claim(ctx, uuid.NewString(), "lifetime", "alert")
	if err != nil {
		t.Fatalf("claim against a nonexistent policy must not be a store error, got: %v", err)
	}
	if claimed {
		t.Fatal("claim against a nonexistent policy must not win")
	}
}

func TestDeleteBudgetPolicy_CascadesClaims(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-cascade-1")

	if _, err := repo.Claim(ctx, policy.ID, "lifetime", "alert"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 1 {
		t.Fatalf("claim count before delete = %d, want 1", got)
	}

	if err := repo.DeleteBudgetPolicy(ctx, policy.ID); err != nil {
		t.Fatalf("delete budget policy: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 0 {
		t.Fatalf("claim count after single-policy delete = %d, want 0", got)
	}
}

func TestDeleteWorkspaceData_CascadesBudgetClaims(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-cascade-2")

	if _, err := repo.Claim(ctx, policy.ID, "lifetime", "alert"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := repo.DeleteWorkspaceData(ctx, "ws-cascade-2"); err != nil {
		t.Fatalf("delete workspace data: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 0 {
		t.Fatalf("claim count after workspace delete = %d, want 0", got)
	}
}

func TestDeleteBudgetPoliciesForRemovedScopes_CascadesClaims(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-cascade-3")

	if _, err := repo.Claim(ctx, policy.ID, "lifetime", "alert"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// No valid scope IDs remain, so the reconciliation sweep removes every
	// workspace-scoped policy (mirrors infra.Reconciler.reconcileBudgetPolicies).
	if _, err := repo.DeleteBudgetPoliciesForRemovedScopes(ctx, "workspace", nil); err != nil {
		t.Fatalf("delete for removed scopes: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 0 {
		t.Fatalf("claim count after reconcile delete = %d, want 0", got)
	}
}

func TestUpdateBudgetPolicy_DiscardsClaims(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	policy := createTestBudgetPolicy(t, repo, "ws-update-1")

	if _, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 1 {
		t.Fatalf("claim count before update = %d, want 1", got)
	}

	policy.LimitSubcents = 2000
	if err := repo.UpdateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("update budget policy: %v", err)
	}
	if got := countBudgetClaims(t, db, policy.ID); got != 0 {
		t.Fatalf("claim count after update = %d, want 0 (AC-OFFICE-COSTS-002.8)", got)
	}

	updated, err := repo.GetBudgetPolicy(ctx, policy.ID)
	if err != nil {
		t.Fatalf("get updated policy: %v", err)
	}
	if updated.LimitSubcents != 2000 {
		t.Fatalf("limit_subcents = %d, want 2000", updated.LimitSubcents)
	}
}

func TestListBudgetPolicies_OrdersByCreatedAtThenID(t *testing.T) {
	repo, db := newBudgetClaimsRepoWithFK(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Two policies sharing the same created_at have no defined order from
	// created_at alone; id is the deterministic tiebreak (AC-OFFICE-COSTS-002.16).
	for _, id := range []string{"policy-b", "policy-a"} {
		if _, err := db.Exec(
			`INSERT INTO office_budget_policies (
				id, workspace_id, scope_type, scope_id, limit_subcents, period,
				alert_threshold_pct, action_on_exceed, created_at, updated_at
			) VALUES (?, 'ws-order', 'workspace', 'ws-order', 100, 'monthly', 80, 'notify_only', ?, ?)`,
			id, now, now,
		); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	policies, err := repo.ListBudgetPolicies(ctx, "ws-order")
	if err != nil {
		t.Fatalf("list budget policies: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("policies = %d, want 2", len(policies))
	}
	if policies[0].ID != "policy-a" || policies[1].ID != "policy-b" {
		t.Fatalf("order = [%s, %s], want [policy-a, policy-b]", policies[0].ID, policies[1].ID)
	}
}
