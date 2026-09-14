package service_test

// Covers AC-OFFICE-BUDGET-005.4: nine /debug/vars counters, one per
// individually-observable admission state, each labelled by run provenance.
// Each test drives a real admission decision (mostly via fakeBudgetEvaluator,
// as budget_admission_test.go already does) and asserts the exact counter
// bumps by exactly one, and that sibling counters for other states do not
// move -- so a mislabelled or misplaced increment fails loudly rather than
// only being caught by a coincidentally-passing total.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

const (
	metricBlockedByLimit      = "office_budget_blocked_by_limit_total"
	metricDeferredEvalFault   = "office_budget_deferred_evaluator_fault_total"
	metricBlockedNoEvaluator  = "office_budget_blocked_absent_evaluator_total"
	metricDeferredWorkspace   = "office_budget_deferred_workspace_lookup_total"
	metricCancelledNoWS       = "office_budget_cancelled_no_workspace_total"
	metricCancelledStale      = "office_budget_cancelled_stale_deferral_total"
	metricBlockedDegraded     = "office_budget_blocked_pricing_degraded_total"
	metricAdmittedDefault     = "office_budget_admitted_default_total"
	metricAdmittedDegradedWin = "office_budget_admitted_degraded_window_total"

	labelUnattended = "provenance=unattended"
	labelAttended   = "provenance=attended"
)

func TestAdmitRun_MetricBlockedByLimit(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{
		Decision:       models.PreLaunchDecisionBlockedByLimit,
		DecidingPolicy: &models.PreLaunchPolicyResult{PolicyID: "p-limit", LimitExceeded: true},
	}}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricBlockedByLimit, labelUnattended)
	beforeDegraded := service.BudgetMetricValueForTest(t, metricBlockedDegraded, labelUnattended)

	agent := makeAgent("worker-metric-blocked-limit", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricBlockedByLimit, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricBlockedByLimit, labelUnattended, got, before+1)
	}
	if got := service.BudgetMetricValueForTest(t, metricBlockedDegraded, labelUnattended); got != beforeDegraded {
		t.Errorf("%s[%s] = %d, want unchanged %d", metricBlockedDegraded, labelUnattended, got, beforeDegraded)
	}
}

func TestAdmitRun_MetricBlockedPricingDegraded(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{
		Decision: models.PreLaunchDecisionBlockedByDegradation,
		DecidingPolicy: &models.PreLaunchPolicyResult{
			PolicyID: "p-degraded", LimitExceeded: false, Degraded: true, DegradationBlocked: true,
		},
	}}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricBlockedDegraded, labelUnattended)
	beforeLimit := service.BudgetMetricValueForTest(t, metricBlockedByLimit, labelUnattended)

	agent := makeAgent("worker-metric-blocked-degraded", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricBlockedDegraded, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricBlockedDegraded, labelUnattended, got, before+1)
	}
	if got := service.BudgetMetricValueForTest(t, metricBlockedByLimit, labelUnattended); got != beforeLimit {
		t.Errorf("%s[%s] = %d, want unchanged %d", metricBlockedByLimit, labelUnattended, got, beforeLimit)
	}
}

func TestAdmitRun_MetricDeferredEvaluatorFault(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchErr: context.DeadlineExceeded}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricDeferredEvalFault, labelAttended)

	agent := makeAgent("worker-metric-deferred-fault", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-metric-deferred-fault", "ws-1")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-metric-deferred-fault"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricDeferredEvalFault, labelAttended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricDeferredEvalFault, labelAttended, got, before+1)
	}
}

func TestAdmitRun_MetricBlockedAbsentEvaluator(t *testing.T) {
	svc := newTestService(t)
	svc.SetBudgetChecker(nil)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricBlockedNoEvaluator, labelUnattended)

	agent := makeAgent("worker-metric-no-evaluator", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricBlockedNoEvaluator, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricBlockedNoEvaluator, labelUnattended, got, before+1)
	}
}

// TestAdmitRun_MetricDeferredWorkspaceLookup covers the one counter whose
// cause never reaches admitRun at all (W2 disposition): a genuine
// GetAgentFromConfig I/O error, instrumented at its existing call site in
// processRun. AC-OFFICE-BUDGET-001.13 gives this a different disposition
// than a lookup that succeeds and finds no row (see
// TestProcessRun_AgentRowDeleted_Cancels below): the query is broken by
// dropping the underlying table after the agent, so the earlier
// create/queue/claim calls that also read it succeed normally and only the
// processRun call under test observes the failure -- a real, non-ErrNoRows
// error, not a simulation of one.
func TestAdmitRun_MetricDeferredWorkspaceLookup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricDeferredWorkspace, labelUnattended)

	agent := makeAgent("worker-metric-lookup-error", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	svc.ExecSQL(t, `DROP TABLE agent_profiles`)
	service.ProcessRunForTest(svc, ctx, run)

	if got := service.BudgetMetricValueForTest(t, metricDeferredWorkspace, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricDeferredWorkspace, labelUnattended, got, before+1)
	}
}

