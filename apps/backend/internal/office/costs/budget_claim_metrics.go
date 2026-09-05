package costs

import "expvar"

// budgetClaimFailuresTotal counts claim-store failures encountered while
// evaluating a budget policy, exported through the stdlib expvar surface at
// /debug/vars in dev mode. Shaped as an expvar.Map, matching
// cost_events_written_total / cost_events_dropped_total in
// internal/office/service/cost_metrics.go, rather than a plain Int, so
// /debug/vars stays consistent. The "op" label has exactly one value,
// "claim": it covers both a policy's own level claim and its companion
// alert claim, and does not grow — a failed claim discard during a policy
// update is a different, already non-silent failure and is deliberately
// not counted here.
var budgetClaimFailuresTotal = expvar.NewMap("budget_claim_failures_total")

const budgetClaimFailureLabel = "op=claim"
