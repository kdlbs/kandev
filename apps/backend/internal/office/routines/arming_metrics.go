package routines

import (
	"expvar"
	"strings"
)

// expvar counters for the routine arming startup scan, exposed via stdlib's
// /debug/vars handler. The label model is "key=value;key=value..." so a
// Prometheus translation layer can split on `;` and `=` later, mirroring
// internal/orchestrator/office_stall_metrics.go.
var (
	// armingScanObservationsTotal counts every routine the scan classified,
	// labelled by workspace, intent, and schedule state
	// (AC-OFFICE-ROUTINE-ARMING-003.6).
	armingScanObservationsTotal = expvar.NewMap("office_routine_arming_scan_observations_total")

	// armingScanSkippedTotal counts a scan step abandoned because an input
	// could not be read, labelled by reason, so a scan failing closed is
	// visible rather than silent (AC-OFFICE-ROUTINE-ARMING-003.7).
	armingScanSkippedTotal = expvar.NewMap("office_routine_arming_scan_skipped_total")
)

// Skip reasons for armingScanSkippedTotal.
const (
	armingSkipNoSignal              = "no_signal"
	armingSkipWorkspaceEnumFailed   = "workspace_enum_failed"
	armingSkipRoutineEnumFailed     = "routine_enum_failed"
	armingSkipClassificationUnknown = "classification_unknown"
)

// armingLabel builds a "k1=v1;k2=v2;..." expvar map key. Returns an empty
// label for an odd number of arguments rather than guessing.
func armingLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// armingScanObserved records one routine the scan reached.
func armingScanObserved(workspaceID, intent string, state ScheduleState) {
	armingScanObservationsTotal.Add(
		armingLabel("workspace", workspaceID, "intent", intent, "schedule_state", string(state)), 1)
}

// armingScanSkipped records a fail-closed skip in the startup scan.
func armingScanSkipped(reason string) {
	armingScanSkippedTotal.Add(armingLabel("reason", reason), 1)
}
