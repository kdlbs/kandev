package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// Stale pending-move expiry.
//
// A move_task_kandev call made while the calling session is mid-turn cannot
// apply immediately — it would race on_enter against the running turn — so it
// is persisted to pending_moves and replayed by handleAgentReady once the turn
// ends. In a healthy system the arm and the replay are seconds to minutes
// apart.
//
// Nothing bounded that window. pending_moves.queued_at was written and never
// read as a freshness check, and no scan ever visited the table: session_id is
// UNIQUE, so a row could only be removed by a take/replace on that exact
// session. A row whose turn never ended cleanly (crash, restart, parked
// session) therefore stayed armed indefinitely and fired whenever the keyed
// session next became messageable. Because messaging a WAITING_FOR_INPUT task
// resumes its session and reaches handleAgentReady, an ordinary nudge could
// silently relocate a card days later, against a board that had moved on.
//
// Two failure modes, two guards:
//
//   - Aged out. Handled at replay time by discardStalePendingMove, and swept
//     as cleanup. Replay time is authoritative: it sits on the only path that
//     can actually move a card, so a stale row cannot apply even if the sweep
//     never ran or the process just booted. Correctness does not depend on a
//     background loop.
//
//   - Keyed session no longer exists. Unreachable from replay by construction
//     — a session that is gone will never emit another agent.ready — so this
//     one is sweep-only.
//
// The sweep deletes rather than tombstones. An expired move never became a
// step transition, so the ADR 0015 transition audit has nothing legitimate to
// record; writing one would fabricate a transition that never happened, and a
// dedicated tombstone table would be durable schema with no consumer. The drop
// is logged at Warn with the whole row instead, so it stays visible in logs and
// diagnostic bundles.
//
// Both guards also drop the move's hand-off prompt. handleMoveTask queues that
// prompt ("You were moved to this step with the following message: ...") before
// the move is applied, and it is authored for the move's *target* step. If the
// move never lands, the on_enter path that would have drained it never runs, so
// leaving it queued misdelivers it to the source step's agent on some later
// turn. applyPendingMove already does this on each of its own drop paths.

