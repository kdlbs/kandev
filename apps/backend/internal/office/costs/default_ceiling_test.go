package costs_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
)

func TestGetWorkspaceBudgetDefault_NoWriteReturnsShippedConstant(t *testing.T) {
	svc, _, _ := newBudgetTestService(t)
	ctx := context.Background()

	got, err := svc.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if got != costs.DefaultCeilingSubcents {
		t.Errorf("got = %d, want the shipped constant %d", got, costs.DefaultCeilingSubcents)
	}
}

func TestSetWorkspaceBudgetDefault_ThenGetReturnsWrittenValue(t *testing.T) {
	svc, _, _ := newBudgetTestService(t)
	ctx := context.Background()

	if err := svc.SetWorkspaceBudgetDefault(ctx, "ws-1", 750_000); err != nil {
		t.Fatalf("SetWorkspaceBudgetDefault: %v", err)
	}

	got, err := svc.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if got != 750_000 {
		t.Errorf("got = %d, want 750000", got)
	}
}

// TestSetWorkspaceBudgetDefault_RejectsNonPositiveLimit pins
// AC-OFFICE-BUDGET-003.8: a non-positive write is rejected and the previous
// effective value stays in force.
func TestSetWorkspaceBudgetDefault_RejectsNonPositiveLimit(t *testing.T) {
	svc, _, _ := newBudgetTestService(t)
	ctx := context.Background()

	if err := svc.SetWorkspaceBudgetDefault(ctx, "ws-1", 300_000); err != nil {
		t.Fatalf("initial SetWorkspaceBudgetDefault: %v", err)
	}

	for _, bad := range []int64{0, -1} {
		if err := svc.SetWorkspaceBudgetDefault(ctx, "ws-1", bad); err == nil {
			t.Errorf("SetWorkspaceBudgetDefault(%d) = nil error, want a validation error", bad)
		}
	}

	got, err := svc.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if got != 300_000 {
		t.Errorf("got = %d, want the previous value 300000 left in force", got)
	}
}

func TestEvaluateDefaultCeiling_UnderLimit_Admits(t *testing.T) {
	svc, repo, _ := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 100, at.Add(-time.Hour), nil)

	got, err := svc.EvaluateDefaultCeiling(ctx, "ws-1", at)
	if err != nil {
		t.Fatalf("EvaluateDefaultCeiling: %v", err)
	}
	if got.LimitExceeded {
		t.Error("LimitExceeded = true, want false: spend is far under the 500000-subcent default")
	}
	if !got.IsDefault {
		t.Error("IsDefault = false, want true")
	}
	if got.PolicyID != "" {
		t.Errorf("PolicyID = %q, want empty: the default is not a office_budget_policies row", got.PolicyID)
	}
	if got.LimitSubcents != costs.DefaultCeilingSubcents {
		t.Errorf("LimitSubcents = %d, want the shipped constant %d", got.LimitSubcents, costs.DefaultCeilingSubcents)
	}
}

func TestEvaluateDefaultCeiling_AtLimit_Blocks(t *testing.T) {
	svc, repo, _ := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", costs.DefaultCeilingSubcents, at.Add(-time.Hour), nil)

	got, err := svc.EvaluateDefaultCeiling(ctx, "ws-1", at)
	if err != nil {
		t.Fatalf("EvaluateDefaultCeiling: %v", err)
	}
	if !got.LimitExceeded {
		t.Error("LimitExceeded = false, want true: spend reached the default ceiling")
	}
}

// TestEvaluateDefaultCeiling_UsesWrittenLimit pins AC-OFFICE-BUDGET-003.5/.7:
// the operator-writable limit, not the shipped constant, is what gets
// evaluated once it has been written.
func TestEvaluateDefaultCeiling_UsesWrittenLimit(t *testing.T) {
	svc, repo, _ := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1_500, at.Add(-time.Hour), nil)

	if err := svc.SetWorkspaceBudgetDefault(ctx, "ws-1", 1_000); err != nil {
		t.Fatalf("SetWorkspaceBudgetDefault: %v", err)
	}

	got, err := svc.EvaluateDefaultCeiling(ctx, "ws-1", at)
	if err != nil {
		t.Fatalf("EvaluateDefaultCeiling: %v", err)
	}
	if !got.LimitExceeded {
		t.Error("LimitExceeded = false, want true: spend exceeds the written 1000-subcent limit")
	}
	if got.LimitSubcents != 1_000 {
		t.Errorf("LimitSubcents = %d, want the written value 1000", got.LimitSubcents)
	}
}

// TestEvaluateDefaultCeiling_PricingDegradedBlocks pins AC-OFFICE-BUDGET-
// 003.11: the default is evaluated by the same pricing-degradation
// criterion as a policy.
func TestEvaluateDefaultCeiling_PricingDegradedBlocks(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	_ = execSQL
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 400, at.Add(-time.Hour), nil)
	unpriced := models.CostSourceUnpriced
	mustCreateCostEvent(t, repo, "agent-1", "", 0, at.Add(-30*time.Minute), &unpriced)

	if err := svc.SetWorkspaceBudgetDefault(ctx, "ws-1", 700); err != nil {
		t.Fatalf("SetWorkspaceBudgetDefault: %v", err)
	}

	got, err := svc.EvaluateDefaultCeiling(ctx, "ws-1", at)
	if err != nil {
		t.Fatalf("EvaluateDefaultCeiling: %v", err)
	}
	if got.LimitExceeded {
		t.Error("LimitExceeded = true, want false: priced spend alone is under the limit")
	}
	if !got.DegradationBlocked {
		t.Error("DegradationBlocked = false, want true: 2*400 >= 700 with an unpriced event present")
	}
}
