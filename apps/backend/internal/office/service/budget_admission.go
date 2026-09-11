package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// Activity-log field key/value literals shared across the budget-admission
// entries in this file and in failRunNoEscalation (retry.go).
const (
	activityFieldCeiling  = "ceiling"
	ceilingNotDetermined  = "not_determined"
	ceilingBuiltInDefault = "built_in_default"
)

// admitRun performs the five pre-launch admission gates of
// AC-OFFICE-BUDGET-001.14 and returns true only when the run may proceed to
// launch. It replaces the old checkBudget, which fail-opened both when no
// evaluator was wired and when the evaluator errored -- two of this
// capability's three closed fail-open paths. The third, zero configured
// policies, is closed by gate 5 (REQ-OFFICE-BUDGET-003's built-in default).
func (si *SchedulerIntegration) admitRun(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
) bool {
	provenance := shared.ClassifyRunProvenance(run.Reason)

	// Gate 1: workspace resolution (AC-OFFICE-BUDGET-001.13). The lookup
	// itself already happened in processRun via GetAgentFromConfig, which
	// now disposes of a lookup ERROR itself (deferWorkspaceLookupFailure)
	// before admitRun is ever called, per the recorded W2 human disposition
	// that GetAgentFromConfig's own error contract is not reshaped. This
	// gate is therefore defense-in-depth, not a live production path: the
	// repository's agentInstanceFilter (workspace_id != '') guarantees any
	// agent GetAgentFromConfig successfully returns already has a non-empty
	// WorkspaceID, so this branch is structurally unreachable via the sole
	// real caller and exists to keep AC-OFFICE-BUDGET-001.13's textual
	// requirement true of admitRun itself, independent of that caller.
	// AdmitRunForTest exercises it directly against a constructed agent.
	if agent.WorkspaceID == "" {
		incBudgetCancelledNoWorkspace(provenance)
		return si.cancelBudgetRun(ctx, run, agent, "no_resolvable_workspace",
			"run_budget_workspace_unresolvable", nil)
	}

	// Gate 2: evaluator presence (AC-OFFICE-BUDGET-001.5/.6).
	if si.svc.budgetChecker == nil {
		if provenance == shared.RunProvenanceAttended {
			return true
		}
		incBudgetBlockedAbsentEvaluator(provenance)
		return si.cancelBudgetRun(ctx, run, agent, "no_budget_evaluator",
			"run_budget_no_evaluator", nil)
	}

	// Gate 3/4: evaluator invocation + applicable policies. The run's
	// project identifier is resolved first (cheap: a single lookup, and its
	// two permanent-cancellation outcomes -- (c) unparseable payload, (d)
	// task not found -- are unconditional regardless of evaluator health),
	// but a *lookup error* (b) must not itself decide the disposition:
	// AC-OFFICE-BUDGET-001.14/-006.7 require project resolution to yield to
	// gate 3 succeeding first, so that a run whose evaluator errors and
	// whose project lookup would also fail gets one determinate disposition
	// (evaluator fault) rather than one depending on which lookup happens to
	// run first. On a lookup error we therefore invoke the evaluator with no
	// project scope (a project-lookup failure is not itself evidence a
	// project applies -- AC-OFFICE-BUDGET-001.15 already treats "no project"
	// this way) to test gate 3 in isolation: an evaluator fault discovered
	// there takes precedence, and only once gate 3 is confirmed healthy does
	// the unresolvable project get its own distinguishable disposition
	// (AC-OFFICE-BUDGET-006.3).
	projectID, res := si.resolveRunProject(ctx, run.Payload)
	switch res {
	case projectResolutionUnparseable:
		incBudgetCancelledUnparseablePayload(provenance)
		return si.cancelBudgetRun(ctx, run, agent, "unparseable_payload",
			"run_budget_payload_unparseable", nil)
	case projectResolutionTaskNotFound:
		incBudgetCancelledTaskNotFound(provenance)
		return si.cancelBudgetRun(ctx, run, agent, "task_not_found",
			"run_budget_task_not_found", nil)
	}
	hasProject := res == projectResolutionFound
	if res == projectResolutionLookupError {
		projectID, hasProject = "", false
	}

	now := time.Now().UTC()
	result, err := si.svc.EvaluatePreLaunch(ctx, agent.WorkspaceID, agent.ID, projectID, hasProject, provenance, now)
	if err != nil {
		var upErr *models.UnevaluatedPolicyError
		if errors.As(err, &upErr) {
			return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralUnevaluatedPolicy, upErr.PolicyID)
		}
		return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralEvaluatorFault, "")
	}
	if res == projectResolutionLookupError {
		// Gate 3 just succeeded (the evaluator itself is healthy) but the
		// project remains unresolvable -- its own distinguishable cause,
		// per AC-OFFICE-BUDGET-006.3/-006.7.
		return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralProjectLookupError, "")
	}
	si.logPolicyObservability(ctx, agent.WorkspaceID, run.ID, result.Policies, now)
	degradedAdmitted := anyDegradedAdmitted(result.Policies)
	if result.Decision != models.PreLaunchDecisionLaunch {
		return si.finishPolicyBlock(ctx, run, agent, result.DecidingPolicy)
	}

	return si.admitDefaultCeilingGate(ctx, run, agent, provenance, result, degradedAdmitted, now)
}

