package quorummetrics

import "expvar"

// workflowQuorumSlateEmpty counts one occurrence, per role, of a quorum
// guard's required slate coming back empty — no decision-required seat
// resolved for that role at the evaluating step
// (REQ-OFFICE-REVIEW-SEATS-004.2). It is distinct from
// workflowQuorumGuardNotFired{reason="slate_empty"}: that counter tracks
// every AC-23 not-fired reason across every guard variant, including
// any_reject (which never reads the seat slate); this one exists solely so
// a dashboard can see which role's slate is empty without decoding a
// shared reason label.
var workflowQuorumSlateEmpty = expvar.NewMap("workflow_quorum_slate_empty_total")

// RecordQuorumSlateEmpty counts one guard evaluation whose required slate
// for role came back empty.
func RecordQuorumSlateEmpty(role string) {
	workflowQuorumSlateEmpty.Add(role, 1)
}
