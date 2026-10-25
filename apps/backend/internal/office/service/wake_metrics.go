package service

import (
	"expvar"

	"go.uber.org/zap"
)

// expvar counters published at package init, exposed via stdlib's
// /debug/vars handler (dev mode). Mirrors cost_metrics.go's shape, but as
// plain counters rather than label maps — a parent task ID is the only
// candidate dimension here and it's unbounded, so there is nothing worth
// labeling on.
//
// These are ParentWakeReconciler's only steady-state signal
// (scheduler_wake_reconciler.go): production cannot otherwise tell a
// working sweep (candidates found, nothing to do) from a dead one
// (handler not ticking at all). The steady-state no-op case — a parent
// whose receipt already matches its current child set, whose wake already
// has an active/finished run, or whose assignee is unresolved/paused/
// stopped/pending-approval — is filtered out in ListStuckParents' SQL
// before a tick ever sees it, so parent_wake_candidates_total itself
// staying flat is that steady-state signal; there is no separate
// "unchanged, skipped" counter, because nothing ever reaches Go to skip.
//
// There were briefly four counters here. parent_wake_receipt_stale_total
// was removed: reconcileOne bumped it unconditionally as its first
// statement, with no branch between it and recordWakeCandidate, so the two
// could never diverge for a processed candidate — pure duplication, not a
// second signal. parent_wake_assignee_unresolved_total stays, but its
// trigger narrowed: SQL now excludes the common unresolved/paused/
// stopped/pending-approval case before Go ever sees the row (see
// ListStuckParents), so this only fires for the residual TOCTOU race
// window between that SELECT and guardAgentStatus's own read a moment
// later — a real, if rare, case (unlike the receipt, an agent's status
// isn't reconciler-exclusive; anything can change it mid-tick). A nonzero
// rate here is itself a signal worth alerting on, since it should be
// close to zero in steady state.
var (
	parentWakeCandidatesTotal         = expvar.NewInt("parent_wake_candidates_total")
	parentWakeEmittedTotal            = expvar.NewInt("parent_wake_emitted_total")
	parentWakeAssigneeUnresolvedTotal = expvar.NewInt("parent_wake_assignee_unresolved_total")
)

const (
	metricWakeCandidate          = "wake.metric.candidate"
	metricWakeEmitted            = "wake.metric.emitted"
	metricWakeAssigneeUnresolved = "wake.metric.assignee_unresolved"
)

// recordWakeCandidate bumps the count of stuck-parent rows the sweep
// observed this tick, before any receipt comparison.
func (s *Service) recordWakeCandidate(parentTaskID string) {
	parentWakeCandidatesTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricWakeCandidate, zap.String("parent_task_id", parentTaskID))
}

// recordWakeEmitted bumps the count of task_children_completed operations
// the reconciler dispatched through the workflow engine.
func (s *Service) recordWakeEmitted(parentTaskID, operationID string) {
	parentWakeEmittedTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Info(metricWakeEmitted,
		zap.String("parent_task_id", parentTaskID), zap.String("operation_id", operationID))
}

// recordWakeAssigneeUnresolved bumps the count of candidates skipped
// because the assignee failed guardAgentStatus's race-window recheck
// after already passing ListStuckParents' SQL-side status filter (see the
// package doc comment above).
func (s *Service) recordWakeAssigneeUnresolved(parentTaskID, reason string) {
	parentWakeAssigneeUnresolvedTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricWakeAssigneeUnresolved,
		zap.String("parent_task_id", parentTaskID), zap.String("reason", reason))
}
