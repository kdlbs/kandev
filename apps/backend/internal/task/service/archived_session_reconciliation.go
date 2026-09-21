package service

import (
	"context"
	"errors"
	"fmt"
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
// in-memory execution backing them return to WAITING_FOR_INPUT for lazy
// recovery.
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
	RecoverTaskSessionByCandidate(ctx context.Context, candidate models.ActiveSessionRecoveryCandidate, staleBefore time.Time) (*models.TaskSession, error)
}

// runOrphanedSessionReconciliation is pass 2 of the reconciliation loop: it
// returns stale STARTING/RUNNING sessions of unarchived tasks to
// WAITING_FOR_INPUT when no live in-memory execution backs them. An
// interrupted turn is settled without a completion event, so the same
// conversation remains available for lazy recovery. Sessions backed by a
// live execution — including survivors re-tracked by startup recovery — are
// never touched. A failed recovery write is retried on the next tick.
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
// admission deadline. A recovery write may finish after that deadline because
// the repository owns a detached write context; the state event and turn
// settlement therefore use the bounded context supplied to the write.
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
	lastMessageBySession, err := s.messages.GetLastMessageTimeBySessionIDs(ctx, sessionIDs(candidates))
	if err != nil {
		s.logger.Warn("orphaned-session reconciliation: failed to load last message times; skipping pass",
			zap.Error(err))
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
		// must not be recovered. The staleBefore predicate inside the recovery
		// UPDATE is the second half of that guard — it re-asserts the staleness
		// cutoff so a row refreshed by an in-flight launch between this check
		// and the write no longer matches.
		if s.executionLivenessChecker.HasLiveExecution(session.ID) {
			continue
		}
		turn, err := s.GetActiveTurn(ctx, session.ID)
		if err != nil {
			s.logger.Warn("orphaned-session reconciliation: failed to inspect current turn; skipping session",
				zap.String("session_id", session.ID),
				zap.Error(err))
			continue
		}
		candidate := models.ActiveSessionRecoveryCandidate{
			TaskID:              session.TaskID,
			SessionID:           session.ID,
			ExpectedState:       session.State,
			ExpectedUpdatedAt:   session.UpdatedAt,
			ExpectedLastEventAt: lastSessionEventAt(session, lastMessageBySession),
		}
		if turn != nil {
			candidate.ExpectedTurnID = turn.ID
		}
		if err := s.captureRecoveryExecutorSnapshot(ctx, &candidate); err != nil {
			s.logger.Warn("orphaned-session reconciliation: failed to capture executor reservation; skipping session",
				zap.String("session_id", session.ID),
				zap.Error(err))
			continue
		}
		// The turn read above can take long enough for a replacement
		// execution to register. Re-check immediately before the guarded write
		// so a live successor keeps ownership of the session.
		if s.executionLivenessChecker.HasLiveExecution(session.ID) {
			continue
		}
		recovered, err := repo.RecoverTaskSessionByCandidate(ctx, candidate, staleBefore)
		if err != nil {
			s.logger.Warn("orphaned-session reconciliation: failed to recover session",
				zap.String("session_id", session.ID),
				zap.Error(err))
			continue
		}
		if recovered == nil {
			// Raced to another state between the candidate read and the
			// write; its new owner is responsible for it now. The next
			// sweep will classify any still-interrupted state again.
			continue
		}
		completionCtx, cancelCompletion := context.WithTimeout(
			context.WithoutCancel(ctx), taskPublicationTimeout,
		)
		s.settleRecoveredSession(completionCtx, candidate, recovered)
		cancelCompletion()
		s.logger.Info("orphaned-session reconciliation: preserved unbacked session for lazy recovery",
			zap.String("task_id", session.TaskID),
			zap.String("session_id", session.ID),
			zap.String("previous_state", string(session.State)))
	}
}

// captureRecoveryExecutorSnapshot records the executor identity observed with
// the session candidate. Post-commit repair must use this immutable snapshot;
// reading the row again after the session write can otherwise stop a successor
// that registered during the effects window.
func (s *Service) captureRecoveryExecutorSnapshot(ctx context.Context, candidate *models.ActiveSessionRecoveryCandidate) error {
	if s == nil || s.executors == nil || candidate == nil || candidate.SessionID == "" {
		return nil
	}
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, candidate.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to capture executor reservation for session recovery: %w", err)
	}
	if running == nil {
		return nil
	}
	candidate.ExpectedExecutorID = running.ID
	candidate.ExpectedExecutorAgentExecutionID = running.AgentExecutionID
	candidate.ExpectedExecutorUpdatedAt = running.UpdatedAt
	return nil
}

