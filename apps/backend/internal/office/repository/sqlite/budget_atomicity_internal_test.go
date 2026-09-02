package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
)

// TestUpdateBudgetPolicy_FailedUpdateRollsBackDiscard proves the claim
// discard and the policy row update are one transaction
// (AC-OFFICE-COSTS-002.8): when the row update fails after the discard has
// already run, the whole transaction rolls back, so the claim survives
// alongside the un-updated policy. failBudgetPolicyUpdateErr is a
// test-only failpoint (see base.go) standing in for a fault-injecting
// driver.
func TestUpdateBudgetPolicy_FailedUpdateRollsBackDiscard(t *testing.T) {
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
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-rollback",
		ScopeType:         "workspace",
		ScopeID:           "ws-rollback",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := repo.Claim(ctx, policy.ID, "2026-09-01T00:00:00Z", "alert"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	repo.failBudgetPolicyUpdateErr = errors.New("injected update failure")
	policy.LimitSubcents = 5000
	if err := repo.UpdateBudgetPolicy(ctx, policy); err == nil {
		t.Fatal("expected the injected update failure to surface")
	}

	var claimCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ?`, policy.ID,
	).Scan(&claimCount); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claimCount != 1 {
		t.Fatalf("claim count after rolled-back update = %d, want 1 (discard must roll back with the failed update)", claimCount)
	}

	stored, err := repo.GetBudgetPolicy(ctx, policy.ID)
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if stored.LimitSubcents != 1000 {
		t.Fatalf("limit_subcents after rolled-back update = %d, want unchanged 1000", stored.LimitSubcents)
	}
}
