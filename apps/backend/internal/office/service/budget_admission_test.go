package service_test

// Covers the five pre-launch admission gates of
// docs/specs/office/requirements/budget-enforcement.md AC-OFFICE-BUDGET-001.14,
// replacing the old checkBudget's two fail-open paths (no evaluator wired,
// evaluator error) and closing the third (zero configured policies) via the
// built-in default at gate 5.

import (
	"context"
	"encoding/json"
	"sort"
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

// deferralAttempts returns the "attempt" field of every activity entry for
// runID matching action, sorted ascending. Sorted rather than
// created_at-ordered because repeated deferrals in a test can land within
// the same timestamp resolution.
func deferralAttempts(t *testing.T, svc *service.Service, wsID, runID, action string) []string {
	t.Helper()
	entries, err := svc.ListActivity(context.Background(), wsID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	var attempts []string
	for _, e := range entries {
		if e.RunID != runID || string(e.Action) != action {
			continue
		}
		var fields map[string]string
		if err := json.Unmarshal([]byte(e.Details), &fields); err != nil {
			t.Fatalf("unmarshal activity details %q: %v", e.Details, err)
		}
		attempts = append(attempts, fields["attempt"])
	}
	sort.Strings(attempts)
	return attempts
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

// TestFinishPolicyBlock_ClearsStaleAgentWorkingStatus and
// TestCancelBudgetRun_ClearsStaleAgentWorkingStatus pin a fix carried over
// from a rebase onto main: main independently added an equivalent
// clearAgentWorking call to the old checkBudget's block branch (fixing a
// stuck-"working" agent), and cancelBudgetRun's own doc comment already
// claimed to mirror cancelStaleRun's release/cancel/publish/log sequence --
// which has always included clearAgentWorking -- without actually doing so.
// admitRun always runs before markAgentWorking in the real pipeline
// (processRun calls admitRun, then prepareAndLaunch marks working), so the
// only way an agent is already "working" when a block/cancel fires for the
// same run.ID is a run that reached the launch boundary once, then got
// requeued (e.g. a contended checkout retry) and is being re-admitted.

// TestFinishPolicyBlock_ClearsStaleAgentWorkingStatus covers finishPolicyBlock.
func TestFinishPolicyBlock_ClearsStaleAgentWorkingStatus(t *testing.T) {
	svc := newTestService(t)
	deciding := &models.PreLaunchPolicyResult{PolicyID: "policy-stale-working", LimitExceeded: true}
	fake := &fakeBudgetEvaluator{
		preLaunchResult: models.PreLaunchResult{
			Decision:       models.PreLaunchDecisionBlockedByLimit,
			DecidingPolicy: deciding,
			Policies:       []models.PreLaunchPolicyResult{*deciding},
		},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	agent := makeAgent("worker-stale-working-block", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	if _, err := svc.RepoForTest().MarkAgentWorking(ctx, agent.ID, run.ID); err != nil {
		t.Fatalf("mark agent working: %v", err)
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before the re-admission block")

	service.AdmitRunForTest(svc, ctx, run, agent)

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after a policy block clears a stale working mark left by an earlier attempt")
}

// TestCancelBudgetRun_ClearsStaleAgentWorkingStatus covers cancelBudgetRun,
// via gate 1's cancellation path.
func TestCancelBudgetRun_ClearsStaleAgentWorkingStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-stale-working-cancel", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	if _, err := svc.RepoForTest().MarkAgentWorking(ctx, agent.ID, run.ID); err != nil {
		t.Fatalf("mark agent working: %v", err)
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before the re-admission cancel")

	unresolvedAgent := *agent
	unresolvedAgent.WorkspaceID = ""
	service.AdmitRunForTest(svc, ctx, run, &unresolvedAgent)

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after a gate-1 cancel clears a stale working mark left by an earlier attempt")
}

// TestFailRunNoEscalation_ClearsStaleAgentWorkingStatus covers
// failRunNoEscalation (retry.go), reached via admitBudgetDeferral once
// RetryCount reaches MaxRetryCount. Same stale-mark scenario as the two
// tests above: a run that reached the launch boundary once, was requeued,
// then exhausted its retries on a budget-admission fault must not leave its
// agent stuck "working" forever.
func TestFailRunNoEscalation_ClearsStaleAgentWorkingStatus(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{preLaunchErr: context.DeadlineExceeded}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	agent := makeAgent("worker-stale-working-no-escalation", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	if _, err := svc.RepoForTest().MarkAgentWorking(ctx, agent.ID, run.ID); err != nil {
		t.Fatalf("mark agent working: %v", err)
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before the re-admission fault")

	run.RetryCount = service.MaxRetryCount
	service.AdmitRunForTest(svc, ctx, run, agent)

	failed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if failed.Status != service.RunStatusFailed {
		t.Fatalf("status at MaxRetryCount = %q, want failed", failed.Status)
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after failRunNoEscalation clears a stale working mark left by an earlier attempt")
}

// TestProcessRun_MissingAgentReleasesPriorCheckout covers a routed run that
// was requeued after launch. The next scheduler pass may find that its agent
// no longer exists, but the run still owns the task checkout from its prior
// attempt. The cancellation must release that ownership.
func TestProcessRun_MissingAgentReleasesPriorCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-routed-missing", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-routed-missing', 'ws-1', 'Routed task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-routed-missing"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}
	acquired, err := svc.RepoForTest().CheckoutTaskForRun(ctx, "task-routed-missing", agent.ID, run.ID)
	if err != nil || !acquired {
		t.Fatalf("seed run checkout: acquired=%v err=%v", acquired, err)
	}
	if _, err := svc.RepoForTest().MarkAgentWorking(ctx, agent.ID, run.ID); err != nil {
		t.Fatalf("mark agent working: %v", err)
	}
	if err := svc.RepoForTest().DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}

	service.ProcessRunForTest(svc, ctx, run)

	replacement := makeAgent("worker-routed-replacement", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, replacement); err != nil {
		t.Fatalf("create replacement agent: %v", err)
	}
	acquired, err = svc.CheckoutTask(ctx, "task-routed-missing", replacement.ID)
	if err != nil {
		t.Fatalf("replacement checkout: %v", err)
	}
	if !acquired {
		t.Fatal("replacement agent could not acquire a checkout left by the missing-agent cancellation")
	}

	cancelled, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get cancelled run: %v", err)
	}
	if cancelled.Status != service.RunStatusCancelled {
		t.Fatalf("run status = %q, want cancelled", cancelled.Status)
	}
}

// TestProcessRun_WorkspaceLookupErrorReleasesPriorCheckout covers the retry
// branch. A temporary repository lookup error must not preserve a checkout
// while the run waits in the queue.
func TestProcessRun_WorkspaceLookupErrorReleasesPriorCheckout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-routed-lookup-error", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-routed-lookup-error', 'ws-1', 'Routed task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-routed-lookup-error"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}
	acquired, err := svc.RepoForTest().CheckoutTaskForRun(ctx, "task-routed-lookup-error", agent.ID, run.ID)
	if err != nil || !acquired {
		t.Fatalf("seed run checkout: acquired=%v err=%v", acquired, err)
	}

	// Make both agent lookup queries fail with a transient database error.
	svc.ExecSQL(t, "DROP TABLE agent_profiles")
	service.ProcessRunForTest(svc, ctx, run)

	acquired, err = svc.CheckoutTask(ctx, "task-routed-lookup-error", "replacement-agent")
	if err != nil {
		t.Fatalf("replacement checkout: %v", err)
	}
	if !acquired {
		t.Fatal("replacement agent could not acquire a checkout left by the lookup-error deferral")
	}

	deferred, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get deferred run: %v", err)
	}
	if deferred.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want queued", deferred.Status)
	}
}