// admitDefaultCeilingGate evaluates gate 5 (AC-OFFICE-BUDGET-003.1/.4/.11):
// the built-in default ceiling, applied to unattended runs only, and only
// when no workspace-scoped blocking-capable daily policy already supersedes
// it. Split out of admitRun to keep its cyclomatic complexity within the
// repo's golangci-lint limit; degradedAdmitted carries whether any of
// result's stored policies were already degraded-admitted at gate 4, so the
// AC-OFFICE-BUDGET-005.4 degraded-window counter still fires at most once
// per run even when both gate 4 and gate 5 saw a degraded window.
func (si *SchedulerIntegration) admitDefaultCeilingGate(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	provenance shared.RunProvenance, result models.PreLaunchResult, degradedAdmitted bool, now time.Time,
) bool {
	if provenance != shared.RunProvenanceUnattended || result.WorkspaceDailyBlockingSuperseded {
		if degradedAdmitted {
			incBudgetAdmittedDegradedWindow(provenance)
		}
		return true
	}
	def, defErr := si.svc.EvaluateDefaultCeiling(ctx, agent.WorkspaceID, now)
	if defErr != nil {
		return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralEvaluatorFault, "")
	}
	si.logPolicyObservability(ctx, agent.WorkspaceID, run.ID, []models.PreLaunchPolicyResult{def}, now)
	if def.LimitExceeded || def.DegradationBlocked {
		return si.finishPolicyBlock(ctx, run, agent, &def)
	}
	incBudgetAdmittedDefault(provenance)
	if degradedAdmitted || isDegradedAdmitted(&def) {
		incBudgetAdmittedDegradedWindow(provenance)
	}
	return true
}

// finishPolicyBlock finishes run as blocked by p -- either a stored policy
// or the built-in default (p.IsDefault). Per AC-OFFICE-BUDGET-005.7/-004.6,
// a policy that blocked purely on pricing degradation (its limit was not
// also reached) carries the distinct run_budget_unmeasurable outcome and a
// distinct activity action; a plain limit block, including one where
// degradation also fired, keeps the existing budget_blocked outcome and
// action, with the degradation flag carried on the entry alone.
func (si *SchedulerIntegration) finishPolicyBlock(
	ctx context.Context, run *models.Run, agent *models.AgentInstance, p *models.PreLaunchPolicyResult,
) bool {
	si.releaseCheckoutIfNeeded(ctx, run)
	si.svc.clearAgentWorking(ctx, agent.ID, run.ID)
	provenance := shared.ClassifyRunProvenance(run.Reason)

	action := "run_budget_blocked"
	outcome := RunOutcomeBudgetBlocked
	if !p.LimitExceeded {
		action = "run_budget_pricing_degraded_blocked"
		outcome = RunOutcomeBudgetUnmeasurable
		incBudgetBlockedPricingDegraded(provenance)
	} else {
		incBudgetBlockedByLimit(provenance)
	}
	wrote, _ := si.svc.FinishRun(ctx, run.ID, outcome)
	if !wrote {
		// Another writer already moved the run out of claimed between the
		// admission decision and this write; it did not actually end via a
		// budget block, so there is nothing to log (Review round 3, R3-1).
		return false
	}

	fields := map[string]string{"degraded": strconv.FormatBool(p.Degraded)}
	if p.IsDefault {
		fields[activityFieldCeiling] = ceilingBuiltInDefault
	} else {
		fields["policy_id"] = p.PolicyID
	}
	si.svc.LogActivityWithRun(ctx, agent.WorkspaceID, "scheduler", "office-scheduler",
		action, "run", run.ID, mustJSON(fields), run.ID, "")
	return false
}

