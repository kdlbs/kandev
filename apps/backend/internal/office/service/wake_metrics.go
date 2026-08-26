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
// (handler not ticking at all).
var (
	parentWakeCandidatesTotal         = expvar.NewInt("parent_wake_candidates_total")
	parentWakeReceiptStaleTotal       = expvar.NewInt("parent_wake_receipt_stale_total")
	parentWakeEmittedTotal            = expvar.NewInt("parent_wake_emitted_total")
	parentWakeUnchangedSkipTotal      = expvar.NewInt("parent_wake_unchanged_skip_total")
	parentWakeAssigneeUnresolvedTotal = expvar.NewInt("parent_wake_assignee_unresolved_total")
)

const (
	metricWakeCandidate          = "wake.metric.candidate"
	metricWakeReceiptStale       = "wake.metric.receipt_stale"
	metricWakeEmitted            = "wake.metric.emitted"
	metricWakeUnchangedSkip      = "wake.metric.unchanged_skip"
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

// recordWakeReceiptStale bumps the count of candidates whose stored
// receipt no longer matches the current child set (or had none), i.e.
// candidates the reconciler is about to re-emit a wake for.
func (s *Service) recordWakeReceiptStale(parentTaskID string) {
	parentWakeReceiptStaleTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricWakeReceiptStale, zap.String("parent_task_id", parentTaskID))
}

// recordWakeEmitted bumps the count of task_children_completed runs the
// reconciler inserted directly via CreateRunTx.
func (s *Service) recordWakeEmitted(parentTaskID, runID string) {
	parentWakeEmittedTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Info(metricWakeEmitted,
		zap.String("parent_task_id", parentTaskID), zap.String("run_id", runID))
}

// recordWakeUnchangedSkip bumps the count of candidates whose receipt
// already matches the current child set — the steady-state, no-op case.
func (s *Service) recordWakeUnchangedSkip(parentTaskID string) {
	parentWakeUnchangedSkipTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricWakeUnchangedSkip, zap.String("parent_task_id", parentTaskID))
}

// recordWakeAssigneeUnresolved bumps the count of candidates skipped
// because no runner could be resolved, or the resolved agent is paused,
// stopped, or pending approval.
func (s *Service) recordWakeAssigneeUnresolved(parentTaskID, reason string) {
	parentWakeAssigneeUnresolvedTotal.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Debug(metricWakeAssigneeUnresolved,
		zap.String("parent_task_id", parentTaskID), zap.String("reason", reason))
}