// reapStalePendingMovesOnce is the per-tick sweep. It reads every armed move,
// drops the ones no replay should ever apply, and leaves everything else alone.
//
// Each row is best-effort: a failure is logged at Warn and skipped, and the
// next tick retries. A single-row error never aborts the scan.
func (s *Service) reapStalePendingMovesOnce(ctx context.Context) {
	if s.messageQueue == nil {
		return
	}
	records, err := s.messageQueue.ListPendingMoves(ctx)
	if err != nil {
		s.logger.Warn("pending-move sweep: list failed; tick skipped", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, record := range records {
		if record.SessionID == "" {
			continue
		}
		reason, drop := s.pendingMoveReapReason(ctx, now, record)
		if !drop {
			continue
		}
		move := record.Move
		moveID := normalizedPendingMoveID(record.SessionID, &move)
		handoffEntryID, listed := s.pendingMoveHandoffPromptID(ctx, record.SessionID, move.TaskID, moveID)
		if !listed {
			s.logger.Warn("pending-move sweep: prompt cleanup failed; row preserved for next tick",
				zap.String("session_id", record.SessionID))
			continue
		}
		removed, err := s.messageQueue.DeletePendingMoveIfMatch(ctx, record, handoffEntryID)
		if err != nil {
			s.logger.Warn("pending-move sweep: delete failed; row preserved for next tick",
				zap.String("session_id", record.SessionID),
				zap.Error(err))
			continue
		}
		if !removed {
			// Raced a take, transfer, or replacement between the list and the
			// compare-and-delete. Whoever won owns the current row now.
			continue
		}
		s.logger.Warn("reaped stale pending move",
			append(pendingMoveLogFields(record.SessionID, &move, moveID), zap.String("reason", reason))...)
		s.pendingMoveHandoffPromptRemoved(ctx, record.SessionID, move.TaskID, moveID, handoffEntryID)
	}
}

// pendingMoveReapReason decides whether a row should be dropped, and why.
//
// Fail-closed on the session check: a row is reaped for a missing session only
// when the lookup says exactly that. Any other error (transient DB failure,
// timeout, cancelled context) preserves the row for the next tick. An uncertain
// guard is a skip, never a delete — the same invariant reclaimIdleSession
// carries.
func (s *Service) pendingMoveReapReason(
	ctx context.Context,
	now time.Time,
	record messagequeue.PendingMoveRecord,
) (string, bool) {
	if record.Move.IsStaleAt(now, messagequeue.PendingMoveTTL) {
		return "ttl_expired", true
	}
	if _, err := s.repo.GetTaskSession(ctx, record.SessionID); err != nil {
		if isTaskSessionNotFound(err) {
			return "session_missing", true
		}
		s.logger.Warn("pending-move sweep: session lookup failed; row preserved for next tick",
			zap.String("session_id", record.SessionID),
			zap.Error(err))
		return "", false
	}
	return "", false
}

// discardStalePendingMove is the replay-time guard, called by handleAgentReady
// before the move is claimed for application. stale reports that the move must
// not apply. retryPending additionally reports that the move or a replacement
// remains armed, so the caller must return without draining its target-step
// prompt through the source step. A fully discarded stale move falls through
// to normal on_turn_complete handling, exactly as if no move had been armed.
//
// The prompt lookup precedes an atomic exact-row delete. A lookup failure
// leaves the durable move for retry; a replacement deletes neither row; and a
// prompt-delete failure rolls back the move delete with the transaction.
func (s *Service) discardStalePendingMove(
	ctx context.Context,
	taskID, sessionID string,
	move *messagequeue.PendingMove,
) (stale, retryPending bool) {
	if move == nil {
		return false, false
	}
	if !move.IsStaleAt(time.Now().UTC(), messagequeue.PendingMoveTTL) {
		return false, false
	}
	moveID := normalizedPendingMoveID(sessionID, move)
	handoffEntryID, listed := s.pendingMoveHandoffPromptID(ctx, sessionID, taskID, moveID)
	if !listed {
		s.logger.Warn("expired pending move preserved after prompt cleanup failure",
			append(pendingMoveLogFields(sessionID, move, moveID), zap.Duration("ttl", messagequeue.PendingMoveTTL))...)
		return true, true
	}
	expected := messagequeue.PendingMoveRecord{SessionID: sessionID, Move: *move}
	removed, err := s.messageQueue.DeletePendingMoveIfMatch(ctx, expected, handoffEntryID)
	if err != nil {
		s.logger.Warn("expired pending move preserved after delete failure",
			append(pendingMoveLogFields(sessionID, move, moveID), zap.Error(err))...)
		return true, true
	}
	if removed {
		s.logger.Warn("dropping expired pending move instead of applying it",
			append(pendingMoveLogFields(sessionID, move, moveID), zap.Duration("ttl", messagequeue.PendingMoveTTL))...)
		s.pendingMoveHandoffPromptRemoved(ctx, sessionID, taskID, moveID, handoffEntryID)
		return true, false
	}
	return true, true
}

// handlePendingMoveAtAgentReady returns true when normal turn-complete queue
// handling must stop. A fully discarded stale move returns false so the caller
// continues exactly as if no move had been armed. Storage failures leave the
// move armed and settle the completed session so a later ready event can retry.
func (s *Service) handlePendingMoveAtAgentReady(
	ctx context.Context,
	taskID, sessionID string,
	session *models.TaskSession,
) bool {
	const maxReplayAttempts = 2
	settleSession := func() {
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
	}

	for attempt := 0; attempt < maxReplayAttempts; attempt++ {
		move, exists, err := s.messageQueue.GetPendingMoveWithError(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to read pending move; row preserved for retry",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
			settleSession()
			return true
		}
		if !exists {
			if attempt > 0 {
				// A concurrent handler won the claim after our compare failed.
				// Do not resume normal turn-complete processing for this event.
				settleSession()
				return true
			}
			return false
		}

		stale, retryPending := s.discardStalePendingMove(ctx, taskID, sessionID, move)
		if retryPending {
			settleSession()
			return true
		}
		if stale {
			return false
		}

		record := messagequeue.PendingMoveRecord{SessionID: sessionID, Move: *move}
		removed, err := s.messageQueue.DeletePendingMoveIfMatch(ctx, record, "")
		if err != nil {
			s.logger.Warn("failed to claim pending move; row preserved for retry",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
			settleSession()
			return true
		}
		if !removed {
			// A replacement raced the read. Re-read once so the successor can be
			// applied by this completed turn instead of waiting for an unrelated
			// ready event. The bounded loop preserves the row if races continue.
			continue
		}
		// The turn is already complete. Settle the session before applying the
		// move so early validation or storage failures inside applyPendingMove
		// cannot leave it RUNNING with no active turn.
		settleSession()
		s.applyPendingMove(ctx, taskID, sessionID, session, move)
		return true
	}

	s.logger.Warn("pending move changed repeatedly during replay; row preserved for retry",
		zap.String("task_id", taskID), zap.String("session_id", sessionID))
	settleSession()
	return true
}

// normalizedPendingMoveID resolves the correlation ID for a move, deriving the
// legacy identity for rows written before move_id existed. Mirrors what
// applyPendingMove does before it uses the ID.
func normalizedPendingMoveID(sessionID string, move *messagequeue.PendingMove) string {
	if move.MoveID != "" {
		return move.MoveID
	}
	return legacyPendingMoveID(sessionID, move)
}

// pendingMoveLogFields renders the whole dropped row. The log line is the only
// record an expired move leaves, so it carries every field needed to work out
// after the fact what was dropped and where it came from.
func pendingMoveLogFields(sessionID string, move *messagequeue.PendingMove, moveID string) []zap.Field {
	return []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("task_id", move.TaskID),
		zap.String("workflow_id", move.WorkflowID),
		zap.String("target_step_id", move.WorkflowStepID),
		zap.String("move_id", moveID),
		zap.String("actor", move.Actor),
		zap.String("sender_session_id", move.SenderSessionID),
		zap.Time("queued_at", move.QueuedAt),
		zap.Duration("age", time.Since(move.QueuedAt)),
	}
}
