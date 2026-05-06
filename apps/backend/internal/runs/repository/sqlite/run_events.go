package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// AppendRunEvent inserts a new event row for a run. Seq is computed
// per-run by selecting MAX(seq)+1 inside the same statement so events
// land monotonically even when multiple writers race. Caller supplies
// only event_type, level, and an opaque JSON payload. The returned
// RunEvent carries the assigned seq + created_at so callers can
// publish it on the event bus without re-reading from disk.
func (r *Repository) AppendRunEvent(
	ctx context.Context,
	runID, eventType, level, payload string,
) (*models.RunEvent, error) {
	if level == "" {
		level = "info"
	}
	if payload == "" {
		payload = "{}"
	}
	now := time.Now().UTC()
	// Resolve next seq up-front so we can return it without a second read.
	var seq int
	if err := r.db.GetContext(ctx, &seq, r.db.Rebind(`
		SELECT COALESCE(MAX(seq) + 1, 0) FROM run_events WHERE run_id = ?
	`), runID); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO run_events (run_id, seq, event_type, level, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), runID, seq, eventType, level, payload, now); err != nil {
		return nil, err
	}
	return &models.RunEvent{
		RunID:     runID,
		Seq:       seq,
		EventType: eventType,
		Level:     level,
		Payload:   payload,
		CreatedAt: now,
	}, nil
}

// ListRunEvents returns events for a run in seq order. afterSeq>=0
// returns events strictly after that seq (used by the run detail's
// live tail to fetch only new events). limit caps the result; pass
// 0 for "no cap".
func (r *Repository) ListRunEvents(
	ctx context.Context, runID string, afterSeq, limit int,
) ([]*models.RunEvent, error) {
	q := `
		SELECT run_id, seq, event_type, level, payload, created_at
		FROM run_events
		WHERE run_id = ? AND seq > ?
		ORDER BY seq ASC
	`
	args := []interface{}{runID, afterSeq}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.RunEvent
	for rows.Next() {
		var e models.RunEvent
		if err := rows.StructScan(&e); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