// cancelBudgetRun cancels run for a permanent, non-retryable admission
// outcome -- an unresolvable workspace, an absent evaluator on an
// unattended run, an unparseable payload, or a task that no longer exists
// -- mirroring cancelStaleRun's release/cancel/publish/log sequence. Always
// returns false.
func (si *SchedulerIntegration) cancelBudgetRun(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	reason, action string, extraFields map[string]string,
) bool {
	si.releaseCheckoutIfNeeded(ctx, run)
	si.svc.clearAgentWorking(ctx, agent.ID, run.ID)

	cancelled, err := si.svc.repo.CancelRun(ctx, run.ID, reason)
	if err != nil {
		si.logger.Error("failed to cancel run", zap.String("run_id", run.ID), zap.Error(err))
		return false
	}
	if !cancelled {
		// Another writer already moved the run out of claimed between the
		// admission decision and this write; it did not actually end via
		// this cancellation, so there is nothing to classify, publish, or
		// log.
		return false
	}

	si.svc.recordTerminalShape(ctx, run, RunStatusCancelled, nil)
	si.svc.publishRunProcessed(ctx, run.ID, RunStatusCancelled, run)

	fields := map[string]string{activityFieldCeiling: ceilingNotDetermined}
	for k, v := range extraFields {
		fields[k] = v
	}
	si.svc.LogActivityWithRun(ctx, agent.WorkspaceID, "scheduler", "office-scheduler",
		action, "run", run.ID, mustJSON(fields), run.ID, "")
	return false
}

// projectResolution enumerates the outcomes of resolving a run's project
// identifier for gate 4 (AC-OFFICE-BUDGET-006.7).
type projectResolution int

const (
	// projectResolutionNone covers outcome (a): a well-formed payload with
	// no task identifier, or a task that resolves with no project. Not a
	// fault; no project-scoped policy applies (AC-OFFICE-BUDGET-001.15).
	projectResolutionNone projectResolution = iota
	projectResolutionFound
	// projectResolutionLookupError covers outcome (b): GetTaskBasicInfo
	// returned an error. An evaluator fault, deferred under
	// AC-OFFICE-BUDGET-006.3.
	projectResolutionLookupError
	// projectResolutionUnparseable covers outcome (c): the payload cannot
	// be parsed as JSON. Must not be read as (a); the run is cancelled,
	// never retried or failed.
	projectResolutionUnparseable
	// projectResolutionTaskNotFound covers outcome (d): the lookup
	// succeeded but the named task does not exist. Cancelled, never
	// retried or failed.
	projectResolutionTaskNotFound
)

// resolveRunProject resolves a run's project identifier for gate 4,
// implementing AC-OFFICE-BUDGET-006.7's four-outcome split. It does not
// reuse extractProjectID (which collapses all four outcomes to "" for its
// other, non-admission callers) or ParseRunPayload (which silently
// swallows a JSON unmarshal error), because gate 4 must distinguish all
// four rather than treating a malformed payload as "no project".
func (si *SchedulerIntegration) resolveRunProject(
	ctx context.Context, payload string,
) (projectID string, res projectResolution) {
	if payload == "" || payload == "{}" {
		return "", projectResolutionNone
	}

	var parsed struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return "", projectResolutionUnparseable
	}
	if parsed.TaskID == "" {
		return "", projectResolutionNone
	}

	info, err := si.svc.repo.GetTaskBasicInfo(ctx, parsed.TaskID)
	if err != nil {
		return "", projectResolutionLookupError
	}
	if info == nil {
		return "", projectResolutionTaskNotFound
	}
	if info.ProjectID == "" {
		return "", projectResolutionNone
	}
	return info.ProjectID, projectResolutionFound
}

// budgetDeferralCause is one of the three AC-OFFICE-BUDGET-006.4 admission
// faults that share the evaluator fault's retry/backoff mechanics
// (AC-OFFICE-BUDGET-001.3/.16) but must stay distinguishable from it and
// from each other in their activity entries (AC-OFFICE-BUDGET-006.5). The
// workspace-lookup-error branch of AC-OFFICE-BUDGET-001.13 is deliberately
// NOT a fourth value here: it never reaches admitRun at all (processRun
// resolves it before admitRun is called, since no *models.AgentInstance
// exists yet to pass in), so it has its own dedicated pair,
// deferWorkspaceLookupFailure/cancelUnresolvableAgentRun, sharing this
// type's action-naming and no-escalation shape but not its cause value.
type budgetDeferralCause int

