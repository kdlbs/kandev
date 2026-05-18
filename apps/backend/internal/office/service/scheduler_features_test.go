package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
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

	// Queue a run.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim should succeed (cooldown is in-memory, not enforced by DB).
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run, got nil")
	}

	// Guard should allow idle agent.
	ok, err := svc.ProcessRunGuard(ctx, run)
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

	// Queue a run.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim should succeed.
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run, got nil")
	}
	if run.AgentProfileID != agent.ID {
		t.Errorf("agent = %q, want %q", run.AgentProfileID, agent.ID)
	}
}

// --- Retry tests ---

func TestRetry_FailedRun_RetriedWithBackoff(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("retry-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Claim the run.
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected run")
	}

	// Simulate 4 consecutive failures with retries.
	for i := 0; i < service.MaxRetryCount; i++ {
		run.RetryCount = i
		if err := svc.HandleRunFailure(ctx, run, nil); err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}

		// The run should be back to queued with scheduled_retry_at in the future.
		// We need to advance the scheduled time to claim it again.
		svc.ExecSQL(t, `UPDATE runs SET scheduled_retry_at = datetime('now', '-1 second') WHERE id = ?`,
			run.ID)

		next, claimErr := svc.ClaimNextRun(ctx)
		if claimErr != nil {
			t.Fatalf("claim after retry %d: %v", i, claimErr)
		}
		if next == nil {
			t.Fatalf("expected run after retry %d", i)
		}
		if next.RetryCount != i+1 {
			t.Errorf("retry %d: retry_count = %d, want %d", i, next.RetryCount, i+1)
		}
		run = next
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

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Set retry count at max.
	run.RetryCount = service.MaxRetryCount

	testErr := errForTest("agent process crashed")
	if err := svc.HandleRunFailure(ctx, run, testErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	// Original run should be marked failed (not claimable).
	next, _ := svc.ClaimNextRun(ctx)
	// The next claimable should be the CEO's agent_error run.
	if next == nil {
		t.Fatal("expected CEO agent_error run")
	}
	if next.AgentProfileID != ceo.ID {
		t.Errorf("agent = %q, want CEO %q", next.AgentProfileID, ceo.ID)
	}
	if next.Reason != service.RunReasonAgentError {
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
		LimitSubcents:     100,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record cost exceeding limit (600 cents > 100 cent limit).
	insertTestTask(t, svc, "task-1", "ws-1")
	insertTestCostEvent(t, svc, "agent-budget-1", "task-1", int64(600))

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
		LimitSubcents:     10000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record small cost (3 cents, well under 10000 cent limit).
	insertTestTask(t, svc, "task-1", "ws-1")
	insertTestCostEvent(t, svc, "agent-budget-2", "task-1", int64(3))

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

// --- Idle run skip tests ---

// insertActionableTask inserts a task in a given state assigned to an agent.
// ADR 0005 Wave F: assignee is recorded as a runner participant.
func insertActionableTask(t *testing.T, svc *service.Service, taskID, agentID, state string) {
	t.Helper()
	svc.ExecSQL(t,
		`INSERT OR IGNORE INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		 VALUES (?, 'ws-1', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		taskID, state, taskID)
	setTestTaskAssignee(t, svc, taskID, agentID)
}

func TestIdleSkip_HeartbeatNoTasks_Skipped(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-skip",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Worker defaults to skip_idle_runs=true, no tasks assigned.

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// No session should have been launched.
	if mock.callCount() != 0 {
		t.Errorf("expected 0 StartTask calls, got %d", mock.callCount())
	}

	// Queue should be empty (run was finished).
	next, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim after tick: %v", err)
	}
	if next != nil {
		t.Error("expected queue to be empty after idle skip")
	}

	// Activity log should have a run_idle_skipped entry.
	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected run_idle_skipped activity entry")
	}
}

func TestIdleSkip_HeartbeatWithActionableTasks_Proceeds(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-has-tasks",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Assign an IN_PROGRESS task to this agent.
	insertActionableTask(t, svc, "task-inprog-1", agent.ID, "IN_PROGRESS")

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// Run should have proceeded to launch (mock called or at least not skipped).
	// Since there is no task_id in the payload the scheduler logs but does not
	// call StartTask (launchOrLog path). Verify the run was finished normally.
	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			t.Error("run should not have been idle-skipped (agent has actionable tasks)")
		}
	}
}

func TestIdleSkip_HeartbeatSkipDisabled_Proceeds(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-skip-off",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
		SkipIdleRuns:       false,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Explicitly disable skip by patching after creation (creation sets true for workers).
	svc.ExecSQL(t,
		`UPDATE agent_profiles SET skip_idle_runs = 0 WHERE id = ?`, agent.ID)

	// No tasks assigned.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			t.Error("run should not be idle-skipped when skip_idle_runs=false")
		}
	}
}

func TestIdleSkip_NonHeartbeatRun_NotSkipped(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-event",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// No tasks. But run reason is task_assigned (event-driven).
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-event-1', 'ws-1', 'Event Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-event-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			t.Error("non-heartbeat run should never be idle-skipped")
		}
	}
}

func TestIdleSkip_CEODefaultFalse_NotSkipped(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	ceo := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "ceo-no-skip",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create ceo: %v", err)
	}
	// CEO has no tasks, but skip_idle_runs defaults to false.

	if err := svc.QueueRun(ctx, ceo.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			t.Error("CEO heartbeat should not be idle-skipped (default skip_idle_runs=false)")
		}
	}
}
