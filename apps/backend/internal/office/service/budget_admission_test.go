package service_test

// Covers the five pre-launch admission gates of
// docs/specs/office/requirements/budget-enforcement.md AC-OFFICE-BUDGET-001.14,
// replacing the old checkBudget's two fail-open paths (no evaluator wired,
// evaluator error) and closing the third (zero configured policies) via the
// built-in default at gate 5.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// fakeBudgetEvaluator lets admission tests force EvaluatePreLaunch/
// EvaluateDefaultCeiling's error and result shapes without needing a real
// repository failure.
type fakeBudgetEvaluator struct {
	preLaunchResult models.PreLaunchResult
	preLaunchErr    error
	defaultResult   models.PreLaunchPolicyResult
	defaultErr      error
}

func (f *fakeBudgetEvaluator) CheckPreExecutionBudget(
	context.Context, string, string, string,
) (bool, string, error) {
	return true, "", nil
}

func (f *fakeBudgetEvaluator) EvaluateBudget(context.Context, string, string, string) error {
	return nil
}

func (f *fakeBudgetEvaluator) EvaluatePreLaunch(
	context.Context, string, string, string, bool, shared.RunProvenance, time.Time,
) (models.PreLaunchResult, error) {
	return f.preLaunchResult, f.preLaunchErr
}

func (f *fakeBudgetEvaluator) EvaluateDefaultCeiling(
	context.Context, string, time.Time,
) (models.PreLaunchPolicyResult, error) {
	return f.defaultResult, f.defaultErr
}

func hasActivityAction(t *testing.T, svc *service.Service, wsID, action string) bool {
	t.Helper()
	entries, err := svc.ListActivity(context.Background(), wsID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if string(e.Action) == action {
			return true
		}
	}
	return false
}

// TestAdmitRun_NoEvaluatorWired_UnattendedCancels covers AC-OFFICE-BUDGET-001.5:
// an absent evaluator is a deployment fact, not a transient fault, so an
// unattended run is cancelled rather than retried or failed.
func TestAdmitRun_NoEvaluatorWired_UnattendedCancels(t *testing.T) {
	svc := newTestService(t)
	svc.SetBudgetChecker(nil)
	ctx := context.Background()

	agent := makeAgent("worker-no-evaluator-unattended", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonRoutineTrigger)
	if run.Status != service.RunStatusCancelled {
		t.Fatalf("status = %q, want cancelled", run.Status)
	}
	if !hasActivityAction(t, svc, "ws-1", "run_budget_no_evaluator") {
		t.Error("expected run_budget_no_evaluator activity entry")
	}
}