// settleRecoveredSession applies the side effects shared by both execution
// loss sweeps. An interrupted turn is closed with zero duration and pending
// tool calls are settled, but no turn.completed event or workflow transition
// is emitted. The recovery state event is published only after the conditional
// session write has committed.
//
//nolint:cyclop // The ordered effects boundary is intentionally explicit.
func (s *Service) settleRecoveredSession(
	ctx context.Context,
	candidate models.ActiveSessionRecoveryCandidate,
	recovered *models.TaskSession,
) {
	if recovered == nil {
		return
	}
	// A retry may observe the stale executor row already repaired by an earlier
	// attempt. That stopped row is still the same recovery generation and may
	// continue through settlement; a newly running or rotated execution never
	// satisfies the repaired-row allowance.
	if !s.recoveredSessionStillOwned(ctx, candidate, recovered, true) {
		return
	}
	settlementComplete := true
	if err := s.repairRecoveredExecutor(ctx, candidate); err != nil {
		settlementComplete = false
	}
	if candidate.ExpectedTurnID != "" {
		turnSettled, successor := s.settleRecoveredTurn(ctx, candidate)
		if successor {
			return
		}
		settlementComplete = settlementComplete && turnSettled
	}
	if !s.recoveredSessionStillOwned(ctx, candidate, recovered, true) {
		return
	}
	if candidate.ExpectedTurnID != "" ||
		candidate.ExpectedState == models.TaskSessionStateCreated ||
		candidate.ExpectedState == models.TaskSessionStateStarting ||
		candidate.ExpectedState == models.TaskSessionStateRunning {
		if _, err := s.markTaskInterrupted(ctx, recovered); err != nil {
			settlementComplete = false
		}
	}
	if !s.recoveredSessionStillOwned(ctx, candidate, recovered, true) {
		return
	}
	// Publish before clearing the durable settlement. If the clear fails, the
	// next sweep republishes this idempotent state event so the recovery cannot
	// be lost between the database write and publication.
	if err := s.publishSessionRecovered(ctx, candidate.TaskID, candidate.ExpectedState, recovered); err != nil {
		settlementComplete = false
	}
	if settlementComplete {
		if err := s.clearRecoverySettlement(ctx, recovered); err != nil {
			s.logger.Warn("failed to clear recovered-session settlement marker",
				zap.String("session_id", candidate.SessionID), zap.Error(err))
		}
	}
}

func (s *Service) settleRecoveredTurn(
	ctx context.Context,
	candidate models.ActiveSessionRecoveryCandidate,
) (settled, successor bool) {
	activeTurn, err := s.GetActiveTurn(ctx, candidate.SessionID)
	if err != nil {
		s.logger.Warn("failed to inspect interrupted turn after recovery",
			zap.String("session_id", candidate.SessionID),
			zap.String("turn_id", candidate.ExpectedTurnID),
			zap.Error(err))
		return false, false
	}
	if activeTurn == nil {
		return s.completeRecoveredTurnToolCalls(ctx, candidate), false
	}
	if activeTurn.ID != candidate.ExpectedTurnID {
		// A different active turn proves a successor crossed the effects
		// boundary. Leave its turn, marker, and event state alone.
		return false, true
	}
	if err := s.turns.AbandonTurn(ctx, candidate.ExpectedTurnID); err != nil {
		s.logger.Warn("failed to abandon interrupted turn after recovery",
			zap.String("session_id", candidate.SessionID),
			zap.String("turn_id", candidate.ExpectedTurnID),
			zap.Error(err))
		return false, false
	}
	return s.completeRecoveredTurnToolCalls(ctx, candidate), false
}

func (s *Service) completeRecoveredTurnToolCalls(
	ctx context.Context,
	candidate models.ActiveSessionRecoveryCandidate,
) bool {
	affected, err := s.turns.CompletePendingToolCallsForTurn(ctx, candidate.ExpectedTurnID)
	if err != nil {
		s.logger.Warn("failed to complete pending tool calls for recovered turn",
			zap.String("session_id", candidate.SessionID),
			zap.String("turn_id", candidate.ExpectedTurnID),
			zap.Error(err))
		return false
	}
	if affected > 0 {
		s.logger.Info("completed pending tool calls for recovered turn",
			zap.String("session_id", candidate.SessionID),
			zap.String("turn_id", candidate.ExpectedTurnID),
			zap.Int64("affected", affected))
	}
	return true
}

