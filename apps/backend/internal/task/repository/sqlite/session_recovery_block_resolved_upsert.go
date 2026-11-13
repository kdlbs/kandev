package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

type storedRecoveryBlockBinding struct {
	ID                   string `db:"id"`
	State                string `db:"state"`
	ConsumerReference    string `db:"consumer_reference"`
	DeliverySubmissionID string `db:"delivery_submission_id"`
	DeliveryStreamID     string `db:"delivery_stream_id"`
}

type recoveryBlockLookup struct {
	where string
	args  []any
}

func (r *Repository) upsertResolvedSessionRecoveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
) (bool, error) {
	queries := []recoveryBlockLookup{
		{
			where: `session_id = ? AND incarnation_id = ? AND expected_generation = ? AND reason = ? AND state = ?`,
			args:  []any{block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason, models.RecoveryBlockOpen},
		},
		{
			where: `id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ? AND reason = ? AND state = ?`,
			args:  []any{block.ID, block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason, models.RecoveryBlockResolved},
		},
	}
	if block.ConsumerReference == "agent_delivery" && block.DeliverySubmissionID != "" {
		queries = append(queries, recoveryBlockLookup{
			where: `session_id = ? AND incarnation_id = ? AND expected_generation = ? AND reason = ?
				AND state = ? AND consumer_reference = ? AND delivery_submission_id = ?
				ORDER BY resolved_at DESC, updated_at DESC, id LIMIT 1`,
			args: []any{block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason,
				models.RecoveryBlockResolved, block.ConsumerReference, block.DeliverySubmissionID},
		})
	}

	for _, query := range queries {
		handled, err := r.tryUpsertResolvedSessionRecoveryBlockByLookup(ctx, tx, block, query)
		if handled || err != nil {
			return handled, err
		}
	}
	return false, nil
}

func (r *Repository) tryUpsertResolvedSessionRecoveryBlockByLookup(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
	lookup recoveryBlockLookup,
) (bool, error) {
	stored, err := r.findRecoveryBlockBindingTx(ctx, tx, lookup.where, lookup.args...)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.updateResolvedSessionRecoveryBlockTx(ctx, tx, block, stored)
}

func (r *Repository) updateResolvedSessionRecoveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
	stored *storedRecoveryBlockBinding,
) (bool, error) {
	submissionDiffers := stored.ConsumerReference == reconstructionDeliveryConsumer &&
		block.ConsumerReference == reconstructionDeliveryConsumer &&
		stored.DeliverySubmissionID != block.DeliverySubmissionID
	if submissionDiffers && stored.State != models.RecoveryBlockOpen {
		return false, nil
	}
	block.ID = stored.ID
	if submissionDiffers {
		_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks SET updated_at = ? WHERE id = ?`),
			block.UpdatedAt, stored.ID)
		return true, err
	}
	streamID := block.DeliveryStreamID
	if stored.ConsumerReference == reconstructionDeliveryConsumer && block.ConsumerReference == reconstructionDeliveryConsumer &&
		stored.DeliverySubmissionID == block.DeliverySubmissionID && streamID == "" {
		streamID = stored.DeliveryStreamID
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks SET state = ?,
		consumer_reference = ?, delivery_submission_id = ?, delivery_stream_id = ?, delivery_sequence = ?,
		delivery_turn_id = ?, delivery_outcome = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
		WHERE id = ?`), block.State, block.ConsumerReference, block.DeliverySubmissionID, streamID,
		block.DeliverySequence, block.DeliveryTurnID, block.DeliveryOutcome, block.AuthorizedAction,
		block.UpdatedAt, block.ResolvedAt, stored.ID)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if updated != 1 {
		return false, fmt.Errorf("update resolved session recovery block %q: affected %d rows", stored.ID, updated)
	}
	return true, nil
}

func (r *Repository) findRecoveryBlockBindingTx(
	ctx context.Context,
	tx *sqlx.Tx,
	where string,
	args ...any,
) (*storedRecoveryBlockBinding, error) {
	query := `SELECT id, state, consumer_reference, delivery_submission_id, delivery_stream_id
		FROM session_recovery_blocks WHERE ` + where
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var stored storedRecoveryBlockBinding
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(
		&stored.ID, &stored.State, &stored.ConsumerReference, &stored.DeliverySubmissionID, &stored.DeliveryStreamID,
	); err != nil {
		return nil, err
	}
	return &stored, nil
}
