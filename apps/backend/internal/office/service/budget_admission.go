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
	// itself already happened in processRun via GetAgentFromConfig; per the
	// recorded W2 human disposition that call site's error contract is not
	// reshaped, so a lookup ERROR never reaches admitRun (processRun already
	// returned early on it, via the pre-existing escalating
	// HandleRunFailure path). Only the "found an agent with no workspace
	// identifier" branch is reachable and decidable here, from the
	// already-fetched agent -- no second lookup is performed.
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
	// project identifier is resolved here, after gate 3 has a chance to
	// succeed but before any policy is evaluated (AC-OFFICE-BUDGET-006.7).
	projectID, res := si.resolveRunProject(ctx, run.Payload)
	switch res {
	case projectResolutionLookupError:
		return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralProjectLookupError, "")
	case projectResolutionUnparseable:
		return si.cancelBudgetRun(ctx, run, agent, "unparseable_payload",
			"run_budget_payload_unparseable", nil)
	case projectResolutionTaskNotFound:
		return si.cancelBudgetRun(ctx, run, agent, "task_not_found",
			"run_budget_task_not_found", nil)
	}
	hasProject := res == projectResolutionFound

	now := time.Now().UTC()
	result, err := si.svc.EvaluatePreLaunch(ctx, agent.WorkspaceID, agent.ID, projectID, hasProject, provenance, now)
	if err != nil {
		var upErr *models.UnevaluatedPolicyError
		if errors.As(err, &upErr) {
			return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralUnevaluatedPolicy, upErr.PolicyID)
		}
		return si.admitBudgetDeferral(ctx, run, agent, budgetDeferralEvaluatorFault, "")
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
	_ = si.svc.FinishRun(ctx, run.ID, outcome)

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

	if err := si.svc.repo.CancelRun(ctx, run.ID, reason); err != nil {
		si.logger.Error("failed to cancel run", zap.String("run_id", run.ID), zap.Error(err))
	} else {
		si.svc.publishRunProcessed(ctx, run.ID, RunStatusCancelled, run)
	}

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
// NOT a fourth value here: per the recorded W2 human disposition,
// GetAgentFromConfig's error contract is not reshaped, so that branch is
// indistinguishable, at its call site, from any other GetAgentFromConfig
// failure and continues to ride the pre-existing HandleRunFailure/
// escalateFailure path -- it never reaches admitRun or this type.
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
		if cause == budgetDeferralEvaluatorFault {
			incBudgetDeferredEvaluatorFault(provenance)
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