// repairRecoveredExecutor releases the stale runtime reservation while
// preserving resume_token, worktree, and other recovery identity. The
// compare-and-set variant is preferred when the executor repository exposes
// it, so a successor launch cannot be marked stopped by a delayed sweep.
func (s *Service) repairRecoveredExecutor(ctx context.Context, candidate models.ActiveSessionRecoveryCandidate) error {
	if s.executors == nil || candidate.SessionID == "" ||
		(candidate.ExpectedExecutorID == "" && candidate.ExpectedExecutorAgentExecutionID == "" && candidate.ExpectedExecutorUpdatedAt.IsZero()) {
		return nil
	}
	if cas, ok := s.executors.(interface {
		RepairExecutorRunningDeadIfCurrent(context.Context, string, string, time.Time) error
	}); ok {
		err := cas.RepairExecutorRunningDeadIfCurrent(
			ctx, candidate.SessionID, candidate.ExpectedExecutorAgentExecutionID, candidate.ExpectedExecutorUpdatedAt,
		)
		if errors.Is(err, models.ErrExecutorRunningNotFound) || errors.Is(err, models.ErrExecutionRotated) {
			return nil
		}
		return err
	} else {
		// Legacy adapters have no compare-and-set primitive. Refuse to claim
		// settlement completed: a post-commit read/repair pair could stop a
		// successor, so the marker and event must remain retryable instead.
		return errors.New("executor repository does not support guarded recovery repair")
	}
}

// recoveredSessionStillOwned verifies that the recovery write still owns its
// effects boundary. It compares the session generation and the executor
// identity captured before the write; a newly registered successor therefore
// prevents stale marker, turn, and event effects from being applied.
func (s *Service) recoveredSessionStillOwned(
	ctx context.Context,
	candidate models.ActiveSessionRecoveryCandidate,
	recovered *models.TaskSession,
	allowRepairedExecutor bool,
) bool {
	if recovered == nil || s.sessions == nil {
		return false
	}
	expectedUpdatedAt := recovered.UpdatedAt
	if !candidate.RecoveredUpdatedAt.IsZero() {
		expectedUpdatedAt = candidate.RecoveredUpdatedAt
	}
	current, err := s.sessions.GetTaskSession(ctx, candidate.SessionID)
	if err != nil || current == nil || current.TaskID != candidate.TaskID ||
		current.State != models.TaskSessionStateWaitingForInput ||
		!current.UpdatedAt.Equal(expectedUpdatedAt) {
		return false
	}
	return s.recoveryExecutorStillOwned(ctx, candidate, allowRepairedExecutor)
}

//nolint:cyclop // Identity, timestamp, and repaired-row guards are one CAS contract.
func (s *Service) recoveryExecutorStillOwned(
	ctx context.Context,
	candidate models.ActiveSessionRecoveryCandidate,
	allowRepairedExecutor bool,
) bool {
	if s.executors == nil {
		return true
	}
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, candidate.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return candidate.ExpectedExecutorID == "" && candidate.ExpectedExecutorAgentExecutionID == "" && candidate.ExpectedExecutorUpdatedAt.IsZero()
	}
	if err != nil || running == nil {
		return false
	}
	if candidate.ExpectedExecutorID == "" && candidate.ExpectedExecutorAgentExecutionID == "" && candidate.ExpectedExecutorUpdatedAt.IsZero() {
		return false
	}
	if candidate.ExpectedExecutorID != "" && running.ID != candidate.ExpectedExecutorID {
		return false
	}
	if candidate.ExpectedExecutorAgentExecutionID != "" && running.AgentExecutionID != candidate.ExpectedExecutorAgentExecutionID {
		return false
	}
	if !candidate.ExpectedExecutorUpdatedAt.IsZero() && !running.UpdatedAt.Equal(candidate.ExpectedExecutorUpdatedAt) {
		return allowRepairedExecutor && running.Status == models.ExecutorRunningStatusStopped &&
			running.AgentExecutionID == candidate.ExpectedExecutorAgentExecutionID
	}
	return true
}

