package pause

import "expvar"

// Office pause/resume counters, matching the existing office_* expvar
// convention (see internal/office/routing metrics). office_pause_blocked_total
// and office_pause_gate_error_total are per-gate maps so a gate that never
// increments while a workspace is paused and active is visibly unreachable
// or unwired, rather than silently absent from a flat counter.
var (
	pauseCreatedTotal  = expvar.NewInt("office_pause_created_total")
	pauseReleasedTotal = expvar.NewInt("office_pause_released_total")
	pauseBlockedTotal  = expvar.NewMap("office_pause_blocked_total")
	gateErrorTotal     = expvar.NewMap("office_pause_gate_error_total")
)

// RecordBlocked increments the per-gate blocked counter. Call from a gate
// site when PauseState reports a confirmed pause.
func RecordBlocked(gate string) {
	pauseBlockedTotal.Add(gate, 1)
}

// RecordGateError increments the per-gate read-error counter. Call from a
// gate site when PauseState itself fails.
func RecordGateError(gate string) {
	gateErrorTotal.Add(gate, 1)
}
