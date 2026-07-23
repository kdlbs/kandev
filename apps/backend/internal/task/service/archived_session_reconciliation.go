package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// StartArchivedSessionReconciliationLoop starts a background goroutine that
// periodically re-finalizes archived tasks whose sessions never made it to a
// terminal DB state.
//
// finalizeCancelledSessions (see service_tasks.go) already bounds its
// session-cancellation retry to a handful of fixed attempts inside
// ArchiveTask's own request: enough to ride out a brief SQLite writer-lock
// blip, but not unbounded. If SQLite's single writer stays occupied for
// longer than every attempt combined, that bounded retry gets exhausted
// after archived_at has already committed, and nothing else in the request
// path ever retries the transition. The archived task is then left with
// sessions stuck in an active DB state (CREATED/STARTING/RUNNING/
// WAITING_FOR_INPUT) forever, and event-driven clients that key their
// "is running" indicators off session.state_changed never learn the
// sessions actually stopped.
//
// This sweep is the eventual-consistency backstop for that residual gap,
// following the exact periodic-sweep shape StartAutoArchiveLoop already
// uses: on a fixed interval, list every archived task that still has an
// active session, and re-invoke finalizeCancelledSessions for each. Calling
// finalizeCancelledSessions again is always safe — its underlying
// CancelActiveTaskSessionsByTaskID UPDATE only matches sessions still in an
// active state, so a task that was already fully reconciled (by ArchiveTask
// itself or by an earlier sweep pass) simply yields an empty cancelledSessions
// slice and is a no-op. As long as the process keeps running, a later pass
// retries once the SQLite contention that exhausted the in-line retry has
// cleared — unlike the fixed 3-attempt budget, this sweep never gives up.
func (s *Service) StartArchivedSessionReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runArchivedSessionReconciliation(ctx)
			}
		}
	}()
	s.logger.Info("archived-session reconciliation loop started (every 1 minute)")
}

func (s *Service) runArchivedSessionReconciliation(ctx context.Context) {
	taskIDs, err := s.tasks.ListArchivedTasksWithActiveSessions(ctx)
	if err != nil {
		s.logger.Error("archived-session reconciliation: failed to list candidates", zap.Error(err))
		return
	}
	if len(taskIDs) == 0 {
		return
	}

	s.logger.Info("archived-session reconciliation: found candidates", zap.Int("count", len(taskIDs)))
	for _, taskID := range taskIDs {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, taskID)
		if err != nil {
			s.logger.Warn("archived-session reconciliation: failed to list active sessions",
				zap.String("task_id", taskID),
				zap.Error(err))
			continue
		}
		if len(activeSessions) == 0 {
			// Already reconciled by a concurrent pass or by ArchiveTask itself
			// between the candidate list query and this read.
			continue
		}
		s.finalizeCancelledSessions(ctx, taskID, activeSessions)
	}
}
