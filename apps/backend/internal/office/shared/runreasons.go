package shared

// Run reason constants shared between routine writers (internal/office/routines)
// and scheduler readers (internal/office/service) so the two cannot silently
// drift out of sync with each other.
const (
	// RunReasonRoutineDispatch is set by RoutineService when it materializes
	// a lightweight (taskless) routine wakeup.
	RunReasonRoutineDispatch = "routine_dispatch"

	// RunReasonHeartbeat is retired: the agent-level heartbeat cron was
	// replaced by the coordinator-heartbeat routine, so no production writer
	// sets it any more. Kept so any pre-retirement run row still queued is
	// still recognized as a periodic taskless wake.
	RunReasonHeartbeat = "heartbeat"
)

// IsPeriodicTasklessWake reports whether reason represents a periodic,
// taskless wake — the class of run the idle-skip gate is allowed to skip
// when the agent has no actionable tasks assigned.
func IsPeriodicTasklessWake(reason string) bool {
	switch reason {
	case RunReasonRoutineDispatch, RunReasonHeartbeat:
		return true
	default:
		return false
	}
}
