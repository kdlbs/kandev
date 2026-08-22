// Package quorummetrics publishes the AC-24a expvar counter for guarded
// workflow transitions that evaluated but did not fire. It is a neutral
// package so the workflow engine can record the outcome without depending on
// the office or orchestrator packages that read /debug/vars.
package quorummetrics

import "expvar"

// workflowQuorumGuardNotFired counts one AC-23 reason each time a guarded
// transition (wait_for_quorum) is evaluated and does not fire, on the
// engine's transition-evaluation path — both the ordinary HandleTrigger
// path and the AC-65-scoped decision re-evaluation path. It also counts the
// AC-16a/F39 session_unresolvable skip, which is a recording-time skip
// rather than a guard-evaluation outcome. The AC-24b/57d read-only
// diagnostic snapshot never increments this counter.
var workflowQuorumGuardNotFired = expvar.NewMap("workflow_quorum_guard_not_fired_total")

// RecordGuardNotFired counts one occurrence of the given AC-23 reason code.
func RecordGuardNotFired(reason string) {
	workflowQuorumGuardNotFired.Add(reason, 1)
}
