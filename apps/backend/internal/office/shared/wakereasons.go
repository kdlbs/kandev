package shared

import runsmodels "github.com/kandev/kandev/internal/runs/models"

// WakeReasonRegistry is the single declared enumeration of every wake
// reason value the system persists on runs.reason, including reasons kept
// only to read historical rows (AC-OFFICE-BACKPRESSURE-001.4/.8). Every
// reason's canonical string constant is declared in this package (here or
// in runreasons.go) — office/service/run.go, office/scheduler/run.go, and
// office/service/failure.go declare only aliases of these constants, never
// an independent raw-literal declaration of a new or existing reason.
// Adding a new wake reason means adding its canonical constant here first,
// then aliasing it from whichever package produces it.
//
// Reasons that map to a class only via a rule other than the reason itself
// (a human actor, or a re-queue) are deliberately absent from this map:
// PriorityClassForReason never needs to resolve them, because
// REQ-OFFICE-BACKPRESSURE-001.3 stops at the first matching rule before the
// reason is consulted.
var WakeReasonRegistry = map[string]runsmodels.PriorityClass{
	// Periodic: unattended, schedule-driven fires only. A row whose
	// originating trigger cannot be recovered (RunReasonRoutineDispatch)
	// is deliberately excluded — see the comment on that constant.
	RunReasonRoutineDispatchCron: runsmodels.PriorityClassPeriodic,
	RunReasonHeartbeat:           runsmodels.PriorityClassPeriodic,

	// Event: every other reason in the registry, by explicit rule rather
	// than by falling through the unmapped fallback.
	RunReasonTaskAssigned:             runsmodels.PriorityClassEvent,
	RunReasonTaskComment:              runsmodels.PriorityClassEvent,
	RunReasonTaskBlockersResolved:     runsmodels.PriorityClassEvent,
	RunReasonTaskChildrenCompleted:    runsmodels.PriorityClassEvent,
	RunReasonApprovalResolved:         runsmodels.PriorityClassEvent,
	RunReasonTaskReviewRequested:      runsmodels.PriorityClassEvent,
	RunReasonTaskChangesRequested:     runsmodels.PriorityClassEvent,
	RunReasonRoutineTrigger:           runsmodels.PriorityClassEvent,
	RunReasonBudgetAlert:              runsmodels.PriorityClassEvent,
	RunReasonAgentError:               runsmodels.PriorityClassEvent,
	RunReasonRoutineDispatchEvent:     runsmodels.PriorityClassEvent,
	RunReasonRoutineDispatch:          runsmodels.PriorityClassEvent,
	RunReasonManualResumeAfterFailure: runsmodels.PriorityClassEvent,
	RunReasonQueueRun:                 runsmodels.PriorityClassEvent,

	// Reactivity-pipeline reasons (office/scheduler/run.go's
	// RunReasonTaskUnblocked family). All are triggered by a task-state
	// change (a human comment, a blocker resolving, a reviewer decision),
	// never by an unattended schedule, so all classify as event.
	RunReasonTaskUnblocked:         runsmodels.PriorityClassEvent,
	RunReasonTaskReopened:          runsmodels.PriorityClassEvent,
	RunReasonTaskReopenedComment:   runsmodels.PriorityClassEvent,
	RunReasonTaskMentioned:         runsmodels.PriorityClassEvent,
	RunReasonStagePending:          runsmodels.PriorityClassEvent,
	RunReasonStageChangesRequested: runsmodels.PriorityClassEvent,
	RunReasonTaskReadyToClose:      runsmodels.PriorityClassEvent,

	// Historical-row-only literals, retained so an old persisted run row
	// still resolves through an explicit rule rather than the fallback.
	RunReasonLegacyBlockersResolved:  runsmodels.PriorityClassEvent,
	RunReasonLegacyChildrenCompleted: runsmodels.PriorityClassEvent,
	RunReasonLegacyReviewStarted:     runsmodels.PriorityClassEvent,
	RunReasonLegacyApprovalStarted:   runsmodels.PriorityClassEvent,
}

// ReasonOnlyHumanCanCause names wake reasons whose priority class is
// `human` regardless of the recorded actor, per
// AC-OFFICE-BACKPRESSURE-001.3 rule 2. Kept separate from
// WakeReasonRegistry: these reasons are also present there (an
// unrecognized-actor row must still resolve them through the registry,
// not lose them entirely), but PriorityClassForRun consults this set
// first, before the registry.
var ReasonOnlyHumanCanCause = map[string]struct{}{
	RunReasonManualResumeAfterFailure: {},
}

// PriorityClassForReason maps a wake reason to a priority class using only
// the reason-decides step of AC-OFFICE-BACKPRESSURE-001.3 (rule 4). It does
// not apply rules 1-3 (actor, human-only reason, re-queue) — callers apply
// those first and only fall through to this function when none matched.
// The second return value is false when the reason is not in the registry,
// per AC-OFFICE-BACKPRESSURE-001.6: the caller assigns `event` and
// increments the unmapped counter itself rather than this function
// silently defaulting, so the two failure paths (registry miss vs.
// recognized-but-mapped-to-event) stay distinguishable to a caller that
// wants to count them separately.
func PriorityClassForReason(reason string) (runsmodels.PriorityClass, bool) {
	class, ok := WakeReasonRegistry[reason]
	return class, ok
}
