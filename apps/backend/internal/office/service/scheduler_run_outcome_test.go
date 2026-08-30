package service_test

// Covers docs/specs/task-delivery-ledger/spec.md, "Office run outcome":
// drives each of the four non-"processed" FinishRun call sites in
// scheduler_integration.go through its real triggering condition (not a
// hardcoded FinishRun call) and asserts the resulting runs.outcome value.
// Existing tests in scheduler_features_test.go already exercise these
// trigger conditions (budget, checkout, idle-skip) for their other side
// effects (activity log entries, StartTask call counts) but never assert
// the outcome column itself, which is the gap this file closes.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
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

// TestSchedulerOutcome_AgentInactive_WritesOutcome covers scheduler_integration.go:218.
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

// TestSchedulerOutcome_IdleSkipped_WritesOutcome covers scheduler_integration.go:247.
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

// TestSchedulerOutcome_TaskTreeHeld_WritesOutcome covers scheduler_integration.go:517.
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

// assertRetriedNotFinished asserts a run was requeued for retry rather than
// finished: still non-terminal, retry_count bumped, a scheduled_retry_at set,
// and no outcome written (outcome is only ever set on the finished path).
func assertRetriedNotFinished(t *testing.T, run *models.Run, wantRetryCount int) {
	t.Helper()
	if run.Status == "finished" || run.Status == "failed" {
		t.Fatalf("status = %q, want a non-terminal (retried) status", run.Status)
	}
	if run.RetryCount != wantRetryCount {
		t.Fatalf("retry_count = %d, want %d", run.RetryCount, wantRetryCount)
	}
	if run.ScheduledRetryAt == nil {
		t.Fatalf("scheduled_retry_at = nil, want a scheduled retry time")
	}
	if run.Outcome != nil {
		t.Fatalf("outcome = %q, want nil (never written on the retry path)", *run.Outcome)
	}
}

// TestSchedulerOutcome_CheckoutError_RetriesInsteadOfFinishing covers
// scheduler_integration.go's tryCheckout error branch. Forces a real DB
// error from CheckoutTask's UPDATE by dropping the tasks table after the
// run is claimed (so ClaimNextRun itself is unaffected) and driving the
// claimed run through processRun directly.
//
// A transient checkout error is retried like any other run failure via
// HandleRunFailure, not finished. This preserves the retry-then-escalate
// behavior already present on the base branch.
func TestSchedulerOutcome_CheckoutError_RetriesInsteadOfFinishing(t *testing.T) {
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
	assertRetriedNotFinished(t, got, 1)

	// Past MaxRetryCount, the same error escalates to a permanent failure
	// instead of retrying again — FailRun deliberately writes a NULL
	// outcome (outcome is not part of the failed-path vocabulary).
	svc.ExecSQL(t, `UPDATE runs SET retry_count = ?, status = 'claimed' WHERE id = ?`,
		service.MaxRetryCount, got.ID)
	got.RetryCount = service.MaxRetryCount
	got.Status = "claimed"
	service.ProcessRunForTest(svc, ctx, got)

	final, err := svc.GetRun(ctx, got.ID)
	if err != nil {
		t.Fatalf("get run after escalation: %v", err)
	}
	if final.Status != "failed" {
		t.Fatalf("status = %q, want failed after exhausting retries", final.Status)
	}
	if final.Outcome != nil {
		t.Fatalf("outcome = %q, want nil on the failed path", *final.Outcome)
	}
}

// TestSchedulerOutcome_CheckoutUnavailable_RetriesInsteadOfFinishing covers
// scheduler_integration.go's requeueContendedCheckout path.
//
// A lost checkout race is requeued via requeueContendedCheckout instead of
// being finished, because the run did not acquire the task checkout.
func TestSchedulerOutcome_CheckoutUnavailable_RetriesInsteadOfFinishing(t *testing.T) {
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
	assertRetriedNotFinished(t, run, 1)
}

// TestSchedulerOutcome_BudgetBlocked_WritesOutcome covers scheduler_integration.go:829.
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

// TestSchedulerOutcome_NoTaskStarter_FailsRun covers the fail-fast wiring
// guard in scheduler_integration.go. A missing TaskStarter is a permanent
// process capability gap, so the run must be visible as failed instead of
// being reported as completed work.
func TestSchedulerOutcome_NoTaskStarter_FailsRun(t *testing.T) {
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
	if run.Status != service.RunStatusFailed {
		t.Fatalf("status = %q, want %q", run.Status, service.RunStatusFailed)
	}
	if run.Outcome != nil {
		t.Fatalf("outcome = %v, want NULL on failed run", *run.Outcome)
	}
	if run.ErrorMessage == "" {
		t.Fatal("expected failed run to include a diagnostic error message")
	}
}

// TestSchedulerOutcome_Processed_WritesOutcome covers
// event_subscribers.go:408 (handleAgentCompleted), the only path that
// yields "processed" — every other FinishRun call site in this file is
// driven through its own real trigger, but until now nothing drove the
// agent-completed subscriber itself and checked runs.outcome: existing
// coverage (TestRunLifecycle_StepCompleteEventsEmitted) publishes the
// same AgentCompleted event but only asserts the run's Events log, never
// the outcome column (spec.md:2118-2120).
//
// The synthetic event carries agent_profile_id, matching every real
// AgentCompleted publish site (lifecycle/events.go sets it from
// execution.officeProfileID()): resolveLifecycleRun's task+agent fallback
// (used here since the event carries no run_id) scopes its lookup by
// agent_profile_id, so an event missing it would never match the claimed
// run and would leave it stuck at "claimed" instead of "finished".
func TestSchedulerOutcome_Processed_WritesOutcome(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-processed")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-processed")

	if err := svc.QueueRun(
		ctx, "worker-processed", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "processed-outcome",
	); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-processed",
		"agent_profile_id": "worker-processed",
	})
	if err := eb.Publish(ctx, events.AgentCompleted, completed); err != nil {
		t.Fatalf("publish completed: %v", err)
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	assertOutcome(t, got, service.RunOutcomeProcessed)
}
