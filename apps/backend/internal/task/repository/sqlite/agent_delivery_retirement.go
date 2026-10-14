package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

type agentDeliverySessionIdentity struct {
	sessionID     string
	incarnationID string
}

func (r *Repository) agentDeliverySessionIdentitiesInTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]agentDeliverySessionIdentity, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, queue_incarnation_id
		FROM task_sessions
		WHERE task_id = ?
		ORDER BY id`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task delivery identities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var identities []agentDeliverySessionIdentity
	for rows.Next() {
		var identity agentDeliverySessionIdentity
		if err := rows.Scan(&identity.sessionID, &identity.incarnationID); err != nil {
			return nil, fmt.Errorf("scan task delivery identity: %w", err)
		}
		if identity.sessionID == "" || identity.incarnationID == "" {
			continue
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task delivery identities: %w", err)
	}
	return identities, nil
}

func (r *Repository) retireAgentDeliverySessionsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identities []agentDeliverySessionIdentity,
) error {
	if err := r.lockAgentDeliverySessionIdentitiesTx(ctx, tx, identities); err != nil {
		return err
	}
	return r.insertAgentDeliveryRetirementJobsTx(ctx, tx, identities)
}

func (r *Repository) lockAgentDeliverySessionIdentitiesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identities []agentDeliverySessionIdentity,
) error {
	if len(identities) == 0 {
		return nil
	}
	sessionIDs := make([]string, 0, len(identities))
	for _, identity := range identities {
		sessionIDs = append(sessionIDs, identity.sessionID)
	}
	return lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), sessionIDs, nil)
}

func (r *Repository) insertAgentDeliveryRetirementJobsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identities []agentDeliverySessionIdentity,
) error {
	if len(identities) == 0 {
		return nil
	}
	if err := r.lockAgentDeliverySessionIdentitiesTx(ctx, tx, identities); err != nil {
		return err
	}
	if err := r.bindRetiringWorkflowReceiptsTx(ctx, tx, identities); err != nil {
		return err
	}
	now := r.nowUTC()
	query := r.db.Rebind(`
		INSERT INTO agent_delivery_cleanup_jobs
			(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?)
		ON CONFLICT (session_id, incarnation_id) DO NOTHING`)
	for _, identity := range identities {
		if _, err := tx.ExecContext(ctx, query, identity.sessionID, identity.incarnationID, now, now, now); err != nil {
			return fmt.Errorf("schedule delivery retirement for session %s: %w", identity.sessionID, err)
		}
	}
	return nil
}

func (r *Repository) bindRetiringWorkflowReceiptsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identities []agentDeliverySessionIdentity,
) error {
	query := r.db.Rebind(`
		UPDATE agent_delivery_effects AS e
		SET session_id = ?, incarnation_id = ?, harness_generation = (
		        SELECT c.harness_generation FROM agent_delivery_cursors c
		        WHERE c.stream_id = e.stream_id AND c.session_id = ? AND c.incarnation_id = ?
		        ORDER BY c.stream_id LIMIT 1
		    ),
		    submission_id = COALESCE(NULLIF(e.submission_id, ''), (
		        SELECT t.submission_id FROM agent_delivery_effects t
		        WHERE t.effect_type = ? AND t.stream_id = e.stream_id AND t.sequence = e.sequence
		          AND t.session_id = ? AND t.incarnation_id = ? AND t.turn_id <> ''
		        ORDER BY t.effect_key LIMIT 1
		    )),
		    turn_id = COALESCE(NULLIF(e.turn_id, ''), (
		        SELECT t.turn_id FROM agent_delivery_effects t
		        WHERE t.effect_type = ? AND t.stream_id = e.stream_id AND t.sequence = e.sequence
		          AND t.session_id = ? AND t.incarnation_id = ? AND t.turn_id <> ''
		        ORDER BY t.effect_key LIMIT 1
		    ))
		WHERE e.effect_type = 'workflow.on_turn_complete'
		  AND e.session_id = '' AND e.incarnation_id = ''
		  AND EXISTS (
		      SELECT 1 FROM agent_delivery_cursors c
		      WHERE c.stream_id = e.stream_id AND c.session_id = ? AND c.incarnation_id = ?
		  )
		  AND EXISTS (
		      SELECT 1 FROM agent_delivery_effects t
		      WHERE t.effect_type = ? AND t.stream_id = e.stream_id AND t.sequence = e.sequence
		        AND t.session_id = ? AND t.incarnation_id = ? AND t.turn_id <> ''
		  )`)
	for _, identity := range identities {
		if _, err := tx.ExecContext(ctx, query,
			identity.sessionID, identity.incarnationID,
			identity.sessionID, identity.incarnationID,
			models.DeliveryTerminalEffectType, identity.sessionID, identity.incarnationID,
			models.DeliveryTerminalEffectType, identity.sessionID, identity.incarnationID,
			identity.sessionID, identity.incarnationID,
			models.DeliveryTerminalEffectType, identity.sessionID, identity.incarnationID,
		); err != nil {
			return fmt.Errorf("bind workflow receipts before session retirement: %w", err)
		}
	}
	return nil
}

func (r *Repository) retireAgentDeliveryTaskSessionsTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	identities, err := r.agentDeliverySessionIdentitiesInTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	return r.retireAgentDeliverySessionsTx(ctx, tx, identities)
}
