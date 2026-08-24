package shared

// Run reason constants shared between routine writers (internal/office/routines)
// and scheduler readers (internal/office/service) so the two cannot silently
// drift out of sync with each other.
const (
	// RunReasonRoutineDispatch is set by RoutineService when it materializes
	// a lightweight (taskless) routine wakeup triggered by its cron
	// schedule — the only routine trigger that represents a periodic,
	// unattended wake. See RoutineDispatchReason.
	RunReasonRoutineDispatch = "routine_dispatch"

	// RunReasonRoutineDispatchEvent is set by RoutineService when a
	// lightweight routine fires from a manual "Fire now" or an inbound
	// webhook. Both are event/user-triggered, not periodic — per
	// docs/specs/office/scheduler.md ("Event-triggered wakeups always
	// proceed - the skip applies only to periodic wakes") a run carrying
	// this reason must never be treated as a skippable idle wake.
	RunReasonRoutineDispatchEvent = "routine_dispatch_event"

	// RunReasonHeartbeat is retired: the agent-level heartbeat cron was
	// replaced by the coordinator-heartbeat routine, so no production writer
	// sets it any more. Kept so any pre-retirement run row still queued is
	// still recognized as a periodic taskless wake.
	RunReasonHeartbeat = "heartbeat"

	// RoutineSourceCron identifies a routine fire triggered by its cron
	// schedule (RoutineRun.Source). Defined here — rather than as a
	// literal at the producer call site — so RoutineDispatchReason and
	// its one production caller (routines.processCronTrigger) share a
	// single source of truth and cannot drift apart the way
	// RunReasonHeartbeat did.
	RoutineSourceCron = "cron"
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

// RoutineDispatchReason returns the run-reason value a lightweight
// routine wakeup should carry for the given RoutineRun.Source. Only a
// cron-driven fire is periodic; a manual "Fire now" or an inbound
// webhook fire is event/user-triggered and must always proceed even
// when the assignee has SkipIdleRuns set — see IsPeriodicTasklessWake.
func RoutineDispatchReason(source string) string {
	if source == RoutineSourceCron {
		return RunReasonRoutineDispatch
	}
	return RunReasonRoutineDispatchEvent
}