const (
	budgetDeferralEvaluatorFault budgetDeferralCause = iota
	budgetDeferralUnevaluatedPolicy
	budgetDeferralProjectLookupError
)

func (c budgetDeferralCause) deferredAction() string {
	switch c {
	case budgetDeferralUnevaluatedPolicy:
		return "run_budget_unevaluated_policy_deferred"
	case budgetDeferralProjectLookupError:
		return "run_budget_project_lookup_deferred"
	default:
		return "run_budget_evaluator_fault_deferred"
	}
}

func (c budgetDeferralCause) failedAction() string {
	switch c {
	case budgetDeferralUnevaluatedPolicy:
		return "run_budget_unevaluated_policy_failed"
	case budgetDeferralProjectLookupError:
		return "run_budget_project_lookup_failed"
	default:
		return "run_budget_evaluator_fault_failed"
	}
}

// admitBudgetDeferral defers (retries) run for one of the three
// AC-OFFICE-BUDGET-006.4 admission-fault causes, or fails it without
// escalation once retries are exhausted (AC-OFFICE-BUDGET-001.17). It
// reuses the existing scheduleRetry/isRetryStale machinery so retry_count,
// backoff and the staleness bound are shared with every other retry class
// (AC-OFFICE-BUDGET-001.16/.18), but never calls escalateFailure. policyID
// names the policy per AC-OFFICE-BUDGET-005.5/-006.5's naming rule and is
// "" for every cause but budgetDeferralUnevaluatedPolicy. Always returns
// false: every call site uses it as `return si.admitBudgetDeferral(...)`.
func (si *SchedulerIntegration) admitBudgetDeferral(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	cause budgetDeferralCause, policyID string,
) bool {
	si.releaseCheckoutIfNeeded(ctx, run)
	provenance := shared.ClassifyRunProvenance(run.Reason)

	if run.RetryCount >= MaxRetryCount {
		incBudgetFailedNoEscalation(cause, provenance)
		if err := si.svc.failRunNoEscalation(ctx, run, agent, cause, policyID); err != nil {
			si.logger.Error("failed to fail run without escalation",
				zap.String("run_id", run.ID), zap.Error(err))
		}
		return false
	}

	if stale, _ := isRetryStale(run); stale {
		incBudgetCancelledStaleDeferral(provenance)
		si.svc.LogActivityWithRun(ctx, agent.WorkspaceID, "scheduler", "office-scheduler",
			"run_budget_deferral_stale_cancelled", "run", run.ID,
			mustJSON(map[string]string{activityFieldCeiling: ceilingNotDetermined, "cause": "budget_deferral"}), run.ID, "")
	} else {
		switch cause {
		case budgetDeferralEvaluatorFault:
			incBudgetDeferredEvaluatorFault(provenance)
		case budgetDeferralUnevaluatedPolicy:
			incBudgetDeferredUnevaluatedPolicy(provenance)
		case budgetDeferralProjectLookupError:
			incBudgetDeferredProjectLookup(provenance)
		}
		fields := map[string]string{
			activityFieldCeiling: ceilingNotDetermined,
			"attempt":            strconv.Itoa(run.RetryCount + 1),
		}
		if policyID != "" {
			fields["policy_id"] = policyID
		}
		si.svc.LogActivityWithRun(ctx, agent.WorkspaceID, "scheduler", "office-scheduler",
			cause.deferredAction(), "run", run.ID, mustJSON(fields), run.ID, "")
	}

	if err := si.svc.scheduleRetry(ctx, run); err != nil {
		si.logger.Error("failed to schedule budget deferral retry",
			zap.String("run_id", run.ID), zap.Error(err))
	}
	return false
}

