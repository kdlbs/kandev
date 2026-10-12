package shared

// Run reason constants shared between routine writers (internal/office/routines)
// and scheduler readers (internal/office/service) so the two cannot silently
// drift out of sync with each other.
const (
	// RunReasonRoutineDispatch is the legacy literal written by every
	// pre-cron-constant build for a lightweight routine dispatch,
	// regardless of trigger (cron, manual, or webhook alike). Because
	// the source cannot be recovered from an already-persisted run row,
	// it is treated as non-skippable rather than guessed — see
	// IsPeriodicTasklessWake. Still the value new code must recognize
	// on any run row queued before an upgrade; do not delete.
	RunReasonRoutineDispatch = "routine_dispatch"

	// RunReasonRoutineDispatchCron is set by RoutineService when it
	// materializes a lightweight (taskless) routine wakeup triggered by
	// its cron schedule — the only routine trigger that represents a
	// periodic, unattended wake. See RoutineDispatchReason.
	RunReasonRoutineDispatchCron = "routine_dispatch_cron"

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

	// The remaining wake-reason values below are canonical here per
	// AC-OFFICE-BACKPRESSURE-001.8: this package is the single place a
	// wake-reason constant is declared. Consumers alias these (e.g.
	// RunReasonTaskAssigned = shared.RunReasonTaskAssigned) rather than
	// re-declaring the string, so a reason cannot silently exist under two
	// unlinked declarations and drop out of WakeReasonRegistry unnoticed.
	// TestWakeReasonRegistry_DeclaresEveryReasonOnlyInSharedPackage fails
	// on a new raw-literal declaration outside this package.
	RunReasonTaskAssigned             = "task_assigned"
	RunReasonTaskComment              = "task_comment"
	RunReasonTaskBlockersResolved     = "task_blockers_resolved"
	RunReasonTaskChildrenCompleted    = "task_children_completed"
	RunReasonApprovalResolved         = "approval_resolved"
	RunReasonTaskReviewRequested      = "task_review_requested"
	RunReasonTaskChangesRequested     = "task_changes_requested"
	RunReasonRoutineTrigger           = "routine_trigger"
	RunReasonBudgetAlert              = "budget_alert"
	RunReasonAgentError               = "agent_error"
	RunReasonManualResumeAfterFailure = "manual_resume_after_failure"

	// RunReasonQueueRun is internal/workflow/engine's defaultQueueReasonR:
	// the workflow engine's queue_run action falls back to this value when
	// the action configures no reason and the triggering event carries no
	// trigger name. Declared here rather than aliased from that package
	// because internal/office/shared imports internal/workflow/engine
	// (QueueRunCallback's RunQueueAdapter dependency), so the reverse
	// import needed to alias it would cycle;
	// TestDefaultQueueReasonResolvesInWakeReasonRegistry in
	// internal/workflow/engine keeps the two literals in sync instead.
	RunReasonQueueRun = "queue_run"

	// Reactivity-pipeline reasons (office/scheduler's reactivity.go).
	RunReasonTaskUnblocked         = "task_unblocked"
	RunReasonTaskReopened          = "task_reopened"
	RunReasonTaskReopenedComment   = "task_reopened_via_comment"
	RunReasonTaskMentioned         = "task_mentioned"
	RunReasonStagePending          = "stage_pending"
	RunReasonStageChangesRequested = "stage_changes_requested"
	RunReasonTaskReadyToClose      = "task_ready_to_close"

	// Historical-row-only literals, retained so an old persisted run row
	// still resolves through an explicit rule rather than the fallback.
	RunReasonLegacyBlockersResolved  = "blockers_resolved"
	RunReasonLegacyChildrenCompleted = "children_completed"
	RunReasonLegacyReviewStarted     = "review_started"
	RunReasonLegacyApprovalStarted   = "approval_started"
)

// IsPeriodicTasklessWake reports whether reason represents a periodic,
// taskless wake — the class of run the idle-skip gate is allowed to skip
// when the agent has no actionable tasks assigned. The legacy
// RunReasonRoutineDispatch literal is deliberately excluded: it was
// written for cron, manual, and webhook fires alike before
// RunReasonRoutineDispatchCron existed, so its trigger cannot be
// recovered from the run row and it defaults to non-skippable.
func IsPeriodicTasklessWake(reason string) bool {
	switch reason {
	case RunReasonRoutineDispatchCron, RunReasonHeartbeat:
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
		return RunReasonRoutineDispatchCron
	}
	return RunReasonRoutineDispatchEvent
}
