package shared

import (
	"expvar"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
)

// expvar maps for the launch-safety and backpressure capability
// (docs/specs/office/requirements/unattended-launch-safety.md,
// launch-budgets.md, launch-backpressure.md, run-causation-chain.md),
// exposed via /debug/vars. Label idiom matches
// internal/orchestrator/office_stall_metrics.go: "k1=v1;k2=v2".
var (
	// LaunchDeferredTotal counts a claim attempt that returned no row
	// while eligible queued runs existed, labelled by the attributed
	// deferral gate (agent_ceiling, workspace_ceiling, instance_ceiling,
	// workspace_budget, routine_budget).
	LaunchDeferredTotal = expvar.NewMap("office_launch_deferred_total")

	// LaunchRefusedTotal counts an enqueue refused by a gate, labelled by
	// the refusing gate (causation_depth, self_trigger,
	// causing_run_unreadable, workspace_missing).
	LaunchRefusedTotal = expvar.NewMap("office_launch_refused_total")

	// LaunchCheckFailedTotal counts a gate that failed closed because an
	// input was unreadable, labelled by gate, so a gate failing closed is
	// distinguishable from a quiet or a correctly-blocking system.
	LaunchCheckFailedTotal = expvar.NewMap("office_launch_check_failed_total")

	// LaunchPriorityUnmappedTotal counts a wake reason that resolved
	// through the unmapped fallback rather than an explicit registry
	// rule, labelled by reason.
	LaunchPriorityUnmappedTotal = expvar.NewMap("office_launch_priority_unmapped_total")

	// LaunchActorMissingTotal counts an enqueue whose actor was absent or
	// unrecognized and was recorded as `system`, labelled by reason.
	LaunchActorMissingTotal = expvar.NewMap("office_launch_actor_missing_total")

	// LaunchCausationInvalidTotal counts a malformed task-boundary carrier
	// value resolved to its most restrictive reading, labelled by reason
	// (AC-OFFICE-RUN-CAUSATION-001.10).
	LaunchCausationInvalidTotal = expvar.NewMap("office_launch_causation_invalid_total")

	// GateOutcomeRecordFailedTotal counts a failure to persist a gate's
	// consecutive-failure state (RecordGateOutcome/RecordGateOutcomeTx),
	// labelled by gate. Per AC-OFFICE-BACKPRESSURE-003.4 this failure must
	// never affect the admission decision, so it is only ever counted, not
	// propagated as an error the caller acts on.
	GateOutcomeRecordFailedTotal = expvar.NewMap("office_gate_outcome_record_failed_total")

	// LaunchClaimScanCapHitTotal counts a ClaimNextEligibleRun attempt
	// that exhausted its candidate-scan safety valve (claimCandidateScanCap
	// in internal/runs/repository/sqlite/claim.go) without finding a
	// claimable row or reaching the end of the queued set. Unlabelled: a
	// single scalar counter is enough to make an otherwise-invisible
	// pathological backlog observable.
	LaunchClaimScanCapHitTotal = expvar.NewInt("office_launch_claim_scan_cap_hit_total")
)

// LaunchSafetyLabel builds a "k1=v1;k2=v2;..." expvar map key, the same
// idiom as internal/orchestrator/office_stall_metrics.go's
// officeStallLabel. Returns an empty label for an odd number of
// arguments rather than guessing.
func LaunchSafetyLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// ClassifyPriority applies AC-OFFICE-BACKPRESSURE-001.3 in full: actor,
// then a human-only reason, then re-queue, then the reason registry,
// stopping at the first match. isRequeue is true for a run re-stamped by
// retry, the recovery sweep, or routing re-dispatch
// (AC-OFFICE-BACKPRESSURE-001.7) — callers must not apply it to a human
// class already persisted; that "unless already human" check is the
// caller's responsibility because only the caller holds the run's current
// persisted class.
func ClassifyPriority(actorKind models.ActorKind, reason string, isRequeue bool) models.PriorityClass {
	if actorKind == models.ActorKindUser {
		return models.PriorityClassHuman
	}
	if _, ok := ReasonOnlyHumanCanCause[reason]; ok {
		return models.PriorityClassHuman
	}
	if isRequeue {
		return models.PriorityClassRecovery
	}
	if class, ok := PriorityClassForReason(reason); ok {
		return class
	}
	LaunchPriorityUnmappedTotal.Add(LaunchSafetyLabel("reason", reason), 1)
	return models.PriorityClassEvent
}
