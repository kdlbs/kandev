package sqlite

import (
	"context"
	"database/sql"
)

// ListContextContinuedSubmissionIDs classifies historical scope without
// completing, retiring or otherwise changing the original unknown work.
func (r *Repository) ListContextContinuedSubmissionIDs(ctx context.Context, sessionID, incarnationID, nativeSessionID string, generation int64) ([]string, error) {
	tx, err := r.ro.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return r.contextContinuedSubmissionIDsTx(ctx, tx, sessionID, incarnationID, nativeSessionID, generation)
}

func (r *Repository) contextContinuedSubmissionIDsTx(ctx context.Context, tx *sql.Tx, sessionID, incarnationID, nativeSessionID string, generation int64) ([]string, error) {
	if sessionID == "" || incarnationID == "" || nativeSessionID == "" || generation < 2 {
		return nil, nil
	}
	var current bool
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM task_sessions ts JOIN harness_session_generations g ON g.session_id = ts.id AND g.incarnation_id = ts.queue_incarnation_id
 WHERE ts.id = ? AND ts.queue_incarnation_id = ? AND ts.state <> 'CANCELLED'
 AND g.generation = ? AND g.native_session_id = ?
 AND NOT EXISTS (SELECT 1 FROM harness_session_generations newer WHERE newer.session_id = ts.id AND newer.incarnation_id = g.incarnation_id AND newer.generation > g.generation)
 )`), sessionID, incarnationID, generation, nativeSessionID).Scan(&current)
	if err != nil || !current {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`SELECT id, harness_generation FROM agent_delivery_submissions
 WHERE session_id = ? AND incarnation_id = ? AND state = 'interrupted_unknown' AND harness_generation > 0 AND harness_generation < ?
 AND terminal_sequence = 0 AND terminal_stream_id = ''`), sessionID, incarnationID, generation)
	if err != nil {
		return nil, err
	}
	type historicalRow struct {
		id         string
		generation int64
	}
	var candidates []historicalRow
	for rows.Next() {
		var row historicalRow
		if err := rows.Scan(&row.id, &row.generation); err != nil {
			_ = rows.Close()
			return nil, err
		}
		candidates = append(candidates, row)
	}
	readErr := rows.Err()
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	var continued []string
	for _, row := range candidates {
		valid, err := r.hasConsumedContextAncestryTx(ctx, tx, sessionID, incarnationID, row.id, row.generation, generation)
		if err != nil {
			return nil, err
		}
		if valid {
			continued = append(continued, row.id)
		}
	}
	return continued, nil
}
