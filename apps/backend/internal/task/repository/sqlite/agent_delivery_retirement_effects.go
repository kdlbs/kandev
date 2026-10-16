package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

type agentDeliveryRetirementPhaseResult struct {
	deleted  int
	scanned  int
	complete bool
}

func (r *Repository) deleteAgentDeliveryRetirementEffectsTx(
	ctx context.Context, tx *sqlx.Tx, job *agentDeliveryRetirementJob, limit int,
) (agentDeliveryRetirementPhaseResult, error) {
	var result agentDeliveryRetirementPhaseResult
	owned, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM agent_delivery_effects WHERE effect_key IN (
		SELECT effect_key FROM agent_delivery_effects
		WHERE session_id = ? AND session_id <> '' ORDER BY effect_key LIMIT ?)`), job.sessionID, limit)
	if err != nil {
		return result, fmt.Errorf("purge owned agent delivery effects: %w", err)
	}
	rows, err := owned.RowsAffected()
	if err != nil {
		return result, err
	}
	result.deleted = int(rows)
	remaining := limit - result.deleted
	if remaining == 0 {
		return result, nil
	}
	var streams []string
	if err := tx.SelectContext(ctx, &streams, r.db.Rebind(`SELECT stream_id FROM agent_delivery_cursors
		WHERE session_id = ? AND stream_id > ? ORDER BY stream_id LIMIT ?`), job.sessionID, job.lastStreamID, remaining); err != nil {
		return result, fmt.Errorf("select retired cursor page: %w", err)
	}
	result.scanned = result.deleted + len(streams)
	if len(streams) == 0 {
		result.complete = true
		return result, nil
	}
	attributed, err := r.deleteAgentDeliveryCursorEffectsTx(ctx, tx, job, remaining)
	if err != nil {
		return result, err
	}
	result.scanned = result.deleted + max(len(streams), attributed)
	result.deleted += attributed
	// A full effect page may leave receipts on any selected stream. Only an
	// exhausted page advances its cursor frontier; deletion and progress commit together.
	if attributed < remaining {
		job.lastStreamID = streams[len(streams)-1]
		result.complete = len(streams) < remaining
	}
	return result, nil
}

func (r *Repository) deleteAgentDeliveryCursorEffectsTx(
	ctx context.Context, tx *sqlx.Tx, job *agentDeliveryRetirementJob, limit int,
) (int, error) {
	query := `WITH cursor_page AS MATERIALIZED (
		SELECT stream_id FROM agent_delivery_cursors
		WHERE session_id = ? AND stream_id > ? ORDER BY stream_id LIMIT ?
	) DELETE FROM agent_delivery_effects WHERE effect_key IN (
		SELECT e.effect_key FROM cursor_page c JOIN agent_delivery_effects e ON e.stream_id = c.stream_id LIMIT ?)`
	args := []any{job.sessionID, job.lastStreamID, limit, limit}
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `WITH cursor_page AS MATERIALIZED (
			SELECT stream_id FROM agent_delivery_cursors
			WHERE session_id = $1 AND stream_id > $2 ORDER BY stream_id LIMIT $3
		) DELETE FROM agent_delivery_effects WHERE effect_key IN (
			SELECT e.effect_key FROM cursor_page c JOIN LATERAL (
				SELECT effect_key FROM agent_delivery_effects
				WHERE stream_id = c.stream_id ORDER BY sequence LIMIT $3
			) e ON TRUE LIMIT $3)`
		args = args[:3]
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("purge cursor-attributed agent delivery effects: %w", err)
	}
	rows, err := res.RowsAffected()
	return int(rows), err
}
