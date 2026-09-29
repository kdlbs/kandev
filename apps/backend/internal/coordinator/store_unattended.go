package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OpenDenial is one recorded denial whose unattended turn is still open.
type OpenDenial struct {
	TurnID    string
	TaskID    string
	SessionID string
	PendingID string
}

// RecordUnattendedDenial matches a permission request against the open
// unattended turn of the conversation task and session: the row's
// session_turn_id must equal activeTurnID or still be unbound. On a match it
// records (turn id, pendingID) once and counts it once, and returns the turn
// row id with matched true. A pendingID already recorded for the open turn
// also matches without counting again, so a redelivery still resolves. A turn
// that settles before the insert records nothing and does not match.
func (s *Store) RecordUnattendedDenial(ctx context.Context, taskID, sessionID, pendingID, activeTurnID string) (turnID string, matched bool, err error) {
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`
		SELECT id FROM coordinator_unattended_turns
		WHERE conversation_task_id = ? AND session_id = ? AND outcome IS NULL
		  AND (session_turn_id IS NULL OR session_turn_id = ?)`),
		taskID, sessionID, activeTurnID).Scan(&turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find unattended turn: %w", err)
	}
	if s.afterUnattendedLookup != nil {
		s.afterUnattendedLookup()
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin unattended denial: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO coordinator_unattended_denials (turn_id, pending_id, created_at)
		SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM coordinator_unattended_turns WHERE id = ? AND outcome IS NULL)
		ON CONFLICT DO NOTHING`), turnID, pendingID, time.Now().UTC(), turnID)
	if err != nil {
		return "", false, fmt.Errorf("record unattended denial: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return "", false, fmt.Errorf("count recorded denial: %w", err)
	}
	if inserted == 0 {
		var recorded int
		if err := tx.QueryRowContext(ctx, tx.Rebind(`
			SELECT COUNT(*) FROM coordinator_unattended_denials d
			JOIN coordinator_unattended_turns t ON t.id = d.turn_id
			WHERE d.turn_id = ? AND d.pending_id = ? AND t.outcome IS NULL`), turnID, pendingID).Scan(&recorded); err != nil {
			return "", false, fmt.Errorf("read recorded denial: %w", err)
		}
		if recorded == 0 {
			return "", false, nil
		}
		return turnID, true, nil
	}
	res, err = tx.ExecContext(ctx, tx.Rebind(`
		UPDATE coordinator_unattended_turns SET denied_permissions = denied_permissions + 1
		WHERE id = ? AND outcome IS NULL`), turnID)
	if err != nil {
		return "", false, fmt.Errorf("count unattended denial: %w", err)
	}
	counted, err := res.RowsAffected()
	if err != nil {
		return "", false, fmt.Errorf("count unattended denial rows: %w", err)
	}
	if counted == 0 {
		return "", false, nil
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit unattended denial: %w", err)
	}
	return turnID, true, nil
}

// ListOpenDenials returns the recorded denials of the coordinator's open
// unattended turns, oldest first, with the task and session of each turn.
func (s *Store) ListOpenDenials(ctx context.Context, coordinatorID string) ([]OpenDenial, error) {
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`
		SELECT d.turn_id, t.conversation_task_id, t.session_id, d.pending_id
		FROM coordinator_unattended_denials d
		JOIN coordinator_unattended_turns t ON t.id = d.turn_id
		WHERE t.coordinator_id = ? AND t.outcome IS NULL
		ORDER BY d.created_at, d.pending_id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("list open denials: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []OpenDenial
	for rows.Next() {
		var d OpenDenial
		if err := rows.Scan(&d.TurnID, &d.TaskID, &d.SessionID, &d.PendingID); err != nil {
			return nil, fmt.Errorf("scan open denial: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