func (s *Service) clearRecoverySettlement(ctx context.Context, session *models.TaskSession) error {
	settlement, ok := models.LoadInterruptedRecoverySettlement(session.Metadata)
	if !ok {
		return nil
	}
	remover, ok := s.sessions.(interface {
		RemoveSessionMetadataKeyIfJSONValue(context.Context, string, string, interface{}) (bool, error)
	})
	if !ok {
		return errors.New("session repository does not support guarded recovery settlement removal")
	}
	_, err := remover.RemoveSessionMetadataKeyIfJSONValue(
		ctx, session.ID, models.SessionMetaKeyRecoverySettlementPending, settlement,
	)
	return err
}

type interruptedTaskMetadataSetter interface {
	SetTaskMetadataKeyIfNotArchived(ctx context.Context, taskID, key string, value interface{}) (bool, error)
}

type interruptedTaskMetadataAbsentSetter interface {
	SetTaskMetadataKeyIfAbsentNotArchived(ctx context.Context, taskID, key string, value interface{}) (bool, error)
}

// interruptedTaskMetadataRecoverySetter adds the session recovery generation
// to the marker write. The task row is updated only while the recovered
// session still owns its WAITING_FOR_INPUT generation and its settlement token
// is present. This closes the gap between the in-memory ownership check and
// the task metadata write when a successor starts concurrently.
type interruptedTaskMetadataRecoverySetter interface {
	SetTaskMetadataKeyIfRecoveryCurrent(
		ctx context.Context,
		taskID, sessionID string,
		expectedSessionUpdatedAt time.Time,
		expectedRecoveryToken, key string,
		value interface{},
	) (bool, error)
}

func (s *Service) markTaskInterrupted(ctx context.Context, recovered *models.TaskSession) (bool, error) {
	if recovered == nil || recovered.TaskID == "" || recovered.ID == "" {
		return false, nil
	}
	settlement, hasSettlement := models.LoadInterruptedRecoverySettlement(recovered.Metadata)
	// The concrete SQLite task repository owns the archive-atomic metadata CAS.
	// Keep this optional so focused task-service users and test repositories
	// without the newer marker primitive remain compatible.
	_, ok := s.tasks.(interruptedTaskMetadataSetter)
	if !ok {
		return false, errors.New("task repository does not support recovery interruption markers")
	}
	value := time.Now().UTC().Format(time.RFC3339Nano)
	expectedUpdatedAt := recovered.UpdatedAt
	if hasSettlement && !settlement.RecoveredUpdatedAt.IsZero() {
		expectedUpdatedAt = settlement.RecoveredUpdatedAt
	}
	var changed bool
	var err error
	if recoverySetter, supported := s.tasks.(interruptedTaskMetadataRecoverySetter); supported && hasSettlement {
		changed, err = recoverySetter.SetTaskMetadataKeyIfRecoveryCurrent(
			ctx,
			recovered.TaskID,
			recovered.ID,
			expectedUpdatedAt,
			settlement.Token,
			models.MetaKeyInterruptedAt,
			value,
		)
	} else if absentSetter, supported := s.tasks.(interruptedTaskMetadataAbsentSetter); supported {
		changed, err = absentSetter.SetTaskMetadataKeyIfAbsentNotArchived(ctx, recovered.TaskID, models.MetaKeyInterruptedAt, value)
	} else {
		// A plain archive guard does not protect a recovery marker from a
		// newer interruption arriving between the ownership check and this write.
		// Fail closed when the adapter lacks either generation-safe primitive.
		s.logger.Warn("skipping recovery interruption marker without generation-safe setter",
			zap.String("task_id", recovered.TaskID),
			zap.String("session_id", recovered.ID))
		return false, errors.New("task repository does not support generation-safe recovery interruption markers")
	}
	if err != nil {
		return false, err
	}
	task, err := s.tasks.GetTask(ctx, recovered.TaskID)
	if err != nil || task == nil {
		if err == nil {
			err = errors.New("task not found after interruption marker write")
		}
		return changed, err
	}
	// Publish after the metadata commit. The task service's per-task FIFO keeps
	// this update ahead of later clears, so connected clients see the warning
	// without a reload.
	s.PublishTaskUpdated(ctx, task)
	return changed, nil
}
