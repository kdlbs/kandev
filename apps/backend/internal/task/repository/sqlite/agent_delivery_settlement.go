package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func deliveryTerminalSettlementEffect(event *models.AgentDeliveryEvent) (*models.AgentDeliveryEffect, error) {
	if event == nil || !event.Terminal || event.SubmissionID == "" || event.Sequence <= 0 || event.StreamID == "" {
		return nil, nil
	}
	outcome := deliveryTerminalOutcome(event.EventType)
	if outcome == "" {
		return nil, nil
	}
	var payload streams.AgentEvent
	_ = json.Unmarshal(event.Payload, &payload)
	now := time.Now().UTC()
	return &models.AgentDeliveryEffect{
		EffectKey:         fmt.Sprintf("agent_delivery.terminal:%s:%d", event.StreamID, event.Sequence),
		StreamID:          event.StreamID,
		Sequence:          event.Sequence,
		EffectType:        models.DeliveryTerminalEffectType,
		State:             models.DeliveryEffectPending,
		SessionID:         event.SessionID,
		IncarnationID:     event.IncarnationID,
		HarnessGeneration: event.HarnessGeneration,
		SubmissionID:      event.SubmissionID,
		TurnID:            payload.TurnID,
		Outcome:           string(outcome),
		CreatedAt:         now,
	}, nil
}

func deliveryTerminalOutcome(eventType string) models.DeliverySubmissionState {
	switch eventType {
	case streams.EventTypeComplete:
		return models.DeliverySubmissionCompleted
	case streams.EventTypeError:
		return models.DeliverySubmissionFailed
	case "cancelled", "canceled":
		return models.DeliverySubmissionCancelled
	default:
		return ""
	}
}

// SettleAgentDeliveryTerminal applies an already-projected terminal effect to
// its exact submission and matching delivery recovery blocks. A pending effect
// remains durable if projection committed but this transaction did not run.
func (r *Repository) SettleAgentDeliveryTerminal(
	ctx context.Context,
	streamID string,
	sequence int64,
	outcome models.DeliverySubmissionState,
	settledAt time.Time,
) (bool, error) {
	if streamID == "" || sequence <= 0 {
		return false, fmt.Errorf("terminal stream and positive sequence are required")
	}
	if settledAt.IsZero() {
		settledAt = r.nowUTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	return r.settleAgentDeliveryTerminalTx(ctx, tx, streamID, sequence, outcome, settledAt)
}

type terminalDeliverySettlement struct {
	effect         *models.AgentDeliveryEffect
	currentState   models.DeliverySubmissionState
	latestSequence int64
}

func (r *Repository) settleAgentDeliveryTerminalTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
	sequence int64,
	outcome models.DeliverySubmissionState,
	settledAt time.Time,
) (bool, error) {
	state, err := r.loadTerminalDeliverySettlementTx(ctx, tx, streamID, sequence, outcome)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	if state.latestSequence > sequence {
		if err := completeSupersededDeliveryEffectTx(ctx, tx, r.db, state.effect.EffectKey, settledAt); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	if state.currentState != outcome {
		updated, err := updateDeliverySubmissionOutcomeTx(ctx, tx, r.db, state.effect, outcome, settledAt)
		if err != nil {
			return false, err
		}
		if !updated {
			return false, tx.Commit()
		}
	}
	if err := r.resolveBoundDeliveryBlocksTx(ctx, tx, state.effect, settledAt); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_effects SET state = ?, completed_at = NULL
		WHERE effect_key = ? AND state != ?`),
		models.DeliveryEffectBlockResolved, state.effect.EffectKey, models.DeliveryEffectCompleted); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) loadTerminalDeliverySettlementTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
	sequence int64,
	outcome models.DeliverySubmissionState,
) (*terminalDeliverySettlement, error) {
	effect, err := r.loadTerminalDeliveryEffectTx(ctx, tx, streamID, sequence)
	if err != nil || !deliveryEffectMatchesOutcome(effect, outcome) {
		return nil, err
	}
	projected, exists, err := r.projectedDeliverySequenceTx(ctx, tx, streamID)
	if err != nil || !exists || projected < sequence {
		return nil, err
	}
	currentState, matches, err := loadDeliverySubmissionStateTx(ctx, tx, r.db, effect)
	if err != nil || !matches {
		return nil, err
	}
	latestSequence, err := latestDeliveryTerminalSequenceTx(ctx, tx, r.db, effect)
	if err != nil {
		return nil, err
	}
	return &terminalDeliverySettlement{
		effect: effect, currentState: currentState, latestSequence: latestSequence,
	}, nil
}

func deliveryEffectMatchesOutcome(effect *models.AgentDeliveryEffect, outcome models.DeliverySubmissionState) bool {
	return effect != nil && effect.Outcome == string(outcome) && validDeliveryTerminalOutcome(outcome)
}

func (r *Repository) projectedDeliverySequenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
) (int64, bool, error) {
	var projected int64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT projected_sequence FROM agent_delivery_cursors WHERE stream_id = ?`), streamID).Scan(&projected)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return projected, err == nil, err
}

func loadDeliverySubmissionStateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	effect *models.AgentDeliveryEffect,
) (models.DeliverySubmissionState, bool, error) {
	var sessionID, incarnationID string
	var generation int64
	var state models.DeliverySubmissionState
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, state
		FROM agent_delivery_submissions WHERE id = ?`), effect.SubmissionID).Scan(
		&sessionID, &incarnationID, &generation, &state)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	matches := sessionID == effect.SessionID && incarnationID == effect.IncarnationID &&
		generation == effect.HarnessGeneration
	return state, matches, nil
}

func latestDeliveryTerminalSequenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	effect *models.AgentDeliveryEffect,
) (int64, error) {
	var sequence int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT COALESCE(MAX(sequence), 0) FROM agent_delivery_effects
		WHERE effect_type = ? AND submission_id = ? AND stream_id = ?`),
		models.DeliveryTerminalEffectType, effect.SubmissionID, effect.StreamID).Scan(&sequence)
	return sequence, err
}

func completeSupersededDeliveryEffectTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	effectKey string,
	completedAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE agent_delivery_effects SET state = ?, completed_at = ?
		WHERE effect_key = ? AND state != ?`),
		models.DeliveryEffectCompleted, completedAt, effectKey, models.DeliveryEffectCompleted)
	return err
}

func updateDeliverySubmissionOutcomeTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	effect *models.AgentDeliveryEffect,
	outcome models.DeliverySubmissionState,
	settledAt time.Time,
) (bool, error) {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE agent_delivery_submissions SET state = ?, outcome = ?, updated_at = ?
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?`),
		outcome, string(outcome), settledAt, effect.SubmissionID, effect.SessionID,
		effect.IncarnationID, effect.HarnessGeneration)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *Repository) resolveBoundDeliveryBlocksTx(
	ctx context.Context,
	tx *sqlx.Tx,
	effect *models.AgentDeliveryEffect,
	settledAt time.Time,
) error {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, reason FROM session_recovery_blocks
		WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ?
		  AND consumer_reference = 'agent_delivery' AND delivery_submission_id = ?
		  AND (delivery_stream_id = '' OR delivery_stream_id = ?) AND state = ?`),
		effect.SessionID, effect.IncarnationID, effect.HarnessGeneration,
		effect.SubmissionID, effect.StreamID, models.RecoveryBlockOpen)
	if err != nil {
		return err
	}
	type blockIdentity struct{ id, reason string }
	var blocks []blockIdentity
	for rows.Next() {
		var block blockIdentity
		if err := rows.Scan(&block.id, &block.reason); err != nil {
			_ = rows.Close()
			return err
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, block := range blocks {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE session_recovery_blocks
			SET state = ?, delivery_stream_id = ?, delivery_sequence = ?, delivery_turn_id = ?,
			    delivery_outcome = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
			WHERE id = ? AND reason = ? AND session_id = ? AND incarnation_id = ?
			  AND expected_generation = ? AND consumer_reference = 'agent_delivery'
			  AND delivery_submission_id = ? AND (delivery_stream_id = '' OR delivery_stream_id = ?)
			  AND state = ?`),
			models.RecoveryBlockResolved, effect.StreamID, effect.Sequence, effect.TurnID,
			effect.Outcome, "durable_delivery_"+effect.Outcome, settledAt, settledAt,
			block.id, block.reason, effect.SessionID, effect.IncarnationID,
			effect.HarnessGeneration, effect.SubmissionID, effect.StreamID, models.RecoveryBlockOpen)
		if err != nil {
			return err
		}
		if _, err := result.RowsAffected(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) loadTerminalDeliveryEffectTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
	sequence int64,
) (*models.AgentDeliveryEffect, error) {
	effectKey := fmt.Sprintf("agent_delivery.terminal:%s:%d", streamID, sequence)
	var effect models.AgentDeliveryEffect
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		       session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome
		FROM agent_delivery_effects WHERE effect_key = ?`), effectKey).Scan(
		&effect.EffectKey, &effect.StreamID, &effect.Sequence, &effect.EffectType,
		&effect.State, &effect.CreatedAt, &effect.CompletedAt, &effect.SessionID,
		&effect.IncarnationID, &effect.HarnessGeneration, &effect.SubmissionID,
		&effect.TurnID, &effect.Outcome)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if effect.EffectType != models.DeliveryTerminalEffectType || effect.Sequence != sequence || effect.StreamID != streamID {
		return nil, fmt.Errorf("terminal delivery effect identity conflict")
	}
	return &effect, nil
}

func (r *Repository) ListPendingAgentDeliverySettlements(
	ctx context.Context,
	sessionID string,
) ([]*models.AgentDeliveryEffect, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		       session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome
		FROM agent_delivery_effects
		WHERE effect_type = ? AND state != ? AND (? = '' OR session_id = ?)
		ORDER BY created_at, stream_id, sequence`),
		models.DeliveryTerminalEffectType, models.DeliveryEffectCompleted, sessionID, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var effects []*models.AgentDeliveryEffect
	for rows.Next() {
		var effect models.AgentDeliveryEffect
		var completedAt sql.NullTime
		if err := rows.Scan(
			&effect.EffectKey, &effect.StreamID, &effect.Sequence, &effect.EffectType,
			&effect.State, &effect.CreatedAt, &completedAt, &effect.SessionID,
			&effect.IncarnationID, &effect.HarnessGeneration, &effect.SubmissionID,
			&effect.TurnID, &effect.Outcome); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			t := completedAt.Time
			effect.CompletedAt = &t
		}
		effects = append(effects, &effect)
	}
	return effects, rows.Err()
}