// cancelUnresolvableAgentRun implements AC-OFFICE-BUDGET-001.13's "lookup
// succeeded and found no agent instance" disposition, called from
// processRun before an *models.AgentInstance is available -- the run
// carries no workspace and therefore no ceiling that could ever be
// evaluated, so it is cancelled rather than retried or failed, and never
// escalated (AC-OFFICE-BUDGET-001.17). Shares its action with admitRun's
// gate 1 (agent found but WorkspaceID == ""), the other branch of the same
// disposition; unlike cancelBudgetRun's other callers, no agent is
// available here, so the activity entry carries no workspace scope.
func (si *SchedulerIntegration) cancelUnresolvableAgentRun(ctx context.Context, run *models.Run) {
	provenance := shared.ClassifyRunProvenance(run.Reason)
	incBudgetCancelledNoWorkspace(provenance)
	si.cleanupWorkspaceLookupRun(ctx, run)

	cancelled, err := si.svc.repo.CancelRun(ctx, run.ID, "no_resolvable_workspace")
	if err != nil {
		si.logger.Error("failed to cancel run", zap.String("run_id", run.ID), zap.Error(err))
		return
	}
	if !cancelled {
		return
	}

	si.svc.recordTerminalShape(ctx, run, RunStatusCancelled, nil)
	si.svc.publishRunProcessed(ctx, run.ID, RunStatusCancelled, run)
	si.svc.LogActivityWithRun(ctx, "", "scheduler", "office-scheduler",
		"run_budget_workspace_unresolvable", "run", run.ID,
		mustJSON(map[string]string{activityFieldCeiling: ceilingNotDetermined}), run.ID, "")
}

// deferWorkspaceLookupFailure implements AC-OFFICE-BUDGET-001.13's "lookup
// returned an error" disposition: deferred/retried on the same terms as an
// evaluator fault (AC-OFFICE-BUDGET-001.3/.16/.18), and at MaxRetryCount
// failed without escalation (AC-OFFICE-BUDGET-006.4) rather than through
// the generic HandleRunFailure/escalateFailure path, which would queue a
// new run for the CEO agent (AC-OFFICE-BUDGET-001.17). Mirrors
// admitBudgetDeferral's shape, but no *models.AgentInstance is available
// here (the lookup itself is what failed), so the activity entry carries
// no workspace scope. The run may still own a checkout from an earlier
// routed launch, so this path releases run-owned state before retrying.
func (si *SchedulerIntegration) deferWorkspaceLookupFailure(ctx context.Context, run *models.Run) {
	provenance := shared.ClassifyRunProvenance(run.Reason)
	si.cleanupWorkspaceLookupRun(ctx, run)

	if run.RetryCount >= MaxRetryCount {
		incBudgetFailedWorkspaceLookup(provenance)
		if _, err := si.svc.FailRun(ctx, run.ID); err != nil {
			si.logger.Error("failed to fail run without escalation",
				zap.String("run_id", run.ID), zap.Error(err))
			return
		}
		si.svc.LogActivityWithRun(ctx, "", "system", "scheduler",
			"run_budget_workspace_lookup_failed", "run", run.ID,
			mustJSON(map[string]string{activityFieldCeiling: ceilingNotDetermined}), run.ID, "")
		return
	}

	if stale, _ := isRetryStale(run); stale {
		incBudgetCancelledStaleDeferral(provenance)
		si.svc.LogActivityWithRun(ctx, "", "scheduler", "office-scheduler",
			"run_budget_deferral_stale_cancelled", "run", run.ID,
			mustJSON(map[string]string{activityFieldCeiling: ceilingNotDetermined, "cause": "workspace_lookup"}), run.ID, "")
	} else {
		incBudgetDeferredWorkspaceLookup(provenance)
		si.svc.LogActivityWithRun(ctx, "", "scheduler", "office-scheduler",
			"run_budget_workspace_lookup_deferred", "run", run.ID,
			mustJSON(map[string]string{
				activityFieldCeiling: ceilingNotDetermined,
				"attempt":            strconv.Itoa(run.RetryCount + 1),
			}), run.ID, "")
	}

	if err := si.svc.scheduleRetry(ctx, run); err != nil {
		si.logger.Error("failed to schedule workspace lookup retry",
			zap.String("run_id", run.ID), zap.Error(err))
	}
}

// cleanupWorkspaceLookupRun clears ownership retained by a routed run that
// was requeued after launch. The owner-scoped release is a no-op when this
// attempt never held a checkout, so a lookup failure cannot steal another
// run's lock.
func (si *SchedulerIntegration) cleanupWorkspaceLookupRun(ctx context.Context, run *models.Run) {
	si.releaseCheckoutIfNeeded(ctx, run)
	if run != nil {
		si.svc.clearAgentWorking(ctx, run.AgentProfileID, run.ID)
	}
}
