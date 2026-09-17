package service

import (
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// QueueOutcomeNone means no enqueue was attempted, or the attempt returned
// an error. It is the zero value, so it is what a widened signature yields
// on any path that returns before deciding an outcome.
//
// It is declared identically in internal/workflow/engine/adapters.go — both
// declarations MUST match, the same invariant QueueOutcome itself already
// carries.
const QueueOutcomeNone QueueOutcome = ""

// QueueSource distinguishes which queue implementation made a dedup
// decision, for the "queue" label on office_run_dedup_total.
type QueueSource string

const (
	// QueueSourceRuns identifies a suppression on the runs table (the
	// idx_run_idempotency windowed lookup or durable unique index).
	QueueSourceRuns QueueSource = "runs"
	// QueueSourceWakeup identifies a suppression on the wakeup-request
	// table. That table has no windowed lookup, so its kind is always
	// "durable".
	QueueSourceWakeup QueueSource = "wakeup"
)

// KeylessCause discriminates why a producer enqueued a wake with no dedup
// key: a generation that should have been available and was not
// (KeylessCauseUnresolved), or a producer that never had an occurrence to
// name (KeylessCauseByDesign). See
// docs/specs/office/system-design/run-dedup-generation-01.md#unresolvable-generation.
type KeylessCause string

const (
	// KeylessCauseUnresolved means the occurrence has a generation that this
	// producer simply was not handed or could not resolve.
	KeylessCauseUnresolved KeylessCause = "unresolved"
	// KeylessCauseByDesign means the producer never had an occurrence
	// identity to name — a status transition with no redelivery path, an
	// agent that expressed no dedup intent, and similar.
	KeylessCauseByDesign KeylessCause = "by_design"
)

func dedupLogger() *logger.Logger {
	return logger.Default().WithFields(zap.String("component", "runs-dedup"))
}

// ReportWindowedDedup records a suppression caught by the recent-duplicate
// lookup (a windowed dedup hit): counts kind="windowed", logs at Info, and
// returns QueueOutcomeDeduped.
func ReportWindowedDedup(q QueueSource, reason, key string) QueueOutcome {
	incRunDedup(q, reason, "windowed")
	dedupLogger().Info("run deduplicated (windowed)",
		zap.String("queue", string(q)),
		zap.String("reason", reason),
		zap.String("key", key))
	return QueueOutcomeDeduped
}

// ReportInsertResult classifies the error from an insert attempt. A unique
// violation on idx_run_idempotency counts kind="durable", logs at Warn with
// key, reason and agent, and returns (QueueOutcomeDeduped, nil). Any other
// non-nil error is returned UNCHANGED with QueueOutcomeNone and moves no
// counter — a disk error is not a dedup decision. A nil error returns
// (QueueOutcomeQueued, nil).
func ReportInsertResult(
	q QueueSource, reason, key, agentProfileID string, err error,
) (QueueOutcome, error) {
	if err == nil {
		return QueueOutcomeQueued, nil
	}
	if runssqlite.IsIdempotencyKeyUniqueViolation(err) {
		return ReportDurableDedup(q, reason, key, agentProfileID), nil
	}
	return QueueOutcomeNone, err
}

// ReportDurableDedup records a durable conflict the caller has already
// classified (the wakeup path's ErrWakeupIdempotencyConflict, or
// ReportInsertResult's own classification): counts kind="durable", logs at
// Warn, and returns QueueOutcomeDeduped.
func ReportDurableDedup(q QueueSource, reason, key, agentProfileID string) QueueOutcome {
	incRunDedup(q, reason, "durable")
	dedupLogger().Warn("run deduplicated (durable index)",
		zap.String("queue", string(q)),
		zap.String("reason", reason),
		zap.String("key", key),
		zap.String("agent_profile_id", agentProfileID))
	return QueueOutcomeDeduped
}

// ReportKeylessEnqueue records a producer's decision to enqueue a wake with
// no dedup key, attributed to reason and discriminated by cause so a
// resolution failure (unresolved) is countable separately from expected,
// by-design keyless traffic. Called by the producer at its own decision
// site, immediately before enqueuing — the queue itself never infers a
// cause from an empty key. detail names what failed to resolve (a fixed
// lowercase snake_case constant, never a formatted message); it is a log
// field only, never a counter label, and is empty for cause=by_design,
// which is counted but never logged.
func ReportKeylessEnqueue(reason string, cause KeylessCause, detail string) {
	incRunDedupKeyless(reason, cause)
	if cause != KeylessCauseUnresolved {
		return
	}
	dedupLogger().Info("run enqueued with no dedup key",
		zap.String("reason", reason),
		zap.String("cause", string(cause)),
		zap.String("detail", detail))
}

// Reason labels for office_assignment_rate_limit_total — the closed
// three-value set AC-OFFICE-ASSIGN-RATE-003.1 requires.
const (
	assignmentRateLimitReasonAllowanceExhausted = "allowance_exhausted"
	assignmentRateLimitReasonCountReadFailed    = "count_read_failed"
	assignmentRateLimitReasonTaskUnattributed   = "task_unattributed"
)

// ReportAssignmentRateLimitRefused records a wake refused because its
// task's REQ-OFFICE-ASSIGN-RATE-001 allowance was already exhausted:
// counts reason="allowance_exhausted", logs at Warn with the fields
// AC-OFFICE-ASSIGN-RATE-003.3 names, and returns QueueOutcomeRateLimited.
// The caller must not insert a row for this wake.
func ReportAssignmentRateLimitRefused(
	taskID, assigneeAgentProfileID, actingAgentID string, observedCount, allowance int,
) QueueOutcome {
	incAssignmentRateLimit(assignmentRateLimitReasonAllowanceExhausted)
	dedupLogger().Warn("assignment wake refused (rate limit)",
		zap.String("task_id", taskID),
		zap.String("assignee_agent_profile_id", assigneeAgentProfileID),
		zap.String("acting_agent_id", actingAgentID),
		zap.Int("observed_count", observedCount),
		zap.Int("allowance", allowance))
	return QueueOutcomeRateLimited
}

// ReportAssignmentRateLimitCountReadFailed records a degraded admission
// caused by a failed window-count read (AC-OFFICE-ASSIGN-RATE-002.2):
// counts reason="count_read_failed" and logs at Warn without an observed
// count, which the failed read did not produce. The wake is still
// admitted — this reports the degradation, it does not decide it.
func ReportAssignmentRateLimitCountReadFailed(
	taskID, assigneeAgentProfileID, actingAgentID string, allowance int, err error,
) {
	incAssignmentRateLimit(assignmentRateLimitReasonCountReadFailed)
	dedupLogger().Warn("assignment wake admitted (rate limit count read failed)",
		zap.String("task_id", taskID),
		zap.String("assignee_agent_profile_id", assigneeAgentProfileID),
		zap.String("acting_agent_id", actingAgentID),
		zap.Int("allowance", allowance),
		zap.Error(err))
}

// ReportAssignmentRateLimitUnattributed records a degraded admission
// caused by an otherwise in-scope wake whose task could not be determined
// (AC-OFFICE-ASSIGN-RATE-002.3): counts reason="task_unattributed" and
// logs at Warn without a task identifier, which is by definition the
// value that could not be determined. actingAgentID is logged only when
// non-empty. The wake is still admitted.
func ReportAssignmentRateLimitUnattributed(actingAgentID string, allowance int) {
	incAssignmentRateLimit(assignmentRateLimitReasonTaskUnattributed)
	fields := []zap.Field{
		zap.String("reason", assignmentRateLimitReasonTaskUnattributed),
		zap.Int("allowance", allowance),
	}
	if actingAgentID != "" {
		fields = append(fields, zap.String("acting_agent_id", actingAgentID))
	}
	dedupLogger().Warn("assignment wake admitted (task unattributed)", fields...)
}