func (r *Repository) CompleteAgentDeliveryTerminalSettlement(
	ctx context.Context,
	effectKey string,
	completedAt time.Time,
) (bool, error) {
	if completedAt.IsZero() {
		completedAt = r.nowUTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_effects SET state = ?, completed_at = ?
		WHERE effect_key = ? AND effect_type = ? AND state = ?`),
		models.DeliveryEffectCompleted, completedAt, effectKey,
		models.DeliveryTerminalEffectType, models.DeliveryEffectBlockResolved)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 1 {
		return count == 1, err
	}
	effect, getErr := r.GetAgentDeliveryEffect(ctx, effectKey)
	if getErr != nil {
		return false, getErr
	}
	return effect.EffectType == models.DeliveryTerminalEffectType && effect.State == models.DeliveryEffectCompleted, nil
}

func (r *Repository) bindProjectedTerminalToRecoveryBlock(
	ctx context.Context,
	tx *sqlx.Tx,
	block *models.SessionRecoveryBlock,
) error {
	if block == nil || block.ConsumerReference != "agent_delivery" || block.DeliverySubmissionID == "" {
		return nil
	}
	var streamID, turnID, outcome string
	var sequence int64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT stream_id, sequence, turn_id, outcome FROM agent_delivery_effects
		WHERE effect_type = ? AND state IN (?, ?) AND session_id = ? AND incarnation_id = ?
		  AND harness_generation = ? AND submission_id = ?
		ORDER BY sequence DESC LIMIT 1`),
		models.DeliveryTerminalEffectType, models.DeliveryEffectBlockResolved,
		models.DeliveryEffectCompleted, block.SessionID, block.IncarnationID,
		block.ExpectedGeneration, block.DeliverySubmissionID).Scan(&streamID, &sequence, &turnID, &outcome)
	if err == sql.ErrNoRows || err == nil && block.DeliveryStreamID != "" && block.DeliveryStreamID != streamID {
		return nil
	}
	if err != nil {
		return err
	}
	block.State = models.RecoveryBlockResolved
	block.DeliveryStreamID = streamID
	block.DeliverySequence = sequence
	block.DeliveryTurnID = turnID
	block.DeliveryOutcome = outcome
	block.AuthorizedAction = "durable_delivery_" + outcome
	resolvedAt := time.Now().UTC()
	block.ResolvedAt = &resolvedAt
	return nil
}

func validDeliveryTerminalOutcome(outcome models.DeliverySubmissionState) bool {
	return outcome == models.DeliverySubmissionCompleted || outcome == models.DeliverySubmissionFailed ||
		outcome == models.DeliverySubmissionCancelled
}
