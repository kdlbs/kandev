package infra_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/infra"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestReconciler creates a Reconciler with an in-memory SQLite repository.
func newTestReconciler(t *testing.T) (*infra.Reconciler, *sqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	r := infra.NewReconciler(repo, log)
	return r, repo
}

func TestReconcileAgentRuntime_NewAgentCreatesRow(t *testing.T) {
	r, repo := newTestReconciler(t)
	ctx := context.Background()

	// Create an agent with idle status.
	agent := &models.AgentInstance{
		ID:          "agent-new",
		Name:        "new-worker",
		WorkspaceID: "default",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Run reconciliation.
	r.ReconcileAll(ctx)

	// The agent should still exist with status=idle (reconcile does not remove
	// rows for agents that still exist in the config table).
	got, err := repo.GetAgentInstance(ctx, "agent-new")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status != models.AgentStatusIdle {
		t.Errorf("status = %q, want idle", got.Status)
	}
}

func TestReconcileAgentRuntime_PreservesExistingStatus(t *testing.T) {
	r, repo := newTestReconciler(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-paused",
		Name:        "paused-worker",
		WorkspaceID: "default",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Transition to paused.
	if err := repo.UpdateAgentStatusFields(ctx, "agent-paused", string(models.AgentStatusPaused), "manual"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	// Run reconciliation.
	r.ReconcileAll(ctx)

	// Status should still be paused.
	got, err := repo.GetAgentInstance(ctx, "agent-paused")
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

func TestReconcileAgentWorkingStatus_ClearsTerminalOwner(t *testing.T) {
	r, repo := newTestReconciler(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-orphaned-working",
		Name:        "orphaned-worker",
		WorkspaceID: "default",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	run := &models.Run{
		ID:             "run-orphaned-working",
		AgentProfileID: agent.ID,
		Reason:         "test",
		Payload:        `{}`,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	claimed, err := repo.ClaimRun(ctx, agent.ID)
	if err != nil {
		t.Fatalf("claim run: %v", err)
	}
	if _, err := repo.MarkAgentWorking(ctx, agent.ID, claimed.ID); err != nil {
		t.Fatalf("mark agent working: %v", err)
	}
	if err := repo.FinishRun(ctx, claimed.ID, "finished", nil); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	r.ReconcileAll(ctx)

	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status != models.AgentStatusIdle {
		t.Fatalf("status = %q, want idle", got.Status)
	}
	var owner string
	if err := repo.ReaderDB().GetContext(ctx, &owner,
		`SELECT working_run_id FROM agent_profiles WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read working owner: %v", err)
	}
	if owner != "" {
		t.Fatalf("working_run_id = %q, want empty", owner)
	}
}

func TestReconcileRoutineTriggers_NewRoutineCreatesTrigger(t *testing.T) {
	r, repo := newTestReconciler(t)
	ctx := context.Background()

	routine := &models.Routine{
		ID:          "routine-new",
		Name:        "daily-check",
		WorkspaceID: "default",
		Status:      "active",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	r.ReconcileAll(ctx)

	// The routine should have a trigger created in the DB.
	triggers, err := repo.ListTriggersByRoutineID(ctx, "routine-new")
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
	r, repo := newTestReconciler(t)
	ctx := context.Background()

	// Create a budget policy referencing an agent that doesn't exist in config.
	policy := &models.BudgetPolicy{
		WorkspaceID:       "default",
		ScopeType:         "agent",
		ScopeID:           "deleted-agent",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if err := repo.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Verify it exists.
	policies, err := repo.ListBudgetPolicies(ctx, "default")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("before reconcile: count = %d, want 1", len(policies))
	}

	// Reconcile should delete it since "deleted-agent" is not in config.
	r.ReconcileAll(ctx)

	policies, err = repo.ListBudgetPolicies(ctx, "default")
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("after reconcile: count = %d, want 0", len(policies))
	}
}
