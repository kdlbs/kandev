package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ResolveInactiveAgentDeliveryRecoveryBlock clears only a bound delivery block
// whose canonical submission can no longer be dispatched automatically.
func (r *Repository) ResolveInactiveAgentDeliveryRecoveryBlock(
	ctx context.Context,
	expected *models.SessionRecoveryBlock,
	resolvedAt time.Time,
) (bool, error) {
	if !validInactiveDeliveryRecoveryBlock(expected) {
		return false, nil
	}
	if resolvedAt.IsZero() {
		resolvedAt = r.nowUTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := r.resolveInactiveAgentDeliveryRecoveryBlockTx(ctx, tx, expected, resolvedAt)
	if err != nil || !changed {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validInactiveDeliveryRecoveryBlock(block *models.SessionRecoveryBlock) bool {
	return block != nil && block.ID != "" && block.SessionID != "" && block.IncarnationID != "" &&
		block.ExpectedGeneration > 0 && block.ConsumerReference == "agent_delivery" &&
		block.DeliverySubmissionID != "" && block.State == models.RecoveryBlockOpen
}

func (r *Repository) resolveInactiveAgentDeliveryRecoveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	expected *models.SessionRecoveryBlock,
	resolvedAt time.Time,
) (bool, error) {
	block, valid, err := r.lockInactiveDeliveryRecoveryBlockTx(ctx, tx, expected)
	if err != nil || !valid {
		return false, err
	}
	state, err := r.inactiveDeliverySubmissionStateTx(ctx, tx, block)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if !deliverySubmissionNoLongerInFlight(state) {
		return false, nil
	}
	recovery := models.AgentDeliveryRecovery{
		SessionID: block.SessionID, IncarnationID: block.IncarnationID,
		HarnessGeneration: block.ExpectedGeneration, SubmissionID: block.DeliverySubmissionID,
	}
	effect, err := r.latestTerminalRecoveryEffectTx(ctx, tx, recovery)
	if err != nil {
		return false, err
	}
	if err := r.advanceRecoverySessionRevisionTx(ctx, tx, block.SessionID); err != nil {
		return false, err
	}
	return r.resolveInactiveDeliveryBlockTx(ctx, tx, block, effect, resolvedAt)
}

func (r *Repository) lockInactiveDeliveryRecoveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	expected *models.SessionRecoveryBlock,
) (*models.SessionRecoveryBlock, bool, error) {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), expected.SessionID); err != nil {
		return nil, false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{expected.SessionID}, []string{expected.DeliveryStreamID}); err != nil {
		return nil, false, err
	}
	if err := lockRecoverySessionRowTx(ctx, tx, r.db, expected.SessionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	block, err := r.loadRecoveryBlockForInactiveDeliveryTx(ctx, tx, expected.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return block, sameOpenDeliveryRecoveryBlock(block, expected), nil
}

func lockRecoverySessionRowTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID string) error {
	query := `SELECT id FROM task_sessions WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += forUpdateClause
	}
	var lockedID string
	return tx.QueryRowxContext(ctx, db.Rebind(query), sessionID).Scan(&lockedID)
}

func (r *Repository) loadRecoveryBlockForInactiveDeliveryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
) (*models.SessionRecoveryBlock, error) {
	query := `SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		consumer_reference, delivery_submission_id, delivery_stream_id
		FROM session_recovery_blocks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var block models.SessionRecoveryBlock
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), id).Scan(
		&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration,
		&block.Reason, &block.State, &block.ConsumerReference, &block.DeliverySubmissionID,
		&block.DeliveryStreamID,
	)
	return &block, err
}

func sameOpenDeliveryRecoveryBlock(actual, expected *models.SessionRecoveryBlock) bool {
	return actual != nil && expected != nil && actual.ID == expected.ID &&
		actual.SessionID == expected.SessionID && actual.IncarnationID == expected.IncarnationID &&
		actual.ExpectedGeneration == expected.ExpectedGeneration && actual.Reason == expected.Reason &&
		actual.State == models.RecoveryBlockOpen && actual.ConsumerReference == "agent_delivery" &&
		actual.DeliverySubmissionID == expected.DeliverySubmissionID &&
		actual.DeliveryStreamID == expected.DeliveryStreamID
}

func (r *Repository) inactiveDeliverySubmissionStateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
) (models.DeliverySubmissionState, error) {
	query := `SELECT state FROM agent_delivery_submissions
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var state models.DeliverySubmissionState
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), block.DeliverySubmissionID,
		block.SessionID, block.IncarnationID, block.ExpectedGeneration).Scan(&state)
	return state, err
}

func deliverySubmissionNoLongerInFlight(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionCompleted || state == models.DeliverySubmissionFailed ||
		state == models.DeliverySubmissionCancelled || state == models.DeliverySubmissionInterruptedUnknown
}

func (r *Repository) resolveInactiveDeliveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
	effect *models.AgentDeliveryEffect,
	resolvedAt time.Time,
) (bool, error) {
	if effect != nil {
		return r.updateDeliveryBlockTerminalBindingTx(ctx, tx, effect, deliveryRecoveryBlockIdentity{
			id: block.ID, reason: block.Reason, streamID: block.DeliveryStreamID,
		}, resolvedAt)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_recovery_blocks
		SET state = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
		  AND reason = ? AND state = ? AND consumer_reference = 'agent_delivery'
		  AND delivery_submission_id = ? AND delivery_stream_id = ?`),
		models.RecoveryBlockResolved, "retry_connection", resolvedAt, resolvedAt,
		block.ID, block.SessionID, block.IncarnationID, block.ExpectedGeneration,
		block.Reason, models.RecoveryBlockOpen, block.DeliverySubmissionID, block.DeliveryStreamID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
