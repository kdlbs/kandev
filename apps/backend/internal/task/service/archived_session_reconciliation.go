package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// StartSessionReconciliationLoop starts the background goroutine that
// periodically reconciles task sessions that no request path owns anymore.
// Each tick runs two passes:
//
//  1. The archived-task pass: re-finalize archived tasks whose sessions
//     never made it to a terminal DB state (see runArchivedSessionReconciliation).
//  2. The active-task pass: detect and heal unarchived tasks holding active
//     sessions whose backing execution is gone (see runActiveSessionSweep in
//     active_session_stall.go).
//
// Pass 1 — archived tasks. finalizeCancelledSessions (see service_tasks.go)
// already bounds its session-cancellation retry to a handful of fixed attempts
// inside ArchiveTask's own request: enough to ride out a brief SQLite
// writer-lock blip, but not unbounded. If SQLite's single writer stays
// occupied for longer than every attempt combined, that bounded retry gets
// exhausted after archived_at has already committed, and nothing else in the
// request path ever retries the transition. The archived task is then left
// with sessions stuck in an active DB state (CREATED/STARTING/RUNNING/
// WAITING_FOR_INPUT) forever, and event-driven clients that key their
// "is running" indicators off session.state_changed never learn the
// sessions actually stopped.
//
// Pass 2 — orphaned sessions of unarchived tasks. A backend restart while an
// ACP prompt turn is open kills the actor that would have transitioned the
// session (the turn-complete path dies with the process), leaving
// task_sessions stuck in STARTING/RUNNING forever on tasks that were never
// archived. Every tick, stale STARTING/RUNNING sessions with no live
// in-memory execution backing them are terminalized.
//
// Both passes follow the exact periodic-sweep shape StartAutoArchiveLoop
// already uses and re-invoke the same finalize transitions; as long as the
// process keeps running, a later pass retries what an earlier pass missed.
func (s *Service) StartSessionReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.runArchivedSessionReconciliation(ctx)
				s.runOrphanedSessionReconciliation(ctx)
				s.runActiveSessionSweep(ctx, now)
			}
		}
	}()
	s.logger.Info("session reconciliation loop started (every 1 minute)")
}

// StartArchivedSessionReconciliationLoop keeps the pre-stall-sweep entry
// point available for integrations that started the archived pass directly.
// New startup wiring should use StartSessionReconciliationLoop so all passes
// run from one ticker.
func (s *Service) StartArchivedSessionReconciliationLoop(ctx context.Context) {
	s.StartSessionReconciliationLoop(ctx)
}

func (s *Service) runArchivedSessionReconciliation(ctx context.Context) {
	deadline := archivecascade.ArchiveDeadline(ctx)
	reconcileCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	taskIDs, err := s.tasks.ListArchivedTasksWithActiveSessions(reconcileCtx)
	if err != nil {
		s.logger.Error("archived-session reconciliation: failed to list candidates", zap.Error(err))
		return
	}
	if len(taskIDs) == 0 {
		return
	}

	s.logger.Info("archived-session reconciliation: found candidates", zap.Int("count", len(taskIDs)))
	for _, taskID := range taskIDs {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(reconcileCtx, taskID)
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
		s.finalizeCancelledSessions(reconcileCtx, taskID, activeSessions, deadline, models.SessionArchiveCancelReason)
	}
}

// orphanedSessionGraceAllowance is the slice added to the launch budget when
// deriving the orphan sweep's staleness cutoff, mirroring the session-ceiling
// reservation expiry: preparation is bounded by constants.AgentLaunchTimeout
// (read, not copied, so an operator-raised preparation budget is honored),
// and the allowance covers the start deadline inside the launch goroutine.
const orphanedSessionGraceAllowance = 5 * time.Minute

// orphanedSessionGracePeriod is how stale a STARTING/RUNNING session's last
// write must be before the sweep may consider it unbacked. A session in those
// states whose row is fresher than one full launch budget can still belong to
// an in-flight launch whose execution has not reached the in-memory store
// yet, so the sweep leaves it alone.
func orphanedSessionGracePeriod() time.Duration {
	return constants.AgentLaunchTimeout + orphanedSessionGraceAllowance
}

