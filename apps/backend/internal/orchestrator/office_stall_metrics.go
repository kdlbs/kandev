package orchestrator

import (
	"expvar"
	"strings"
)

// expvar maps for the Office stall detectors, exposed via stdlib's
// /debug/vars handler. The label model is "key=value;key=value..." so a
// Prometheus translation layer can split on `;` and `=` later, mirroring
// internal/office/scheduler/metrics_vars.go.
//
// Counters only. A consumer that wants a snapshot of what is currently stuck
// re-runs the detectors' own predicates against the database; these record
// events.
var (
	// officeStallStrandedSignalTotal counts Office tasks observed holding a
	// stranded step-completion signal. Labelled by `gate` so it is visible
	// which of the two watchdog sites observed it.
	officeStallStrandedSignalTotal = expvar.NewMap("office_stall_stranded_signal_total")

	// officeStallDecisionWaitingTotal counts Office tasks observed parked at
	// a decision-required step with no run in flight.
	officeStallDecisionWaitingTotal = expvar.NewMap("office_stall_decision_waiting_total")

	// officeStallDetectorSkippedTotal counts evaluations abandoned because an
	// input could not be read. Both detectors fail closed, so without this a
	// silently degraded detector is indistinguishable from a quiet system —
	// which is the failure mode that matters for a surface-only capability.
	officeStallDetectorSkippedTotal = expvar.NewMap("office_stall_detector_skipped_total")
)

// Skip reasons for officeStallDetectorSkippedTotal.
const (
	officeStallSkipRunReaderUnwired      = "run_reader_unwired"
	officeStallSkipRunReaderError        = "run_reader_error"
	officeStallSkipParticipantStore      = "participant_store_unwired"
	officeStallSkipDecisionStore         = "decision_store_unwired"
	officeStallSkipParticipantReadFailed = "participant_read_failed"
	officeStallSkipDecisionReadFailed    = "decision_read_failed"
	officeStallSkipCandidateListUnwired  = "candidate_lister_unwired"
	officeStallSkipCandidateListFailed   = "candidate_list_failed"
)

// officeStallLabel builds a "k1=v1;k2=v2;..." expvar map key. Returns an
// empty label for an odd number of arguments rather than guessing.
func officeStallLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// officeStallSkipped records a fail-closed skip from either detector.
func officeStallSkipped(reason string) {
	officeStallDetectorSkippedTotal.Add(officeStallLabel("reason", reason), 1)
}