// TestProcessRun_AgentRowDeleted_Cancels covers AC-OFFICE-BUDGET-001.13's
// other workspace-lookup disposition: the lookup itself succeeds (no I/O
// error) but finds no row, because the agent was deleted after the run was
// queued. Unlike a lookup error (deferred/retried above), this is
// cancelled -- an orphaned run has no workspace and so no ceiling that
// could ever be evaluated -- and must never reach MaxRetryCount or
// escalate. Drives it through the real processRun pipeline (not
// AdmitRunForTest, which only reaches admitRun's separate, structurally
// unreachable-in-production gate-1 branch for an agent found with an empty
// WorkspaceID -- see TestAdmitRun_MetricCancelledNoWorkspace above).
func TestProcessRun_AgentRowDeleted_Cancels(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	beforeCancelled := service.BudgetMetricValueForTest(t, metricCancelledNoWS, labelUnattended)
	beforeDeferred := service.BudgetMetricValueForTest(t, metricDeferredWorkspace, labelUnattended)

	agent := makeAgent("worker-metric-agent-deleted", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	svc.ExecSQL(t, `DELETE FROM agent_profiles WHERE id = ?`, agent.ID)
	service.ProcessRunForTest(svc, ctx, run)

	if got := service.BudgetMetricValueForTest(t, metricCancelledNoWS, labelUnattended); got != beforeCancelled+1 {
		t.Errorf("%s[%s] = %d, want %d", metricCancelledNoWS, labelUnattended, got, beforeCancelled+1)
	}
	if got := service.BudgetMetricValueForTest(t, metricDeferredWorkspace, labelUnattended); got != beforeDeferred {
		t.Errorf("%s[%s] = %d, want unchanged %d (a deleted agent must cancel, not defer)",
			metricDeferredWorkspace, labelUnattended, got, beforeDeferred)
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != service.RunStatusCancelled {
		t.Errorf("run status = %q, want %q", got.Status, service.RunStatusCancelled)
	}
	if got.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0 (cancelled, never retried)", got.RetryCount)
	}
}

// TestAdmitRun_MetricCancelledNoWorkspace covers gate 1 via AdmitRunForTest
// rather than the full scheduler pipeline: GetAgentFromConfig's own
// agentInstanceFilter (workspace_id must be non-empty) means a real
// ClaimNextRun/processRun cycle can never hand admitRun an agent with an
// empty WorkspaceID, mirroring resolveRunProject's already-documented
// unreachable branches. The gate 1 code path itself is real and still needs
// coverage.
func TestAdmitRun_MetricCancelledNoWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricCancelledNoWS, labelUnattended)

	agent := makeAgent("worker-metric-no-workspace", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	unresolvedAgent := *agent
	unresolvedAgent.WorkspaceID = ""
	service.AdmitRunForTest(svc, ctx, run, &unresolvedAgent)

	if got := service.BudgetMetricValueForTest(t, metricCancelledNoWS, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricCancelledNoWS, labelUnattended, got, before+1)
	}
}

// TestAdmitRun_MetricCancelledStaleDeferral drives admitBudgetDeferral's
// stale branch by backdating a claimed run's RequestedAt past retryMaxAge,
// mirroring the RetryCount mutation the MaxRetryCount test in
// budget_admission_test.go already uses to reach a similar deep branch.
func TestAdmitRun_MetricCancelledStaleDeferral(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchErr: context.DeadlineExceeded}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricCancelledStale, labelUnattended)

	agent := makeAgent("worker-metric-stale-deferral", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}
	run.RequestedAt = time.Now().Add(-25 * time.Hour)

	service.ProcessRunForTest(svc, ctx, run)

	if got := service.BudgetMetricValueForTest(t, metricCancelledStale, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricCancelledStale, labelUnattended, got, before+1)
	}
}

func TestAdmitRun_MetricAdmittedDefault(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{
		preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch},
		defaultResult:   models.PreLaunchPolicyResult{IsDefault: true},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricAdmittedDefault, labelUnattended)
	beforeDegraded := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelUnattended)

	agent := makeAgent("worker-metric-admitted-default", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricAdmittedDefault, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricAdmittedDefault, labelUnattended, got, before+1)
	}
	if got := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelUnattended); got != beforeDegraded {
		t.Errorf("%s[%s] = %d, want unchanged %d (default was not degraded)", metricAdmittedDegradedWin, labelUnattended, got, beforeDegraded)
	}
}

// TestAdmitRun_MetricAdmittedDegradedWindow_ViaDefault covers the gate-5
// branch of AC-OFFICE-BUDGET-005.4's carved-out counter: the default's
// window was degraded but did not block, and the run still launches.
func TestAdmitRun_MetricAdmittedDegradedWindow_ViaDefault(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{
		preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch},
		defaultResult: models.PreLaunchPolicyResult{
			IsDefault: true, Degraded: true, DegradationBlocked: false, LimitExceeded: false,
		},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelUnattended)

	agent := makeAgent("worker-metric-degraded-default", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelUnattended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricAdmittedDegradedWin, labelUnattended, got, before+1)
	}
}

// TestAdmitRun_MetricAdmittedDegradedWindow_ViaStoredPolicyBypassingGate5
// covers the other launch point that can carry a degraded-admitted policy:
// an attended run, whose provenance bypasses gate 5 entirely, but which was
// still admitted against a stored policy's degraded window at gate 4.
func TestAdmitRun_MetricAdmittedDegradedWindow_ViaStoredPolicyBypassingGate5(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{
		preLaunchResult: models.PreLaunchResult{
			Decision: models.PreLaunchDecisionLaunch,
			Policies: []models.PreLaunchPolicyResult{
				{PolicyID: "p-degraded-admitted", Degraded: true, DegradationBlocked: false, LimitExceeded: false},
			},
		},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()
	before := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelAttended)

	agent := makeAgent("worker-metric-degraded-attended", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-metric-degraded-attended", "ws-1")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-metric-degraded-attended"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if got := service.BudgetMetricValueForTest(t, metricAdmittedDegradedWin, labelAttended); got != before+1 {
		t.Errorf("%s[%s] = %d, want %d", metricAdmittedDegradedWin, labelAttended, got, before+1)
	}
}
