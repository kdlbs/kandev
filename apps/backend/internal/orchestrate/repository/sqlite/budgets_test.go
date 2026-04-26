package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestBudgetPolicy_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitCents:        50000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetBudgetPolicy(ctx, policy.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LimitCents != 50000 {
		t.Errorf("limit_cents = %d, want 50000", got.LimitCents)
	}

	policies, err := repo.ListBudgetPolicies(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("count = %d, want 1", len(policies))
	}

	policy.LimitCents = 100000
	if err := repo.UpdateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.GetBudgetPolicy(ctx, policy.ID)
	if got.LimitCents != 100000 {
		t.Errorf("updated limit_cents = %d, want 100000", got.LimitCents)
	}

	if err := repo.DeleteBudgetPolicy(ctx, policy.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