// TestAdmitRun_NoEvaluatorWired_AttendedLaunches covers AC-OFFICE-BUDGET-001.6:
// an absent evaluator never blocks an attended run.
func TestAdmitRun_NoEvaluatorWired_AttendedLaunches(t *testing.T) {
	svc := newTestService(t)
	svc.SetBudgetChecker(nil)
	ctx := context.Background()

	agent := makeAgent("worker-no-evaluator-attended", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-no-evaluator-attended", "ws-1")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-no-evaluator-attended"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if hasActivityAction(t, svc, "ws-1", "run_budget_no_evaluator") {
		t.Error("attended run must not be blocked by an absent evaluator")
	}
	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	if run.Status == service.RunStatusCancelled {
		t.Fatalf("status = %q, attended run must not be cancelled", run.Status)
	}
}

// TestAdmitRun_EvaluatorFault_DefersThenFailsWithoutEscalation covers
// AC-OFFICE-BUDGET-001.3/.4/.17: a plain evaluator error (not naming any
// policy) defers the run, and failing it at MaxRetryCount neither escalates
// to the CEO agent nor queues any new run.
func TestAdmitRun_EvaluatorFault_DefersThenFailsWithoutEscalation(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchErr: context.DeadlineExceeded}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	ceo := makeAgent("ceo-evaluator-fault", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create ceo: %v", err)
	}
	agent := makeAgent("worker-evaluator-fault", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-evaluator-fault", "ws-1")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-evaluator-fault"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	if run.Status != service.RunStatusQueued {
		t.Fatalf("status after first fault = %q, want queued (deferred)", run.Status)
	}
	if !hasActivityAction(t, svc, "ws-1", "run_budget_evaluator_fault_deferred") {
		t.Error("expected run_budget_evaluator_fault_deferred activity entry")
	}

	// Drive the run to MaxRetryCount and re-process it directly (bypassing
	// the real backoff delay, exactly as the existing retry tests do).
	run.RetryCount = service.MaxRetryCount
	service.ProcessRunForTest(svc, ctx, run)

	failed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if failed.Status != service.RunStatusFailed {
		t.Fatalf("status at MaxRetryCount = %q, want failed", failed.Status)
	}
	if !hasActivityAction(t, svc, "ws-1", "run_budget_evaluator_fault_failed") {
		t.Error("expected run_budget_evaluator_fault_failed activity entry")
	}

	// AC-OFFICE-BUDGET-001.17: no escalation run queued for the CEO.
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, r := range runs {
		if r.AgentProfileID == ceo.ID {
			t.Errorf("unexpected run queued for CEO agent after budget-fault failure: %+v", r)
		}
	}
}

// TestAdmitRun_UnevaluatedPolicy_NamesPolicyInDeferral covers
// AC-OFFICE-BUDGET-006.1/.5: an unevaluated-policy fault is distinguishable
// from a plain evaluator fault and names the policy it could not evaluate.
func TestAdmitRun_UnevaluatedPolicy_NamesPolicyInDeferral(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{
		preLaunchErr: &models.UnevaluatedPolicyError{PolicyID: "policy-xyz", Err: context.DeadlineExceeded},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	agent := makeAgent("worker-unevaluated-policy", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-unevaluated-policy", "ws-1")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-unevaluated-policy"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	if run.Status != service.RunStatusQueued {
		t.Fatalf("status = %q, want queued (deferred)", run.Status)
	}
	if !hasActivityAction(t, svc, "ws-1", "run_budget_unevaluated_policy_deferred") {
		t.Error("expected run_budget_unevaluated_policy_deferred activity entry")
	}
	if hasActivityAction(t, svc, "ws-1", "run_budget_evaluator_fault_deferred") {
		t.Error("unevaluated-policy fault must not also fire the generic evaluator-fault action")
	}
}

// TestAdmitRun_DefaultCeiling_BlocksUnattendedRunWithZeroPolicies closes the
// third of the three fail-open paths this capability replaces: with zero
// configured policies, an unattended run over the built-in default's daily
// ceiling is blocked rather than launched unconditionally.
func TestAdmitRun_DefaultCeiling_BlocksUnattendedRunWithZeroPolicies(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-default-ceiling", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestCostEvent(t, svc, agent.ID, "task-default-ceiling", int64(600_000))
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonRoutineTrigger)
	assertOutcome(t, run, service.RunOutcomeBudgetBlocked)
	if !hasActivityAction(t, svc, "ws-1", "run_budget_blocked") {
		t.Error("expected run_budget_blocked activity entry")
	}
}

// TestResolveRunProject covers AC-OFFICE-BUDGET-006.7's four-outcome split.
// Tested directly against resolveRunProject rather than through the full
// scheduler pipeline: checkoutTask's own contention query independently
// requires the named task to already exist (a 0-row checkout update reads
// identically to "held by another agent"), so a task-not-found or a
// tasks-table failure never reaches admitRun through that path at all in
// this codebase's existing checkout flow -- resolveRunProject's own
// contract is what this capability adds and is what needs covering here.
func TestResolveRunProject(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	insertTestTask(t, svc, "task-with-project", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = ? WHERE id = ?`, "project-1", "task-with-project")
	insertTestTask(t, svc, "task-no-project", "ws-1")

	cases := []struct {
		name          string
		payload       string
		wantProjectID string
		wantRes       int
	}{
		{"empty payload", "", "", service.ProjectResolutionNoneForTest},
		{"empty object payload", "{}", "", service.ProjectResolutionNoneForTest},
		{"no task_id key", `{"other":"field"}`, "", service.ProjectResolutionNoneForTest},
		{"task resolves with no project", `{"task_id":"task-no-project"}`, "", service.ProjectResolutionNoneForTest},
		{"task resolves with a project", `{"task_id":"task-with-project"}`, "project-1", service.ProjectResolutionFoundForTest},
		{"unparseable payload", `{not-json`, "", service.ProjectResolutionUnparseableForTest},
		{"task does not exist", `{"task_id":"does-not-exist"}`, "", service.ProjectResolutionTaskNotFoundForTest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotRes := service.ResolveRunProjectForTest(svc, ctx, tc.payload)
			if gotID != tc.wantProjectID || gotRes != tc.wantRes {
				t.Errorf("resolveRunProject(%q) = (%q, %d), want (%q, %d)",
					tc.payload, gotID, gotRes, tc.wantProjectID, tc.wantRes)
			}
		})
	}

	t.Run("lookup error", func(t *testing.T) {
		svc.ExecSQL(t, `DROP TABLE tasks`)
		_, gotRes := service.ResolveRunProjectForTest(svc, ctx, `{"task_id":"task-with-project"}`)
		if gotRes != service.ProjectResolutionLookupErrorForTest {
			t.Errorf("resolution = %d, want lookup error (%d)", gotRes, service.ProjectResolutionLookupErrorForTest)
		}
	})
}
