package service_test

// Covers docs/specs/task-delivery-ledger/spec.md, "Office run outcome":
// drives each of the six non-"processed" FinishRun call sites in
// scheduler_integration.go through its real triggering condition (not a
// hardcoded FinishRun call) and asserts the resulting runs.outcome value.
// Existing tests in scheduler_features_test.go already exercise these
// trigger conditions (budget, checkout, idle-skip) for their other side
// effects (activity log entries, StartTask call counts) but never assert
// the outcome column itself, which is the gap this file closes.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// findRunForAgent locates the run queued for agentID with the given reason
// after a scheduler tick has processed it. Mirrors the lookup pattern in
// TestSchedulerIntegration_ResolvesExecutorFromTaskProject.
func findRunForAgent(t *testing.T, svc *service.Service, ctx context.Context, wsID, agentID, reason string) *models.Run {
	t.Helper()
	runs, err := svc.ListRuns(ctx, wsID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, run := range runs {
		if run.AgentProfileID == agentID && run.Reason == reason {
			return run
		}
	}
	t.Fatalf("no run found for agent %s reason %s: %#v", agentID, reason, runs)
	return nil
}

func assertOutcome(t *testing.T, run *models.Run, want string) {
	t.Helper()
	if run.Status != "finished" {
		t.Fatalf("status = %q, want finished", run.Status)
	}
	if run.Outcome == nil {
		t.Fatalf("outcome = nil, want %q", want)
	}
	if *run.Outcome != want {
		t.Fatalf("outcome = %q, want %q", *run.Outcome, want)
	}
}

// TestSchedulerOutcome_AgentInactive_WritesOutcome covers scheduler_integration.go:196.
// isAgentActive's guard exists for the race window between claim and
// processing, so the agent is paused after the run is claimed rather than
// before — ClaimNextRun's own query already excludes non-idle/working agents,
// which is exactly why this scenario needs ProcessRunForTest instead of
// RunSchedulerTick.
func TestSchedulerOutcome_AgentInactive_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-inactive-outcome", models.AgentRoleWorker)
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
	if run == nil {
		t.Fatal("expected a run")
	}

	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	service.ProcessRunForTest(svc, ctx, run)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	assertOutcome(t, got, service.RunOutcomeAgentInactive)
}

// TestSchedulerOutcome_IdleSkipped_WritesOutcome covers scheduler_integration.go:218.
func TestSchedulerOutcome_IdleSkipped_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-outcome-worker",
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

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonHeartbeat)
	assertOutcome(t, run, service.RunOutcomeIdleSkipped)
}

// TestSchedulerOutcome_TaskTreeHeld_WritesOutcome covers scheduler_integration.go:479.
func TestSchedulerOutcome_TaskTreeHeld_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-tree-outcome', 'ws-1', 'Tree Held Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	svc.ExecSQL(t, `INSERT INTO office_task_tree_holds
		(id, workspace_id, root_task_id, mode, created_at)
		VALUES ('hold-outcome-1', 'ws-1', 'task-tree-outcome', ?, CURRENT_TIMESTAMP)`,
		models.TreeHoldModePause)
	svc.ExecSQL(t, `INSERT INTO office_task_tree_hold_members
		(hold_id, task_id, depth) VALUES ('hold-outcome-1', 'task-tree-outcome', 0)`)

	agent := makeAgent("worker-tree-held-outcome", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-tree-outcome"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	assertOutcome(t, run, service.RunOutcomeTaskTreeHeld)
}

// TestSchedulerOutcome_CheckoutError_WritesOutcome covers scheduler_integration.go:589.
// Forces a real DB error from CheckoutTask's UPDATE by dropping the tasks
// table after the run is claimed (so ClaimNextRun itself is unaffected) and
// driving the claimed run through processRun directly.
func TestSchedulerOutcome_CheckoutError_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-checkout-error', 'ws-1', 'Checkout Error Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	agent := makeAgent("worker-checkout-error", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-checkout-error"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run")
	}

	svc.ExecSQL(t, `DROP TABLE tasks`)

	service.ProcessRunForTest(svc, ctx, run)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	assertOutcome(t, got, service.RunOutcomeCheckoutError)
}

// TestSchedulerOutcome_CheckoutUnavailable_WritesOutcome covers scheduler_integration.go:596.
func TestSchedulerOutcome_CheckoutUnavailable_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-checkout-unavail', 'ws-1', 'Checkout Unavailable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	// Another agent already holds the checkout.
	ok, err := svc.CheckoutTask(ctx, "task-checkout-unavail", "someone-else-agent")
	if err != nil || !ok {
		t.Fatalf("seed checkout: ok=%v err=%v", ok, err)
	}

	agent := makeAgent("worker-checkout-unavail", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-checkout-unavail"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	assertOutcome(t, run, service.RunOutcomeCheckoutUnavailable)
}

// TestSchedulerOutcome_BudgetBlocked_WritesOutcome covers scheduler_integration.go:619.
func TestSchedulerOutcome_BudgetBlocked_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-budget-outcome", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           agent.ID,
		LimitSubcents:     100,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertTestTask(t, svc, "task-budget-outcome", "ws-1")
	insertTestCostEvent(t, svc, agent.ID, "task-budget-outcome", int64(600))

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-budget-outcome"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	assertOutcome(t, run, service.RunOutcomeBudgetBlocked)
}

// TestSchedulerOutcome_NoAgentLaunched_WritesOutcome covers scheduler_integration.go:642.
// No TaskStarter is wired, so launchOrLog logs rather than launches and
// prepareAndLaunch reaches finishRun (see spec.md, "Office run outcome",
// "Why :639 is not processed" — the outcome is no_agent_launched, never
// processed, because no agent actually ran).
func TestSchedulerOutcome_NoAgentLaunched_WritesOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTestTask(t, svc, "task-no-launch", "ws-1")

	agent := makeAgent("worker-no-launch", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_pc"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-no-launch"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	assertOutcome(t, run, service.RunOutcomeNoAgentLaunched)
}
