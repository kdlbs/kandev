package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) EnsureRestartRecoveryBlock(
	ctx context.Context,
	candidate *models.SessionRecoveryBlock,
) (*models.SessionRecoveryBlock, bool, error) {
	if err := validateRestartRecoveryBlockCandidate(candidate); err != nil {
		return nil, false, err
	}
	block := *candidate
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), block.SessionID); err != nil {
		return nil, false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{block.SessionID}, nil); err != nil {
		return nil, false, err
	}
	current, err := restartRecoveryCandidateIsCurrentTx(ctx, tx, r.db.Rebind, block)
	if err != nil || !current {
		return nil, false, err
	}
	open, err := r.listOpenRestartRecoveryBlocksTx(ctx, tx, block)
	if err != nil {
		return nil, false, err
	}
	if len(open) != 0 {
		existing := reusableRestartRecoveryBlock(open)
		return commitRestartRecoveryBlockResult(tx, existing, false)
	}
	repeated, err := r.matchesResolvedRestartRecoveryIncidentTx(ctx, tx, block)
	if err != nil {
		return nil, false, err
	}
	if repeated {
		return commitRestartRecoveryBlockResult(tx, nil, false)
	}
	inserted, err := r.insertRestartRecoveryBlockTx(ctx, tx, &block)
	if err != nil || !inserted {
		return nil, false, err
	}
	if err := r.advanceRecoverySessionRevisionTx(ctx, tx, block.SessionID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return &block, true, nil
}

func commitRestartRecoveryBlockResult(
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
	created bool,
) (*models.SessionRecoveryBlock, bool, error) {
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return block, created, nil
}

func (r *Repository) matchesResolvedRestartRecoveryIncidentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block models.SessionRecoveryBlock,
) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM session_recovery_blocks resolved
		WHERE resolved.session_id = ? AND resolved.incarnation_id = ? AND resolved.expected_generation = ?
		AND resolved.reason = ? AND resolved.state = 'resolved' AND resolved.consumer_reference = 'agent_delivery'`
	args := []any{block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason}
	if block.Reason == deliveryUnknownPromptReason {
		query += ` AND resolved.delivery_submission_id = ?)`
		args = append(args, block.DeliverySubmissionID)
	} else {
		query += ` AND (resolved.resolved_at IS NULL OR NOT EXISTS (
			SELECT 1 FROM agent_delivery_submissions successor
			WHERE successor.session_id = resolved.session_id AND successor.incarnation_id = resolved.incarnation_id
			AND successor.harness_generation = resolved.expected_generation
			AND successor.state IN ('dispatching', 'interrupted_unknown')
			AND successor.created_at > resolved.resolved_at
		)))`
	}
	var repeated bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(&repeated)
	return repeated, err
}

func validateRestartRecoveryBlockCandidate(block *models.SessionRecoveryBlock) error {
	if block == nil {
		return fmt.Errorf("invalid restart recovery block candidate")
	}
	if block.SessionID == "" || block.IncarnationID == "" || block.ExpectedGeneration <= 0 {
		return fmt.Errorf("invalid restart recovery block candidate")
	}
	if block.ConsumerReference != reconstructionDeliveryConsumer || block.State != models.RecoveryBlockOpen {
		return fmt.Errorf("invalid restart recovery block candidate")
	}
	if block.DeliverySequence != 0 || block.DeliveryTurnID != "" || block.DeliveryOutcome != "" ||
		block.AuthorizedAction != "" || block.ResolvedAt != nil {
		return fmt.Errorf("invalid restart recovery block candidate")
	}
	return validateRestartRecoveryBlockBinding(block)
}

func validateRestartRecoveryBlockBinding(block *models.SessionRecoveryBlock) error {
	switch block.Reason {
	case deliveryUnknownPromptReason:
		if block.DeliverySubmissionID == "" {
			return fmt.Errorf("submission-bound restart recovery block is missing its submission")
		}
	case deliveryUnresolvedWorkReason:
		if block.DeliverySubmissionID != "" || block.DeliveryStreamID != "" {
			return fmt.Errorf("generic restart recovery block has a delivery binding")
		}
	default:
		return fmt.Errorf("unsupported restart recovery block reason")
	}
	return nil
}

func reusableRestartRecoveryBlock(open []models.SessionRecoveryBlock) *models.SessionRecoveryBlock {
	if len(open) != 1 || !restartRecoveryFenceEligible(open[0]) {
		return nil
	}
	return &open[0]
}

func restartRecoveryCandidateIsCurrentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	block models.SessionRecoveryBlock,
) (bool, error) {
	var current bool
	err := tx.QueryRowxContext(ctx, rebind(`SELECT EXISTS (
		SELECT 1 FROM task_sessions ts JOIN harness_session_generations g
		ON g.session_id = ts.id AND g.incarnation_id = ts.queue_incarnation_id
		WHERE ts.id = ? AND ts.queue_incarnation_id = ? AND g.generation = ?
		AND g.generation = (SELECT MAX(generation) FROM harness_session_generations
			WHERE session_id = ts.id AND incarnation_id = ts.queue_incarnation_id)
		)`), block.SessionID, block.IncarnationID, block.ExpectedGeneration).Scan(&current)
	return current, err
}

func (r *Repository) listOpenRestartRecoveryBlocksTx(
	ctx context.Context,
	tx *sqlx.Tx,
	owner models.SessionRecoveryBlock,
) ([]models.SessionRecoveryBlock, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		consumer_reference, delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id,
		delivery_outcome, authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks WHERE session_id = ? AND incarnation_id = ?
		AND expected_generation = ? AND state = ? ORDER BY created_at, id`),
		owner.SessionID, owner.IncarnationID, owner.ExpectedGeneration, models.RecoveryBlockOpen)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var blocks []models.SessionRecoveryBlock
	for rows.Next() {
		var block models.SessionRecoveryBlock
		if err := rows.Scan(&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration,
			&block.Reason, &block.State, &block.ConsumerReference, &block.DeliverySubmissionID, &block.DeliveryStreamID,
			&block.DeliverySequence, &block.DeliveryTurnID, &block.DeliveryOutcome, &block.AuthorizedAction,
			&block.CreatedAt, &block.UpdatedAt, &block.ResolvedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func restartRecoveryFenceEligible(block models.SessionRecoveryBlock) bool {
	if block.ID == "" || block.ConsumerReference != reconstructionDeliveryConsumer || block.State != models.RecoveryBlockOpen ||
		block.DeliverySequence != 0 || block.DeliveryTurnID != "" ||
		(block.DeliveryOutcome != "" && block.DeliveryOutcome != reconstructionUnknownOutcome) {
		return false
	}
	switch block.Reason {
	case deliveryUnknownPromptReason:
		return block.DeliverySubmissionID != ""
	case deliveryUnresolvedWorkReason:
		return block.DeliverySubmissionID == "" && block.DeliveryStreamID == ""
	default:
		return false
	}
}

func (r *Repository) insertRestartRecoveryBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
) (bool, error) {
	if err := prepareSessionRecoveryBlock(block); err != nil {
		return false, err
	}
	if err := r.bindProjectedTerminalToRecoveryBlock(ctx, tx, block); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
		delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
		authorized_action, created_at, updated_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, incarnation_id, expected_generation, reason) WHERE state = 'open' DO NOTHING`),
		block.ID, block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason, block.State,
		block.ConsumerReference, block.DeliverySubmissionID, block.DeliveryStreamID, block.DeliverySequence,
		block.DeliveryTurnID, block.DeliveryOutcome, block.AuthorizedAction, block.CreatedAt, block.UpdatedAt, block.ResolvedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
