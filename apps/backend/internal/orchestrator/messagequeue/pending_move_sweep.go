package messagequeue

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Sweep support for pending_moves.
//
// TakePendingMove can only reach a move whose session still reaches turn-end,
// so it is not enough on its own: session_id is UNIQUE, and a row whose keyed
// session is gone (or whose turn never ended cleanly) is removed by nothing.
// These two methods give the orchestrator's reaper a session-independent view
// of the table plus an exact-row atomic discard. See
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

// DeletePendingMoveIfMatch removes the deferred move only when it is still the
// exact row the caller inspected. A fresh move can replace a listed row before
// the sweep reaches DELETE; matching move_id and queued_at preserves that
// replacement. The optional hand-off prompt is deleted after the comparison
// succeeds but before commit, so either both deletes commit or both roll back.
func (r *sqliteRepository) DeletePendingMoveIfMatch(
	ctx context.Context,
	expected PendingMoveRecord,
	handoffEntryID string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete pending move tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Same per-session lock every other pending-move mutation takes. Without
	// it the sweep could delete a row that a concurrent TransferSession just
	// re-keyed, or race TakePendingMove across backend instances.
	if err := r.lockSessionTx(ctx, tx, expected.SessionID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM pending_moves
		WHERE session_id = ? AND move_id = ? AND queued_at = ?
	`), expected.SessionID, expected.Move.MoveID, expected.Move.QueuedAt)
	if err != nil {
		return false, fmt.Errorf("delete pending move: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete pending move rows affected: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	if handoffEntryID != "" {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			DELETE FROM queued_messages
			WHERE id = ? AND session_id = ? AND task_id = ? AND queued_by = ?
		`), handoffEntryID, expected.SessionID, expected.Move.TaskID, QueuedByMoveTask); err != nil {
			return false, fmt.Errorf("delete pending move hand-off prompt: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
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

// DeletePendingMoveIfMatch removes the deferred move and optional hand-off
// prompt under one lock, but only when the move still matches the inspected
// row.
func (r *memoryRepository) DeletePendingMoveIfMatch(
	_ context.Context,
	expected PendingMoveRecord,
	handoffEntryID string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.pendingMoves[expected.SessionID]
	if !ok || stored.MoveID != expected.Move.MoveID || !stored.QueuedAt.Equal(expected.Move.QueuedAt) {
		return false, nil
	}
	delete(r.pendingMoves, expected.SessionID)
	if handoffEntryID != "" {
		entries := r.entries[expected.SessionID]
		for i, entry := range entries {
			if entry.ID != handoffEntryID || entry.TaskID != expected.Move.TaskID || entry.QueuedBy != QueuedByMoveTask {
				continue
			}
			r.entries[expected.SessionID] = append(entries[:i], entries[i+1:]...)
			if len(r.entries[expected.SessionID]) == 0 {
				delete(r.entries, expected.SessionID)
				delete(r.nextPosition, expected.SessionID)
			}
			break
		}
	}
	return true, nil
}

// ListPendingMoves returns every armed deferred move, keyed by session. Used by
// the orchestrator's sweep, which must see rows whose session will never emit
// another agent.ready.
func (s *Service) ListPendingMoves(ctx context.Context) ([]PendingMoveRecord, error) {
	return s.repo.ListPendingMoves(ctx)
}

// DeletePendingMoveIfMatch atomically drops an exact deferred move and its
// optional correlated hand-off prompt without applying either one.
func (s *Service) DeletePendingMoveIfMatch(
	ctx context.Context,
	expected PendingMoveRecord,
	handoffEntryID string,
) (bool, error) {
	return s.repo.DeletePendingMoveIfMatch(ctx, expected, handoffEntryID)
}
