package service

import (
	"expvar"

	"github.com/kandev/kandev/internal/office/shared"
)

// expvar maps for AC-OFFICE-BUDGET-005.4's nine required counters, in the
// same /debug/vars surface as the existing Office metrics (routing_*,
// cost_events_*). Mirrors scheduler/metrics_vars.go's shape: counters only,
// each labelled by run provenance (AC-OFFICE-BUDGET-007.1).
//
// Two of AC-OFFICE-BUDGET-006.4/-006.5's admission-fault causes --
// unevaluated-policy deferral and project-lookup-error deferral -- are not
// named in AC-OFFICE-BUDGET-005.4's literal nine-counter enumeration and so
// have no counter here; both remain individually observable via their own
// distinctly-actioned activity entries (AC-OFFICE-BUDGET-005.3/.5/.6,
// run_budget_unevaluated_policy_deferred / run_budget_project_lookup_deferred).
// Recorded as an accepted gap against the AC's literal text, not silently
// widened, consistent with this capability's Y1 disposition.
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
