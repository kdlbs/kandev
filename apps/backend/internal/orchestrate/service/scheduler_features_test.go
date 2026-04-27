package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// --- Heartbeat cooldown tests ---
// Cooldown enforcement moved from DB query to service layer.
// These tests verify the guard check uses in-memory agent state.

func TestCooldown_RecentFinish_GuardAllows(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-cd-1",
		WorkspaceID: "ws-1",
		Name:        "cooldown-agent",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
		CooldownSec: 5,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Queue a wakeup.
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim should succeed (cooldown is in-memory, not enforced by DB).
	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if wakeup == nil {
		t.Fatal("expected a wakeup, got nil")
	}

	// Guard should allow idle agent.
	ok, err := svc.ProcessWakeupGuard(ctx, wakeup)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Error("guard should allow idle agent")
	}
}

func TestCooldown_PastCooldown_ClaimedNormally(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:          "agent-cd-2",
		WorkspaceID: "ws-1",
		Name:        "cooldown-past-agent",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
		CooldownSec: 5,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Queue a wakeup.
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim should succeed.
	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if wakeup == nil {
		t.Fatal("expected a wakeup, got nil")
	}
	if wakeup.AgentInstanceID != agent.ID {
		t.Errorf("agent = %q, want %q", wakeup.AgentInstanceID, agent.ID)
	}
}

// --- Retry tests ---

func TestRetry_FailedWakeup_RetriedWithBackoff(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("retry-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim the wakeup.
	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if wakeup == nil {
		t.Fatal("expected wakeup")
	}

	// Simulate 4 consecutive failures with retries.
	for i := 0; i < service.MaxRetryCount; i++ {
		wakeup.RetryCount = i
		if err := svc.HandleWakeupFailure(ctx, wakeup, nil); err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}

		// The wakeup should be back to queued with scheduled_retry_at in the future.
		// We need to advance the scheduled time to claim it again.
		svc.ExecSQL(t, `UPDATE orchestrate_wakeup_queue SET scheduled_retry_at = datetime('now', '-1 second') WHERE id = ?`,
			wakeup.ID)

		next, claimErr := svc.ClaimNextWakeup(ctx)
		if claimErr != nil {
			t.Fatalf("claim after retry %d: %v", i, claimErr)
		}
		if next == nil {
			t.Fatalf("expected wakeup after retry %d", i)
		}
		if next.RetryCount != i+1 {
			t.Errorf("retry %d: retry_count = %d, want %d", i, next.RetryCount, i+1)
		}
		wakeup = next
	}
}

func TestRetry_FifthFailure_MarkedFailed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create CEO so escalation path works.
	ceo := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "ceo-retry",
		Role:        models.AgentRoleCEO,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create ceo: %v", err)
	}

	agent := makeAgent("retry-fail-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	wakeup, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Set retry count at max.
	wakeup.RetryCount = service.MaxRetryCount

	testErr := errForTest("agent process crashed")
	if err := svc.HandleWakeupFailure(ctx, wakeup, testErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	// Original wakeup should be marked failed (not claimable).
	next, _ := svc.ClaimNextWakeup(ctx)
	// The next claimable should be the CEO's agent_error wakeup.
	if next == nil {
		t.Fatal("expected CEO agent_error wakeup")
	}
	if next.AgentInstanceID != ceo.ID {
		t.Errorf("agent = %q, want CEO %q", next.AgentInstanceID, ceo.ID)
	}
	if next.Reason != service.WakeupReasonAgentError {
		t.Errorf("reason = %q, want agent_error", next.Reason)
	}
}

// --- Atomic task checkout tests ---

func TestCheckout_TwoAgents_OnlyOneSucceeds(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Insert a task.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-co-1', 'ws-1', 'Checkout Test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	agent1 := &models.AgentInstance{
		ID:          "agent-co-1",
		WorkspaceID: "ws-1",
		Name:        "checkout-agent-1",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	agent2 := &models.AgentInstance{
		ID:          "agent-co-2",
		WorkspaceID: "ws-1",
		Name:        "checkout-agent-2",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent1); err != nil {
		t.Fatalf("create agent1: %v", err)
	}
	if err := svc.CreateAgentInstance(ctx, agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	// First agent checks out.
	ok1, err := svc.CheckoutTask(ctx, "task-co-1", agent1.ID)
	if err != nil {
		t.Fatalf("checkout 1: %v", err)
	}
	if !ok1 {
		t.Fatal("expected first checkout to succeed")
	}

	// Second agent tries to checkout same task.
	ok2, err := svc.CheckoutTask(ctx, "task-co-1", agent2.ID)
	if err != nil {
		t.Fatalf("checkout 2: %v", err)
	}
	if ok2 {
		t.Error("expected second checkout to fail (task locked by agent 1)")
	}

	// Same agent re-checkout should succeed (idempotent).
	ok1Again, err := svc.CheckoutTask(ctx, "task-co-1", agent1.ID)
	if err != nil {
		t.Fatalf("re-checkout: %v", err)
	}
	if !ok1Again {
		t.Error("expected re-checkout by same agent to succeed")
	}
}

func TestCheckout_ReleasedAfterFinish(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-co-2', 'ws-1', 'Release Test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	agent1 := &models.AgentInstance{
		ID:          "agent-rel-1",
		WorkspaceID: "ws-1",
		Name:        "release-agent-1",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	agent2 := &models.AgentInstance{
		ID:          "agent-rel-2",
		WorkspaceID: "ws-1",
		Name:        "release-agent-2",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent1); err != nil {
		t.Fatalf("create agent1: %v", err)
	}
	if err := svc.CreateAgentInstance(ctx, agent2); err != nil {
		t.Fatalf("create agent2: %v", err)
	}

	// Agent 1 checks out.
	ok, err := svc.CheckoutTask(ctx, "task-co-2", agent1.ID)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if !ok {
		t.Fatal("checkout should succeed")
	}

	// Release.
	if err := svc.ReleaseTaskCheckout(ctx, "task-co-2"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Agent 2 can now check out.
	ok2, err := svc.CheckoutTask(ctx, "task-co-2", agent2.ID)
	if err != nil {
		t.Fatalf("checkout after release: %v", err)
	}
	if !ok2 {
		t.Error("expected checkout by agent 2 to succeed after release")
	}
}

// --- Pre-execution budget check tests ---

func TestPreBudget_OverBudget_Blocked(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-budget-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-budget-1",
		LimitCents:        100,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record cost exceeding limit.
	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-budget-1", "",
		"claude-sonnet-4", "anthropic",
		2_000_000, 0, 0,
	)

	allowed, reason, err := svc.CheckPreExecutionBudget(ctx, "agent-budget-1", "", "ws-1")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if allowed {
		t.Error("expected blocked, got allowed")
	}
	if reason == "" {
		t.Error("expected reason, got empty")
	}
}

func TestPreBudget_UnderBudget_Allowed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-budget-2")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-budget-2",
		LimitCents:        10000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record small cost (well under limit).
	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-budget-2", "",
		"claude-sonnet-4", "anthropic",
		10_000, 0, 0,
	)

	allowed, reason, err := svc.CheckPreExecutionBudget(ctx, "agent-budget-2", "", "ws-1")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !allowed {
		t.Errorf("expected allowed, got blocked: %s", reason)
	}
}

// errForTest is a simple error type for testing.
type errForTest string

func (e errForTest) Error() string { return string(e) }
