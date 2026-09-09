package routines

import "time"

// MarshalRoutinePayloadForTest exposes marshalRoutinePayload to external
// test packages so cross-package tests (e.g. the prompt builder's gap
// rendering, AC-OFFICE-ROUTINE-CATCHUP-002.5) can round-trip through the
// real wakeup-request wire format instead of a hand-written JSON literal
// that could silently drift from what dispatch actually sends.
func MarshalRoutinePayloadForTest(
	routineID string, vars map[string]string, missedTicks int, firstMissed time.Time, truncated bool,
) (string, error) {
	var gap *gapSummary
	if missedTicks > 0 {
		gap = &gapSummary{MissedTicks: missedTicks, FirstMissed: firstMissed, Truncated: truncated}
	}
	return marshalRoutinePayload(routineID, vars, gap)
}
