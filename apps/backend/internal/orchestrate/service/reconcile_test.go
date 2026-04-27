package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestReconcileAgentRuntime_NewAgentCreatesRow(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent in the config (filesystem).
	agent := &models.AgentInstance{
		ID:          "agent-new",
		Name:        "new-worker",
		WorkspaceID: "default",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Run reconciliation.
	r := service.NewReconciler(svc)
	r.ReconcileAll(ctx)

	// The agent should now have status=idle (merged from runtime table).
	got, err := svc.GetAgentInstance(ctx, "agent-new")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status != models.AgentStatusIdle {
		t.Errorf("status = %q, want idle", got.Status)
	}
}

func TestReconcileAgentRuntime_PreservesExistingStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-paused",
		Name:        "paused-worker",
		WorkspaceID: "default",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Transition to paused via UpdateAgentStatus (which persists to runtime table).
	if _, err := svc.UpdateAgentStatus(ctx, "agent-paused", models.AgentStatusPaused, "manual"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	// Run reconciliation.
	r := service.NewReconciler(svc)
	r.ReconcileAll(ctx)

	// Status should still be paused.
	got, err := svc.GetAgentInstance(ctx, "agent-paused")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status != models.AgentStatusPaused {
		t.Errorf("status = %q, want paused", got.Status)
	}
	if got.PauseReason != "manual" {
		t.Errorf("pause_reason = %q, want manual", got.PauseReason)
	}
}

func TestReconcileRoutineTriggers_NewRoutineCreatesTrigger(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := &models.Routine{
		ID:          "routine-new",
		Name:        "daily-check",
		WorkspaceID: "default",
		Status:      "active",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	r := service.NewReconciler(svc)
	r.ReconcileAll(ctx)

	// The routine should have a trigger created in the DB.
	triggers, err := svc.ListRoutineTriggers(ctx, "routine-new")
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
	if triggers[0].Kind != "manual" {
		t.Errorf("trigger kind = %q, want manual", triggers[0].Kind)
	}
}

func TestReconcileBudgetPolicies_DeletesOrphans(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create a budget policy referencing an agent that doesn't exist in config.
	policy := &models.BudgetPolicy{
		WorkspaceID:       "default",
		ScopeType:         "agent",
		ScopeID:           "deleted-agent",
		LimitCents:        1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Verify it exists.
	policies, err := svc.ListBudgetPolicies(ctx, "default")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("before reconcile: count = %d, want 1", len(policies))
	}

	// Reconcile should delete it since "deleted-agent" is not in config.
	r := service.NewReconciler(svc)
	r.ReconcileAll(ctx)

	policies, err = svc.ListBudgetPolicies(ctx, "default")
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("after reconcile: count = %d, want 0", len(policies))
	}
}