// TestAdmitRun_RepeatedDeferral_AttemptIncrementsPerActivity covers
// AC-OFFICE-BUDGET-005.6: each deferral of the same run writes its own
// activity entry -- never coalesced into a single running total -- and each
// entry's "attempt" field tracks that call's run.RetryCount+1.
func TestAdmitRun_RepeatedDeferral_AttemptIncrementsPerActivity(t *testing.T) {
	svc := newTestService(t)
	fake := &fakeBudgetEvaluator{
		preLaunchErr: &models.UnevaluatedPolicyError{PolicyID: "policy-attempt", Err: context.DeadlineExceeded},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	agent := makeAgent("worker-attempt-increments", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestTask(t, svc, "task-attempt-increments", "ws-1")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-attempt-increments"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	for i := 0; i < 3; i++ {
		service.AdmitRunForTest(svc, ctx, run, agent)
		run.RetryCount++
	}

	got := deferralAttempts(t, svc, "ws-1", run.ID, "run_budget_unevaluated_policy_deferred")
	want := []string{"1", "2", "3"}
	if len(got) != len(want) {
		t.Fatalf("attempts = %v, want %v (one entry per deferral, not coalesced)", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("attempts = %v, want %v", got, want)
			break
		}
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

// TestAdmitRun_PricingDegradedBlock_UnmeasurableOutcome covers
// AC-OFFICE-BUDGET-004.6/-005.7: a policy that blocks purely on pricing
// degradation (its limit was never confirmed reached) carries the distinct
// run_budget_unmeasurable outcome and its own activity action, not the plain
// limit-block pair.
func TestAdmitRun_PricingDegradedBlock_UnmeasurableOutcome(t *testing.T) {
	svc := newTestService(t)
	deciding := &models.PreLaunchPolicyResult{
		PolicyID: "policy-pricing-degraded", Degraded: true, DegradationBlocked: true, LimitExceeded: false,
	}
	fake := &fakeBudgetEvaluator{
		preLaunchResult: models.PreLaunchResult{
			Decision:       models.PreLaunchDecisionBlockedByDegradation,
			DecidingPolicy: deciding,
			Policies:       []models.PreLaunchPolicyResult{*deciding},
		},
	}
	svc.SetBudgetChecker(fake)
	ctx := context.Background()

	agent := makeAgent("worker-pricing-degraded-block", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonRoutineTrigger)
	assertOutcome(t, run, service.RunOutcomeBudgetUnmeasurable)
	if !hasActivityAction(t, svc, "ws-1", "run_budget_pricing_degraded_blocked") {
		t.Error("expected run_budget_pricing_degraded_blocked activity entry")
	}
	if hasActivityAction(t, svc, "ws-1", "run_budget_blocked") {
		t.Error("a pricing-degraded block must not also fire the plain limit-block action")
	}
}

// TestAdmitRun_DefaultCeiling_ExemptsAttendedRun covers AC-OFFICE-BUDGET-003.3:
// the built-in default ceiling gates unattended runs only. An attended run
// launches even though its workspace already exceeds the default ceiling
// (mirroring TestAdmitRun_DefaultCeiling_BlocksUnattendedRunWithZeroPolicies's
// spend fixture, but with a task-assigned, attended run instead).
func TestAdmitRun_DefaultCeiling_ExemptsAttendedRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-default-ceiling-attended", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	insertTestCostEvent(t, svc, agent.ID, "task-default-ceiling-attended", int64(600_000))
	insertTestTask(t, svc, "task-default-ceiling-attended-run", "ws-1")
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-default-ceiling-attended-run"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	if run.Status == service.RunStatusCancelled {
		t.Fatalf("status = %q, attended run must not be cancelled by the default ceiling", run.Status)
	}
	if run.Status == "finished" && run.Outcome != nil && *run.Outcome == service.RunOutcomeBudgetBlocked {
		t.Fatalf("attended run was finished with outcome %q, want not blocked by the default ceiling", *run.Outcome)
	}
	if hasActivityAction(t, svc, "ws-1", "run_budget_blocked") {
		t.Error("attended run must not trigger a default-ceiling block")
	}
}

// activityActionForRun returns the single activity action recorded for
// runID within wsID's activity log, failing the test if none is found.
// cancelUnresolvableAgentRun logs with an empty workspace scope (no agent
// available to read WorkspaceID from), so callers pass "" for that scenario.
func activityActionForRun(t *testing.T, svc *service.Service, wsID, runID string) string {
	t.Helper()
	entries, err := svc.ListActivity(context.Background(), wsID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.RunID == runID {
			return string(e.Action)
		}
	}
	t.Fatalf("no activity entry found for run %s in workspace %q", runID, wsID)
	return ""
}

// TestBudgetAdmission_ActionsAreDistinguishable covers AC-OFFICE-BUDGET-005.3/
// -006.5: every admission-fault, cancellation, and block disposition this
// capability introduces writes its own distinct activity action, so an
// operator (or an alerting query keyed on the action string) can always
// tell dispositions apart. Each scenario runs against its own service
// instance so setup that mutates shared state (a dropped table, a forced
// retry count) can't leak into another scenario's result.
func TestBudgetAdmission_ActionsAreDistinguishable(t *testing.T) {
	type observedAction struct{ scenario, action string }
	var seen []observedAction
	record := func(scenario, action string) {
		seen = append(seen, observedAction{scenario, action})
	}

	t.Run("workspace_lookup_deferred", func(t *testing.T) {
		svc := newTestService(t)
		ctx := context.Background()
		agent := makeAgent("worker-actions-ws-deferred", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		svc.ExecSQL(t, `DROP TABLE agent_profiles`)
		service.ProcessRunForTest(svc, ctx, run)
		// deferWorkspaceLookupFailure logs with no workspace scope: no
		// *models.AgentInstance was ever resolved to read a WorkspaceID from.
		record("workspace_lookup_deferred", activityActionForRun(t, svc, "", run.ID))
	})

	t.Run("workspace_lookup_failed", func(t *testing.T) {
		svc := newTestService(t)
		ctx := context.Background()
		agent := makeAgent("worker-actions-ws-failed", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		svc.ExecSQL(t, `DROP TABLE agent_profiles`)
		run.RetryCount = service.MaxRetryCount
		service.ProcessRunForTest(svc, ctx, run)
		record("workspace_lookup_failed", activityActionForRun(t, svc, "", run.ID))
	})

	t.Run("workspace_unresolvable", func(t *testing.T) {
		svc := newTestService(t)
		ctx := context.Background()
		agent := makeAgent("worker-actions-ws-unresolvable", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		svc.ExecSQL(t, `DELETE FROM agent_profiles WHERE id = ?`, agent.ID)
		service.ProcessRunForTest(svc, ctx, run)
		record("workspace_unresolvable", activityActionForRun(t, svc, "", run.ID))
	})

	t.Run("unevaluated_policy_deferred", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{
			preLaunchErr: &models.UnevaluatedPolicyError{PolicyID: "policy-actions", Err: context.DeadlineExceeded},
		}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-unevaluated-deferred", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		service.RunSchedulerTick(svc, ctx)
		run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonRoutineTrigger)
		record("unevaluated_policy_deferred", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	t.Run("unevaluated_policy_failed", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{
			preLaunchErr: &models.UnevaluatedPolicyError{PolicyID: "policy-actions", Err: context.DeadlineExceeded},
		}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-unevaluated-failed", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		run.RetryCount = service.MaxRetryCount
		service.ProcessRunForTest(svc, ctx, run)
		record("unevaluated_policy_failed", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	// project_lookup_deferred/failed, payload_unparseable and task_not_found
	// all drive admitRun directly via AdmitRunForTest rather than through
	// RunSchedulerTick/ProcessRunForTest: as TestResolveRunProject's doc
	// comment explains, checkoutTask's own contention query independently
	// requires the named task to exist (or the tasks table to be queryable
	// at all), so a task-not-found or tasks-table failure is intercepted by
	// checkoutTask as "held by another agent" / a checkout error before the
	// run ever reaches admitRun through the scheduler pipeline -- exactly
	// the scenario each of these needs to construct.

	t.Run("project_lookup_deferred", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch}}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-project-deferred", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		svc.ExecSQL(t, `DROP TABLE tasks`)
		run.Payload = `{"task_id":"any-task"}`
		service.AdmitRunForTest(svc, ctx, run, agent)
		record("project_lookup_deferred", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	t.Run("project_lookup_failed", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch}}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-project-failed", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		svc.ExecSQL(t, `DROP TABLE tasks`)
		run.Payload = `{"task_id":"any-task"}`
		run.RetryCount = service.MaxRetryCount
		service.AdmitRunForTest(svc, ctx, run, agent)
		record("project_lookup_failed", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	t.Run("payload_unparseable", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch}}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-unparseable", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		run.Payload = `{not-json`
		service.AdmitRunForTest(svc, ctx, run, agent)
		record("payload_unparseable", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	t.Run("task_not_found", func(t *testing.T) {
		svc := newTestService(t)
		fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{Decision: models.PreLaunchDecisionLaunch}}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-task-not-found", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim: %v", err)
		}
		run.Payload = `{"task_id":"does-not-exist"}`
		service.AdmitRunForTest(svc, ctx, run, agent)
		record("task_not_found", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	t.Run("pricing_degraded_blocked", func(t *testing.T) {
		svc := newTestService(t)
		deciding := &models.PreLaunchPolicyResult{
			PolicyID: "policy-actions-degraded", Degraded: true, DegradationBlocked: true, LimitExceeded: false,
		}
		fake := &fakeBudgetEvaluator{preLaunchResult: models.PreLaunchResult{
			Decision: models.PreLaunchDecisionBlockedByDegradation, DecidingPolicy: deciding,
			Policies: []models.PreLaunchPolicyResult{*deciding},
		}}
		svc.SetBudgetChecker(fake)
		ctx := context.Background()
		agent := makeAgent("worker-actions-pricing-degraded", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue: %v", err)
		}
		service.RunSchedulerTick(svc, ctx)
		run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonRoutineTrigger)
		record("pricing_degraded_blocked", activityActionForRun(t, svc, "ws-1", run.ID))
	})

	const wantScenarios = 10
	if len(seen) != wantScenarios {
		t.Fatalf("expected %d scenarios to record an action, got %d: %+v", wantScenarios, len(seen), seen)
	}
	byAction := make(map[string]string, len(seen))
	for _, o := range seen {
		if prior, ok := byAction[o.action]; ok {
			t.Errorf("action %q fired for both %q and %q; every disposition must be independently distinguishable",
				o.action, prior, o.scenario)
			continue
		}
		byAction[o.action] = o.scenario
	}
}
