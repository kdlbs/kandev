package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) RecordAgentDeliveryWorkflowOutcome(
	ctx context.Context,
	taskID string,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if !validAgentDeliveryWorkflowNoOp(taskID, effect) {
		return false, errors.New("exact completed workflow no-op identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskHierarchy(ctx, tx, nil, []string{taskID}); err != nil {
		return false, err
	}
	if err := r.lockTaskStepForWrite(ctx, tx, taskID); err != nil {
		return false, err
	}
	if err := r.requireWorkflowOutcomeSourceTx(ctx, tx, taskID, effect); err != nil {
		if errors.Is(err, errAgentDeliveryWorkflowSourceChanged) {
			return false, tx.Commit()
		}
		return false, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{effect.SessionID}, []string{effect.StreamID}); err != nil {
		return false, err
	}
	valid, err := r.agentDeliveryWorkflowTerminalMatchesTx(ctx, tx, effect)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, tx.Commit()
	}
	if _, err := insertDeliveryEffectTx(ctx, tx, r.db.Rebind, effect); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validAgentDeliveryWorkflowNoOp(taskID string, effect *models.AgentDeliveryEffect) bool {
	if !hasExactAgentDeliveryTerminalIdentity(effect) || taskID == "" || effect.TaskID != taskID ||
		effect.EffectKey == "" || effect.EffectType != "workflow.on_turn_complete" ||
		effect.State != models.DeliveryEffectCompleted || effect.CompletedAt == nil {
		return false
	}
	if effect.EffectKey != "workflow.on_turn_complete:"+effect.TurnID {
		return false
	}
	return effect.Outcome == "no_automatic_transition" || effect.Outcome == "not_required"
}

func hasExactAgentDeliveryTerminalIdentity(effect *models.AgentDeliveryEffect) bool {
	return effect != nil && effect.TurnID != "" && effect.SubmissionID != "" &&
		effect.StreamID != "" && effect.Sequence > 0 && effect.SessionID != "" &&
		effect.IncarnationID != "" && effect.HarnessGeneration > 0
}

var errAgentDeliveryWorkflowSourceChanged = errors.New("workflow source changed before delivery outcome receipt")

func (r *Repository) requireWorkflowOutcomeSourceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	effect *models.AgentDeliveryEffect,
) error {
	var workflowID, stepID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT workflow_id, workflow_step_id FROM tasks WHERE id = ?`), taskID).Scan(&workflowID, &stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return repoerrors.ErrTaskNotFound
	}
	if err != nil {
		return err
	}
	if workflowID != effect.WorkflowID || stepID != effect.WorkflowStepID {
		return errAgentDeliveryWorkflowSourceChanged
	}
	if stepID == "" {
		if effect.WorkflowPolicyUpdatedAt != nil {
			return errAgentDeliveryWorkflowSourceChanged
		}
		return nil
	}
	if effect.WorkflowPolicyUpdatedAt == nil {
		return errAgentDeliveryWorkflowSourceChanged
	}
	var updatedAt sql.NullTime
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT updated_at FROM workflow_steps WHERE id = ? AND workflow_id = ?`), stepID, workflowID).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errAgentDeliveryWorkflowSourceChanged
		}
		return err
	}
	if !updatedAt.Valid || !updatedAt.Time.Equal(*effect.WorkflowPolicyUpdatedAt) {
		return errAgentDeliveryWorkflowSourceChanged
	}
	return nil
}