// orphanedSessionRepository is the narrow capability the orphan sweep needs
// off its sessions repository. Kept as an optional interface — type-asserted
// off s.sessions like taskWorkspaceMetadataCASSetter — rather than widening
// repository.SessionRepository, whose many test doubles would each have to
// grow methods this sweep never exercises through them. The concrete sqlite
// repository is the only production implementer.
type orphanedSessionRepository interface {
	ListStaleRunningSessionsOnUnarchivedTasks(ctx context.Context, staleBefore time.Time) ([]*models.TaskSession, error)
	CancelRunningTaskSessionByID(ctx context.Context, sessionID, reason string, staleBefore time.Time) (*models.TaskSession, error)
}

// runOrphanedSessionReconciliation is pass 2 of the reconciliation loop: it
// terminalizes stale STARTING/RUNNING sessions of unarchived tasks that no
// live in-memory execution backs. Without it, a backend restart mid-turn
// leaves those sessions RUNNING forever (#3711): the actor that would have
// transitioned them died with the process, and no other path ever retries.
// Sessions backed by a live execution — including survivors re-tracked by
// startup recovery — are never touched, and the per-session cancel keeps
// healthy sibling sessions of the same task (e.g. WAITING_FOR_INPUT) intact.
// A session whose cancellation write fails is simply retried on the next
// tick; the sweep never gives up for as long as the process runs.
func (s *Service) runOrphanedSessionReconciliation(ctx context.Context) {
	if s.executionLivenessChecker == nil {
		// Absence-from-store is the sweep's only dead signal; without the
		// checker it can never prove a session unbacked, so the pass is inert.
		return
	}
	repo, ok := s.sessions.(orphanedSessionRepository)
	if !ok {
		return
	}
	staleBefore := time.Now().UTC().Add(-orphanedSessionGracePeriod())
	candidates, err := repo.ListStaleRunningSessionsOnUnarchivedTasks(ctx, staleBefore)
	if err != nil {
		s.logger.Error("orphaned-session reconciliation: failed to list candidates", zap.Error(err))
		return
	}
	if len(candidates) == 0 {
		return
	}
	s.reconcileOrphanedSessions(ctx, candidates, staleBefore)
}

func (s *Service) reconcileOrphanedSessions(ctx context.Context, candidates []*models.TaskSession, staleBefore time.Time) {
	deadline := archivecascade.ArchiveDeadline(ctx)
	reconcileCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	s.reconcileOrphanedSessionsUntil(reconcileCtx, candidates, staleBefore, deadline)
}

// reconcileOrphanedSessionsUntil applies one orphan sweep within the supplied
// admission deadline. A cancellation write may finish after that deadline
// because the repository owns a detached write context; its post-commit
// effects therefore receive a fresh bounded context instead of the expired
// sweep context.
func (s *Service) reconcileOrphanedSessionsUntil(
	ctx context.Context,
	candidates []*models.TaskSession,
	staleBefore time.Time,
	deadline time.Time,
) {
	repo, ok := s.sessions.(orphanedSessionRepository)
	if !ok {
		return
	}
	for _, session := range candidates {
		if session == nil || session.ID == "" {
			continue
		}
		if !time.Now().Before(deadline) {
			s.logger.Debug("orphaned-session reconciliation: admission deadline reached",
				zap.Time("deadline", deadline))
			return
		}
		// Re-check liveness at the moment of the write, not just at the
		// candidate read: a launch that raced the grace window since the read
		// must not be reaped. The staleBefore predicate inside the cancel
		// UPDATE is the second half of that guard — it re-asserts the
		// staleness cutoff so a row refreshed by an in-flight launch between
		// this check and the write no longer matches.
		if s.executionLivenessChecker.HasLiveExecution(session.ID) {
			continue
		}
		cancelled, err := repo.CancelRunningTaskSessionByID(ctx, session.ID, models.SessionOrphanedCancelReason, staleBefore)
		if err != nil {
			s.logger.Warn("orphaned-session reconciliation: failed to cancel session",
				zap.String("session_id", session.ID),
				zap.Error(err))
			continue
		}
		if cancelled == nil {
			// Raced to another state between the candidate read and the
			// write; its new owner is responsible for it now.
			continue
		}
		s.logger.Info("orphaned-session reconciliation: terminalized unbacked session",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.String("previous_state", string(session.State)))
		completionDeadline := time.Now().Add(taskPublicationTimeout)
		completionCtx, cancelCompletion := context.WithDeadline(
			context.WithoutCancel(ctx), completionDeadline,
		)
		s.runCancelledSessionEffects(completionCtx, session.TaskID,
			[]*models.TaskSession{session}, []*models.TaskSession{cancelled},
			completionDeadline, models.SessionOrphanedCancelReason)
		cancelCompletion()
	}
}
