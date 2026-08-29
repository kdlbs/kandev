package messagequeue

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
)

// Sweep support for pending_moves.
//
// TakePendingMove can only reach a move whose session still reaches turn-end,
// so it is not enough on its own: session_id is UNIQUE, and a row whose keyed
// session is gone (or whose turn never ended cleanly) is removed by nothing.
// These two methods give the orchestrator's reaper a session-independent view
// of the table plus an unconditional delete. See
// orchestrator/pending_move_reaper.go for the policy that drives them.

// ListPendingMoves returns every armed deferred move, keyed by session.
func (r *sqliteRepository) ListPendingMoves(ctx context.Context) ([]PendingMoveRecord, error) {
	rows, err := r.ro.QueryxContext(ctx, `
		SELECT session_id, move_id, task_id, workflow_id, workflow_step_id, step_position, queued_at, actor, sender_session_id
		FROM pending_moves
	`)
	if err != nil {
		return nil, fmt.Errorf("list pending moves: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []PendingMoveRecord
	for rows.Next() {
		var (
			sessionID, moveID, taskID, workflowID, workflowStepID string
			position                                              int
			queuedAt                                              time.Time
			actor, senderSessionID                                string
		)
		if err := rows.Scan(&sessionID, &moveID, &taskID, &workflowID, &workflowStepID,
			&position, &queuedAt, &actor, &senderSessionID); err != nil {
			return nil, fmt.Errorf("scan pending move: %w", err)
		}
		records = append(records, PendingMoveRecord{
			SessionID: sessionID,
			Move: PendingMove{
				MoveID:          moveID,
				TaskID:          taskID,
				WorkflowID:      workflowID,
				WorkflowStepID:  workflowStepID,
				Position:        position,
				QueuedAt:        queuedAt,
				Actor:           actor,
				SenderSessionID: senderSessionID,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending moves: %w", err)
	}
	return records, nil
}

// DeletePendingMove removes the deferred move for a session, reporting whether
// a row was actually removed.
func (r *sqliteRepository) DeletePendingMove(ctx context.Context, sessionID string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete pending move tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Same per-session lock every other pending-move mutation takes. Without
	// it the sweep could delete a row that a concurrent TransferSession just
	// re-keyed, or race TakePendingMove across backend instances.
	if err := r.lockSessionTx(ctx, tx, sessionID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM pending_moves WHERE session_id = ?`), sessionID)
	if err != nil {
		return false, fmt.Errorf("delete pending move: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete pending move rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ListPendingMoves returns every armed deferred move, keyed by session.
func (r *memoryRepository) ListPendingMoves(_ context.Context) ([]PendingMoveRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pendingMoves) == 0 {
		return nil, nil
	}
	records := make([]PendingMoveRecord, 0, len(r.pendingMoves))
	for sessionID, move := range r.pendingMoves {
		records = append(records, PendingMoveRecord{SessionID: sessionID, Move: *move})
	}
	// Map iteration order is random; sort so callers (and tests) see a stable
	// sweep order.
	sort.Slice(records, func(i, j int) bool { return records[i].SessionID < records[j].SessionID })
	return records, nil
}

// DeletePendingMove removes the deferred move for a session, reporting whether
// a row was actually removed.
func (r *memoryRepository) DeletePendingMove(_ context.Context, sessionID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pendingMoves[sessionID]; !ok {
		return false, nil
	}
	delete(r.pendingMoves, sessionID)
	return true, nil
}

// ListPendingMoves returns every armed deferred move, keyed by session. Used by
// the orchestrator's sweep, which must see rows whose session will never emit
// another agent.ready.
func (s *Service) ListPendingMoves(ctx context.Context) ([]PendingMoveRecord, error) {
	records, err := s.repo.ListPendingMoves(ctx)
	if err != nil {
		s.logger.Error("list pending moves failed", zap.Error(err))
		return nil, err
	}
	return records, nil
}

// DeletePendingMove drops a session's deferred move without applying it.
// Reports whether a row was actually removed.
func (s *Service) DeletePendingMove(ctx context.Context, sessionID string) (bool, error) {
	removed, err := s.repo.DeletePendingMove(ctx, sessionID)
	if err != nil {
		s.logger.Error("delete pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false, err
	}
	return removed, nil
}
