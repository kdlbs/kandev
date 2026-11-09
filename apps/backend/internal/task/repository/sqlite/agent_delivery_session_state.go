package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// CorrectSettledAgentDeliverySessionState conditionally repairs a stale
// STARTING or RUNNING session from one exact settled terminal. The writer
// transaction re-reads every identity, receipt, projection, turn, successor,
// archive, and generation proof before it may write.
func (r *Repository) CorrectSettledAgentDeliverySessionState(
	ctx context.Context,
	request models.AgentDeliverySessionStateRequest,
) (*models.AgentDeliverySessionStateResult, error) {
	if !validDeliverySessionStateRequest(request) {
		return &models.AgentDeliverySessionStateResult{
			Outcome: models.AgentDeliverySessionStateUnavailable,
		}, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), request.SessionID); err != nil {
		return nil, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{request.SessionID}, []string{request.StreamID}); err != nil {
		return nil, err
	}
	result, err := r.correctSettledAgentDeliverySessionStateTx(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func validDeliverySessionStateRequest(request models.AgentDeliverySessionStateRequest) bool {
	return request.SessionID != "" && request.StreamID != "" && request.Sequence > 0 &&
		request.SubmissionID != "" && request.TurnID != "" && request.IncarnationID != "" &&
		request.HarnessGeneration > 0 &&
		validDeliveryTerminalOutcome(models.DeliverySubmissionState(request.Outcome))
}

type settledSessionStateEvidence struct {
	taskID       string
	executionID  string
	incarnation  string
	currentState models.TaskSessionState
}

func (r *Repository) correctSettledAgentDeliverySessionStateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request models.AgentDeliverySessionStateRequest,
) (*models.AgentDeliverySessionStateResult, error) {
	evidence, outcome, err := r.loadSettledSessionStateEvidenceTx(ctx, tx, request)
	if err != nil || outcome != "" {
		return deliverySessionStateResult(outcome), err
	}
	if outcome, err := r.verifySettledTerminalEvidenceTx(ctx, tx, request); err != nil || outcome != "" {
		return deliverySessionStateResult(outcome), err
	}
	now := r.nowUTC()
	updated, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET state = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ?
		  AND state IN (?, ?)
		  AND EXISTS (
			SELECT 1 FROM executors_running er
			WHERE er.session_id = task_sessions.id AND er.agent_execution_id = ?)`),
		models.TaskSessionStateWaitingForInput, now, request.SessionID, evidence.taskID,
		evidence.incarnation, models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		evidence.executionID)
	if err != nil {
		return nil, err
	}
	rows, err := updated.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return deliverySessionStateResult(models.AgentDeliverySessionStateSuperseded), nil
	}
	return &models.AgentDeliverySessionStateResult{
		Outcome:   models.AgentDeliverySessionStateChanged,
		OldState:  evidence.currentState,
		NewState:  models.TaskSessionStateWaitingForInput,
		UpdatedAt: now,
	}, nil
}

func deliverySessionStateResult(outcome models.AgentDeliverySessionStateOutcome) *models.AgentDeliverySessionStateResult {
	return &models.AgentDeliverySessionStateResult{Outcome: outcome}
}

func (r *Repository) loadSettledSessionStateEvidenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request models.AgentDeliverySessionStateRequest,
) (*settledSessionStateEvidence, models.AgentDeliverySessionStateOutcome, error) {
	var evidence settledSessionStateEvidence
	var archivedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT ts.task_id, COALESCE(er.agent_execution_id, ''), ts.queue_incarnation_id,
		       ts.state, t.archived_at
		FROM task_sessions ts
		LEFT JOIN executors_running er ON er.session_id = ts.id
		JOIN tasks t ON t.id = ts.task_id
		WHERE ts.id = ?`), request.SessionID).Scan(
		&evidence.taskID, &evidence.executionID, &evidence.incarnation,
		&evidence.currentState, &archivedAt)
	if err == sql.ErrNoRows {
		return nil, models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return nil, "", err
	}
	if archivedAt.Valid {
		return nil, models.AgentDeliverySessionStateSuperseded, nil
	}
	if request.TaskID != "" && request.TaskID != evidence.taskID ||
		request.ExecutionID != "" && request.ExecutionID != evidence.executionID ||
		request.IncarnationID != evidence.incarnation || evidence.executionID == "" {
		return nil, models.AgentDeliverySessionStateSuperseded, nil
	}
	switch evidence.currentState {
	case models.TaskSessionStateWaitingForInput:
		return nil, models.AgentDeliverySessionStateAlreadyConsistent, nil
	case models.TaskSessionStateStarting, models.TaskSessionStateRunning:
	default:
		return nil, models.AgentDeliverySessionStateSuperseded, nil
	}
	return &evidence, "", nil
}

func (r *Repository) verifySettledTerminalEvidenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	currentGeneration, err := currentHarnessGenerationTx(ctx, tx, r.db, request)
	if err != nil {
		return "", err
	}
	if currentGeneration != request.HarnessGeneration {
		return models.AgentDeliverySessionStateSuperseded, nil
	}
	verifiers := []func(context.Context, *sqlx.Tx, *sqlx.DB, models.AgentDeliverySessionStateRequest) (models.AgentDeliverySessionStateOutcome, error){
		verifyTerminalEffectTx,
		verifyTerminalInboxTx,
		verifyTerminalProjectionTx,
		verifyTerminalSubmissionTx,
		verifySettledTurnTx,
		verifyNoOpenSessionTurnsTx,
	}
	for _, verify := range verifiers {
		outcome, err := verify(ctx, tx, r.db, request)
		if err != nil || outcome != "" {
			return outcome, err
		}
	}
	successors, err := countCurrentOwnerSuccessorsTx(ctx, tx, r.db, request)
	if err != nil {
		return "", err
	}
	if successors > 0 {
		return models.AgentDeliverySessionStateSuperseded, nil
	}
	return "", nil
}

func currentHarnessGenerationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (int64, error) {
	var generation int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT COALESCE(MAX(generation), 0)
		FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ?`),
		request.SessionID, request.IncarnationID).Scan(&generation)
	return generation, err
}

func verifyTerminalEffectTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	effectKey := fmt.Sprintf("agent_delivery.terminal:%s:%d", request.StreamID, request.Sequence)
	var state, outcome string
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT state, outcome FROM agent_delivery_effects
		WHERE effect_key = ? AND stream_id = ? AND sequence = ? AND effect_type = ?
		  AND session_id = ? AND incarnation_id = ? AND harness_generation = ?
		  AND submission_id = ? AND turn_id = ?`),
		effectKey, request.StreamID, request.Sequence, models.DeliveryTerminalEffectType,
		request.SessionID, request.IncarnationID, request.HarnessGeneration,
		request.SubmissionID, request.TurnID).Scan(&state, &outcome)
	if err == sql.ErrNoRows {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if state != models.DeliveryEffectBlockResolved && state != models.DeliveryEffectCompleted {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if outcome != request.Outcome {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	return "", nil
}

func verifyTerminalInboxTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	var eventType string
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT event_type FROM agent_delivery_inbox
		WHERE stream_id = ? AND sequence = ? AND session_id = ? AND incarnation_id = ?
		  AND harness_generation = ? AND submission_id = ?`),
		request.StreamID, request.Sequence, request.SessionID, request.IncarnationID,
		request.HarnessGeneration, request.SubmissionID).Scan(&eventType)
	if err == sql.ErrNoRows {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if !terminalDeliveryEventType(eventType) {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	return "", nil
}

func terminalDeliveryEventType(eventType string) bool {
	switch eventType {
	case deliveryCompleteEventType, deliveryErrorEventType, string(models.DeliverySubmissionCancelled), deliveryCanceledEventType:
		return true
	default:
		return false
	}
}

func verifyTerminalProjectionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	var projected int64
	var sessionID, incarnationID string
	var generation int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT projected_sequence, session_id, incarnation_id, harness_generation
		FROM agent_delivery_cursors WHERE stream_id = ?`), request.StreamID).Scan(
		&projected, &sessionID, &incarnationID, &generation)
	if err == sql.ErrNoRows {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if sessionID != request.SessionID || incarnationID != request.IncarnationID ||
		generation != request.HarnessGeneration || projected < request.Sequence {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	return "", nil
}

func verifyTerminalSubmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	var sessionID, incarnationID, state, outcome string
	var generation int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, state, outcome
		FROM agent_delivery_submissions WHERE id = ?`), request.SubmissionID).Scan(
		&sessionID, &incarnationID, &generation, &state, &outcome)
	if err == sql.ErrNoRows {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if sessionID != request.SessionID || incarnationID != request.IncarnationID ||
		generation != request.HarnessGeneration || !validDeliveryTerminalOutcome(models.DeliverySubmissionState(state)) ||
		outcome != request.Outcome {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	return "", nil
}

func verifySettledTurnTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	var sessionID, taskID string
	var completedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT task_session_id, task_id, completed_at
		FROM task_session_turns WHERE id = ?`), request.TurnID).Scan(
		&sessionID, &taskID, &completedAt)
	if err == sql.ErrNoRows {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if sessionID != request.SessionID || request.TaskID != "" && request.TaskID != taskID || !completedAt.Valid {
		return models.AgentDeliverySessionStateUnavailable, nil
	}
	return "", nil
}

func verifyNoOpenSessionTurnsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (models.AgentDeliverySessionStateOutcome, error) {
	var open int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT COUNT(*) FROM task_session_turns
		WHERE task_session_id = ? AND completed_at IS NULL`), request.SessionID).Scan(&open)
	if err != nil {
		return "", err
	}
	if open > 0 {
		return models.AgentDeliverySessionStateSuperseded, nil
	}
	return "", nil
}

func countCurrentOwnerSuccessorsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request models.AgentDeliverySessionStateRequest,
) (int64, error) {
	var successors int64
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_submissions
		WHERE session_id = ? AND id <> ? AND state IN (?, ?, ?)`),
		request.SessionID, request.SubmissionID,
		models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted,
		models.DeliverySubmissionDispatching).Scan(&successors)
	return successors, err
}

// LoadCurrentSettledAgentDeliveryTerminal returns the current persisted
// prompt/turn binding after its terminal settlement completed. It reads the
// submission's recorded terminal identity instead of scanning event payloads.
func (r *Repository) LoadCurrentSettledAgentDeliveryTerminal(
	ctx context.Context,
	sessionID string,
) (*models.AgentDeliverySessionStateRequest, bool, error) {
	if sessionID == "" {
		return nil, false, nil
	}
	var request models.AgentDeliverySessionStateRequest
	var outcome string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT submission.id, submission.session_id, submission.incarnation_id,
		       submission.harness_generation, submission.terminal_stream_id,
		       submission.terminal_sequence, submission.terminal_turn_id, submission.outcome,
		       ts.task_id, COALESCE(er.agent_execution_id, '')
		FROM agent_delivery_submissions submission
		JOIN task_sessions ts ON ts.id = submission.session_id
		LEFT JOIN executors_running er ON er.session_id = ts.id
		WHERE submission.session_id = ? AND submission.fully_settled_at IS NOT NULL
		  AND submission.terminal_stream_id <> '' AND submission.terminal_sequence > 0
		  AND submission.terminal_turn_id <> ''
		  AND ts.state IN (?, ?)
		ORDER BY submission.fully_settled_at DESC, submission.created_at DESC
		LIMIT 1`), sessionID,
		models.TaskSessionStateStarting, models.TaskSessionStateRunning).Scan(
		&request.SubmissionID, &request.SessionID, &request.IncarnationID,
		&request.HarnessGeneration, &request.StreamID, &request.Sequence,
		&request.TurnID, &outcome, &request.TaskID, &request.ExecutionID)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !validDeliveryTerminalOutcome(models.DeliverySubmissionState(outcome)) {
		return nil, false, nil
	}
	request.Outcome = outcome
	return &request, true, nil
}

// ListSettledAgentDeliverySessionStateCandidates returns a bounded set of
// completed terminal bindings whose session row still looks active.
func (r *Repository) ListSettledAgentDeliverySessionStateCandidates(
	ctx context.Context,
	limit int,
) ([]*models.AgentDeliverySessionStateRequest, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT submission.id, submission.session_id, submission.incarnation_id,
		       submission.harness_generation, submission.terminal_stream_id,
		       submission.terminal_sequence, submission.terminal_turn_id, submission.outcome,
		       ts.task_id, COALESCE(er.agent_execution_id, '')
		FROM agent_delivery_submissions submission
		JOIN task_sessions ts ON ts.id = submission.session_id
		LEFT JOIN executors_running er ON er.session_id = ts.id
		WHERE submission.fully_settled_at IS NOT NULL
		  AND submission.terminal_stream_id <> '' AND submission.terminal_sequence > 0
		  AND submission.terminal_turn_id <> ''
		  AND ts.state IN (?, ?)
		ORDER BY submission.fully_settled_at DESC, submission.created_at DESC
		LIMIT ?`), models.TaskSessionStateStarting, models.TaskSessionStateRunning, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var candidates []*models.AgentDeliverySessionStateRequest
	for rows.Next() {
		var request models.AgentDeliverySessionStateRequest
		var outcome string
		if err := rows.Scan(
			&request.SubmissionID, &request.SessionID, &request.IncarnationID,
			&request.HarnessGeneration, &request.StreamID, &request.Sequence,
			&request.TurnID, &outcome, &request.TaskID, &request.ExecutionID); err != nil {
			return nil, err
		}
		if !validDeliveryTerminalOutcome(models.DeliverySubmissionState(outcome)) {
			continue
		}
		request.Outcome = outcome
		candidates = append(candidates, &request)
	}
	return candidates, rows.Err()
}