func (r *Repository) workflowPolicyMatchesTx(
	ctx context.Context,
	tx interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workflowID, stepID string,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if effect.WorkflowStepID == "" {
		return effect.WorkflowPolicyUpdatedAt == nil, nil
	}
	if effect.WorkflowID != workflowID || effect.WorkflowStepID != stepID || effect.WorkflowPolicyUpdatedAt == nil {
		return false, nil
	}
	var updatedAt sql.NullTime
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT updated_at FROM workflow_steps WHERE id = ? AND workflow_id = ?`), stepID, workflowID).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return updatedAt.Valid && updatedAt.Time.Equal(*effect.WorkflowPolicyUpdatedAt), nil
}

func (r *Repository) agentDeliveryWorkflowTerminalMatchesTx(
	ctx context.Context,
	tx interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	var sessionTaskID, incarnationID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, queue_incarnation_id FROM task_sessions WHERE id = ?`), effect.SessionID).Scan(
		&sessionTaskID, &incarnationID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && incarnationID != effect.IncarnationID {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if sessionTaskID != effect.TaskID {
		return false, nil
	}
	var submissionState, submissionOutcome, terminalState, terminalOutcome string
	var submissionIncarnation string
	var submissionGeneration int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT s.state, s.outcome, s.incarnation_id, s.harness_generation,
		       t.state, t.outcome
		FROM agent_delivery_submissions s
		JOIN agent_delivery_effects t ON t.submission_id = s.id
		WHERE s.id = ? AND s.session_id = ? AND t.effect_key = ?
		  AND t.effect_type = ? AND t.stream_id = ? AND t.sequence = ?`),
		effect.SubmissionID, effect.SessionID, fmt.Sprintf("agent_delivery.terminal:%s:%d", effect.StreamID, effect.Sequence),
		models.DeliveryTerminalEffectType, effect.StreamID, effect.Sequence).Scan(
		&submissionState, &submissionOutcome, &submissionIncarnation, &submissionGeneration,
		&terminalState, &terminalOutcome)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !agentDeliveryWorkflowSubmissionMatches(effect, submissionIncarnation, submissionGeneration,
		submissionState, submissionOutcome, terminalState, terminalOutcome) {
		return false, nil
	}
	return r.agentDeliveryWorkflowCursorMatchesTx(ctx, tx, effect)
}

func agentDeliveryWorkflowSubmissionMatches(
	effect *models.AgentDeliveryEffect,
	incarnationID string,
	generation int64,
	submissionState, submissionOutcome, terminalState, terminalOutcome string,
) bool {
	if incarnationID != effect.IncarnationID || generation != effect.HarnessGeneration {
		return false
	}
	switch terminalState {
	case models.DeliveryEffectPending:
		return true
	case models.DeliveryEffectBlockResolved, models.DeliveryEffectCompleted:
		return submissionState == terminalOutcome && submissionOutcome == terminalOutcome
	default:
		return false
	}
}

func (r *Repository) agentDeliveryWorkflowCursorMatchesTx(
	ctx context.Context,
	tx interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	var streamSessionID, streamIncarnationID string
	var streamGeneration, projectedSequence, terminalSequence int64
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT c.session_id, c.incarnation_id, c.harness_generation, c.projected_sequence,
		       t.sequence
		FROM agent_delivery_effects t
		JOIN agent_delivery_cursors c ON c.stream_id = t.stream_id
		WHERE t.effect_key = ? AND t.turn_id = ? AND t.session_id = ?
		  AND t.incarnation_id = ? AND t.harness_generation = ? AND t.submission_id = ?`),
		fmt.Sprintf("agent_delivery.terminal:%s:%d", effect.StreamID, effect.Sequence), effect.TurnID,
		effect.SessionID, effect.IncarnationID, effect.HarnessGeneration, effect.SubmissionID).Scan(
		&streamSessionID, &streamIncarnationID, &streamGeneration, &projectedSequence, &terminalSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return streamSessionID == effect.SessionID && streamIncarnationID == effect.IncarnationID &&
		streamGeneration == effect.HarnessGeneration && projectedSequence >= terminalSequence, nil
}

func (r *Repository) BackfillLegacyAgentDeliveryWorkflowEffect(
	ctx context.Context,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if !validLegacyAgentDeliveryWorkflowIdentity(effect) {
		return false, errors.New("exact delivery workflow identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{effect.SessionID}, []string{effect.StreamID}); err != nil {
		return false, err
	}
	var terminalCount, exactTerminalCount int
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_effects
		WHERE effect_type = ? AND turn_id = ?`),
		models.DeliveryTerminalEffectType, effect.TurnID).Scan(&terminalCount); err != nil {
		return false, err
	}
	if terminalCount != 1 {
		return false, tx.Commit()
	}
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_effects
		WHERE effect_type = ? AND turn_id = ? AND session_id = ? AND incarnation_id = ?
		  AND harness_generation = ? AND submission_id = ? AND stream_id = ? AND sequence = ?`),
		models.DeliveryTerminalEffectType, effect.TurnID, effect.SessionID, effect.IncarnationID,
		effect.HarnessGeneration, effect.SubmissionID, effect.StreamID, effect.Sequence).Scan(&exactTerminalCount); err != nil {
		return false, err
	}
	if exactTerminalCount != 1 {
		return false, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_effects
		SET stream_id = ?, sequence = ?, session_id = ?, incarnation_id = ?, harness_generation = ?,
		    submission_id = ?, turn_id = ?, outcome = 'applied'
		WHERE effect_key = ? AND effect_type = 'workflow.on_turn_complete' AND state = ?
		  AND stream_id = '' AND sequence = 0 AND session_id = '' AND incarnation_id = ''
		  AND harness_generation = 0 AND submission_id = '' AND turn_id = '' AND outcome = ''`),
		effect.StreamID, effect.Sequence, effect.SessionID, effect.IncarnationID,
		effect.HarnessGeneration, effect.SubmissionID, effect.TurnID,
		effect.EffectKey, models.DeliveryEffectCompleted)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return rows == 1, nil
}

func validLegacyAgentDeliveryWorkflowIdentity(effect *models.AgentDeliveryEffect) bool {
	return effect != nil && effect.EffectKey != "" && effect.TurnID != "" && effect.SessionID != "" &&
		effect.StreamID != "" && effect.Sequence > 0 && effect.SubmissionID != ""
}
