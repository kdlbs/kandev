package service

import (
	"expvar"

	"github.com/kandev/kandev/internal/office/shared"
)

// expvar maps for AC-OFFICE-BUDGET-005.4's nine required counters, in the
// same /debug/vars surface as the existing Office metrics (routing_*,
// cost_events_*). Mirrors scheduler/metrics_vars.go's shape: counters only,
// each labelled by run provenance (AC-OFFICE-BUDGET-007.1).
var (
	budgetBlockedByLimitTotal          = expvar.NewMap("office_budget_blocked_by_limit_total")
	budgetDeferredEvaluatorFaultTotal  = expvar.NewMap("office_budget_deferred_evaluator_fault_total")
	budgetBlockedAbsentEvaluatorTotal  = expvar.NewMap("office_budget_blocked_absent_evaluator_total")
	budgetDeferredWorkspaceLookupTotal = expvar.NewMap("office_budget_deferred_workspace_lookup_total")
	budgetCancelledNoWorkspaceTotal    = expvar.NewMap("office_budget_cancelled_no_workspace_total")
	budgetCancelledStaleDeferralTotal  = expvar.NewMap("office_budget_cancelled_stale_deferral_total")
	budgetBlockedPricingDegradedTotal  = expvar.NewMap("office_budget_blocked_pricing_degraded_total")
	budgetAdmittedDefaultTotal         = expvar.NewMap("office_budget_admitted_default_total")
	budgetAdmittedDegradedWindowTotal  = expvar.NewMap("office_budget_admitted_degraded_window_total")
)

// expvar maps for AC-OFFICE-BUDGET-006.6's seven additional counters, one
// per state AC-OFFICE-BUDGET-006.5 names beyond the nine above: two
// deferral-attempt counters for the causes AC-OFFICE-BUDGET-005.4's literal
// enumeration omits (unevaluated-policy, project-lookup-error --
// workspace-lookup deferral already has budgetDeferredWorkspaceLookupTotal
// above), three MaxRetryCount-failure counters, one per AC-OFFICE-BUDGET-
// 006.4 cause (the "evaluator error itself" is explicitly out of that
// criterion's scope, so it is not a fourth), and two AC-OFFICE-BUDGET-006.7
// cancellation counters. Kept in a separate var block from the nine above
// because they answer a distinct criterion, not a revision of it.
var (
	budgetDeferredUnevaluatedPolicyTotal = expvar.NewMap("office_budget_deferred_unevaluated_policy_total")
	budgetDeferredProjectLookupTotal     = expvar.NewMap("office_budget_deferred_project_lookup_total")
	budgetFailedWorkspaceLookupTotal     = expvar.NewMap("office_budget_failed_workspace_lookup_total")
	budgetFailedUnevaluatedPolicyTotal   = expvar.NewMap("office_budget_failed_unevaluated_policy_total")
	budgetFailedProjectLookupTotal       = expvar.NewMap("office_budget_failed_project_lookup_total")
	budgetCancelledUnparseablePayload    = expvar.NewMap("office_budget_cancelled_unparseable_payload_total")
	budgetCancelledTaskNotFoundTotal     = expvar.NewMap("office_budget_cancelled_task_not_found_total")
)

// provenanceLabel renders a run's provenance as the "provenance=..." expvar
// map key every counter in this file shares.
func provenanceLabel(p shared.RunProvenance) string {
	if p.Attended() {
		return "provenance=attended"
	}
	return "provenance=unattended"
}

func incBudgetBlockedByLimit(p shared.RunProvenance) {
	budgetBlockedByLimitTotal.Add(provenanceLabel(p), 1)
}

func incBudgetDeferredEvaluatorFault(p shared.RunProvenance) {
	budgetDeferredEvaluatorFaultTotal.Add(provenanceLabel(p), 1)
}

func incBudgetBlockedAbsentEvaluator(p shared.RunProvenance) {
	budgetBlockedAbsentEvaluatorTotal.Add(provenanceLabel(p), 1)
}

func incBudgetDeferredWorkspaceLookup(p shared.RunProvenance) {
	budgetDeferredWorkspaceLookupTotal.Add(provenanceLabel(p), 1)
}

func incBudgetCancelledNoWorkspace(p shared.RunProvenance) {
	budgetCancelledNoWorkspaceTotal.Add(provenanceLabel(p), 1)
}

func incBudgetCancelledStaleDeferral(p shared.RunProvenance) {
	budgetCancelledStaleDeferralTotal.Add(provenanceLabel(p), 1)
}

func incBudgetBlockedPricingDegraded(p shared.RunProvenance) {
	budgetBlockedPricingDegradedTotal.Add(provenanceLabel(p), 1)
}

func incBudgetAdmittedDefault(p shared.RunProvenance) {
	budgetAdmittedDefaultTotal.Add(provenanceLabel(p), 1)
}

// incBudgetAdmittedDegradedWindow implements AC-OFFICE-BUDGET-005.4's carved
// out rule for this one counter: callers must only invoke this at a point
// where the run's final disposition is already known to be launch, and only
// when at least one evaluated policy or the default was degraded-admitted
// (isDegradedAdmitted) -- never once per AC-OFFICE-BUDGET-004.8 activity
// entry, since a later gate can still block the run after that entry is
// written.
func incBudgetAdmittedDegradedWindow(p shared.RunProvenance) {
	budgetAdmittedDegradedWindowTotal.Add(provenanceLabel(p), 1)
}

func incBudgetDeferredUnevaluatedPolicy(p shared.RunProvenance) {
	budgetDeferredUnevaluatedPolicyTotal.Add(provenanceLabel(p), 1)
}

func incBudgetDeferredProjectLookup(p shared.RunProvenance) {
	budgetDeferredProjectLookupTotal.Add(provenanceLabel(p), 1)
}

func incBudgetFailedWorkspaceLookup(p shared.RunProvenance) {
	budgetFailedWorkspaceLookupTotal.Add(provenanceLabel(p), 1)
}

func incBudgetFailedUnevaluatedPolicy(p shared.RunProvenance) {
	budgetFailedUnevaluatedPolicyTotal.Add(provenanceLabel(p), 1)
}

func incBudgetFailedProjectLookup(p shared.RunProvenance) {
	budgetFailedProjectLookupTotal.Add(provenanceLabel(p), 1)
}

func incBudgetCancelledUnparseablePayload(p shared.RunProvenance) {
	budgetCancelledUnparseablePayload.Add(provenanceLabel(p), 1)
}

func incBudgetCancelledTaskNotFound(p shared.RunProvenance) {
	budgetCancelledTaskNotFoundTotal.Add(provenanceLabel(p), 1)
}

// incBudgetFailedNoEscalation increments the MaxRetryCount-failure counter
// matching cause, per AC-OFFICE-BUDGET-006.6's "not one counter shared
// across the three" rule. The evaluator-fault-proper cause is deliberately
// not routed here: AC-OFFICE-BUDGET-006.4 excludes "the evaluator error
// itself" from its own three named causes.
func incBudgetFailedNoEscalation(cause budgetDeferralCause, p shared.RunProvenance) {
	switch cause {
	case budgetDeferralUnevaluatedPolicy:
		incBudgetFailedUnevaluatedPolicy(p)
	case budgetDeferralProjectLookupError:
		incBudgetFailedProjectLookup(p)
	}
}
