package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
		removed, err := s.messageQueue.DeletePendingMove(ctx, record.SessionID)
		if err != nil {
			s.logger.Warn("pending-move sweep: delete failed; row preserved for next tick",
				zap.String("session_id", record.SessionID),
				zap.Error(err))
			continue
		}
		if !removed {
			// Raced a take or a transfer between the list and the delete.
			// Whoever won owns the row now.
			continue
		}
		move := record.Move
		s.logger.Warn("reaped stale pending move",
			append(pendingMoveLogFields(record.SessionID, &move), zap.String("reason", reason))...)
		s.removePendingMoveHandoffPrompt(ctx, record.SessionID, move.TaskID, normalizedPendingMoveID(record.SessionID, &move))
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
// between TakePendingMove and applyPendingMove. Returns true when the move was
// discarded, in which case the caller falls through to its normal
// on_turn_complete handling — the correct semantics for "as if no pending move
// existed".
//
// The row is already gone (TakePendingMove removed it), so discarding is just
// declining to apply it, plus dropping the correlated hand-off prompt.
func (s *Service) discardStalePendingMove(ctx context.Context, taskID, sessionID string, move *messagequeue.PendingMove) bool {
	if move == nil {
		return false
	}
	if !move.IsStaleAt(time.Now().UTC(), messagequeue.PendingMoveTTL) {
		return false
	}
	moveID := normalizedPendingMoveID(sessionID, move)
	move.MoveID = moveID
	s.logger.Warn("dropping expired pending move instead of applying it",
		append(pendingMoveLogFields(sessionID, move), zap.Duration("ttl", messagequeue.PendingMoveTTL))...)
	s.removePendingMoveHandoffPrompt(ctx, sessionID, taskID, moveID)
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
func pendingMoveLogFields(sessionID string, move *messagequeue.PendingMove) []zap.Field {
	return []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("task_id", move.TaskID),
		zap.String("workflow_id", move.WorkflowID),
		zap.String("target_step_id", move.WorkflowStepID),
		zap.String("move_id", normalizedPendingMoveID(sessionID, move)),
		zap.String("actor", move.Actor),
		zap.String("sender_session_id", move.SenderSessionID),
		zap.Time("queued_at", move.QueuedAt),
		zap.Duration("age", time.Since(move.QueuedAt)),
	}
}
