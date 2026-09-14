package shared

import "github.com/kandev/kandev/internal/office/models"

// WakeReasonRegistry is the single declared enumeration of every wake
// reason value the system persists on runs.reason, including reasons kept
// only to read historical rows (AC-OFFICE-BACKPRESSURE-001.4/.8). Every
// reason constant declared elsewhere in the codebase (office/service/run.go,
// office/scheduler/run.go, office/service/failure.go, this package's own
// RunReasonRoutineDispatch* family) must have its string value present here
// — those packages' exported constants are aliases of, or literal
// duplicates of, an entry in this map, never an independent declaration of
// a new reason. Adding a new wake reason means adding it here first.
//
// Reasons that map to a class only via a rule other than the reason itself
// (a human actor, or a re-queue) are deliberately absent from this map:
// PriorityClassForReason never needs to resolve them, because
// REQ-OFFICE-BACKPRESSURE-001.3 stops at the first matching rule before the
// reason is consulted.
var WakeReasonRegistry = map[string]models.PriorityClass{
	// Periodic: unattended, schedule-driven fires only. A row whose
	// originating trigger cannot be recovered (RunReasonRoutineDispatch)
	// is deliberately excluded — see the comment on that constant.
	"routine_dispatch_cron": models.PriorityClassPeriodic,
	"heartbeat":             models.PriorityClassPeriodic,

	// Event: every other reason in the registry, by explicit rule rather
	// than by falling through the unmapped fallback.
	"task_assigned":               models.PriorityClassEvent,
	"task_comment":                models.PriorityClassEvent,
	"task_blockers_resolved":      models.PriorityClassEvent,
	"task_children_completed":     models.PriorityClassEvent,
	"approval_resolved":           models.PriorityClassEvent,
	"task_review_requested":       models.PriorityClassEvent,
	"task_changes_requested":      models.PriorityClassEvent,
	"routine_trigger":             models.PriorityClassEvent,
	"budget_alert":                models.PriorityClassEvent,
	"agent_error":                 models.PriorityClassEvent,
	"routine_dispatch_event":      models.PriorityClassEvent,
	"routine_dispatch":            models.PriorityClassEvent,
	"manual_resume_after_failure": models.PriorityClassEvent,

	// Reactivity-pipeline reasons (office/scheduler/run.go's
	// RunReasonTaskUnblocked family). All are triggered by a task-state
	// change (a human comment, a blocker resolving, a reviewer decision),
	// never by an unattended schedule, so all classify as event.
	"task_unblocked":            models.PriorityClassEvent,
	"task_reopened":             models.PriorityClassEvent,
	"task_reopened_via_comment": models.PriorityClassEvent,
	"task_mentioned":            models.PriorityClassEvent,
	"stage_pending":             models.PriorityClassEvent,
	"stage_changes_requested":   models.PriorityClassEvent,
	"task_ready_to_close":       models.PriorityClassEvent,

	// Historical-row-only literals, retained so an old persisted run row
	// still resolves through an explicit rule rather than the fallback.
	"blockers_resolved":  models.PriorityClassEvent,
	"children_completed": models.PriorityClassEvent,
	"review_started":     models.PriorityClassEvent,
	"approval_started":   models.PriorityClassEvent,
}

// ReasonOnlyHumanCanCause names wake reasons whose priority class is
// `human` regardless of the recorded actor, per
// AC-OFFICE-BACKPRESSURE-001.3 rule 2. Kept separate from
// WakeReasonRegistry: these reasons are also present there (an
// unrecognized-actor row must still resolve them through the registry,
// not lose them entirely), but PriorityClassForRun consults this set
// first, before the registry.
var ReasonOnlyHumanCanCause = map[string]struct{}{
	"manual_resume_after_failure": {},
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
func PriorityClassForReason(reason string) (models.PriorityClass, bool) {
	class, ok := WakeReasonRegistry[reason]
	return class, ok
}
