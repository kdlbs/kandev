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

// TestClaimExceeded_CompanionFailure_RollsBackWholePair proves the
// exceeded-level claim and its alert-level companion are one transaction
// (AC-OFFICE-COSTS-003.10): when the companion insert fails after the
// exceeded insert has already run, the whole transaction rolls back, so
// neither row survives. failBudgetExceededCompanionErr is a test-only
// failpoint (see base.go) standing in for a fault-injecting driver.
func TestClaimExceeded_CompanionFailure_RollsBackWholePair(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
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
		WorkspaceID:       "ws-companion-rollback",
		ScopeType:         "workspace",
		ScopeID:           "ws-companion-rollback",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	repo.failBudgetExceededCompanionErr = errors.New("injected companion failure")
	claimed, err := repo.ClaimExceeded(ctx, policy.ID, "lifetime", policy.Revision)
	if err == nil {
		t.Fatal("expected the injected companion failure to surface")
	}
	if claimed {
		t.Fatal("a failed pair must report claimed=false")
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ?`, policy.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if count != 0 {
		t.Fatalf("claim rows after a rolled-back pair = %d, want 0 (neither row must survive)", count)
	}
}
