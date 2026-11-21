package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const lockInferenceClosureSessionRow = " FOR UPDATE OF ts"

// GetInferenceClosureBlock reads the writer connection: a generation change
// cannot hide an unfinished closure from admission.
func (r *Repository) GetInferenceClosureBlock(ctx context.Context, sessionID string) (*models.SessionRecoveryBlock, error) {
	var b models.SessionRecoveryBlock
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
  SELECT id, session_id, incarnation_id, reason, state, consumer_reference
  FROM session_recovery_blocks WHERE session_id = ? AND consumer_reference = ? AND state = ?
  ORDER BY created_at LIMIT 1`), sessionID, models.InferenceClosureConsumer, models.RecoveryBlockOpen).
		Scan(&b.ID, &b.SessionID, &b.IncarnationID, &b.Reason, &b.State, &b.ConsumerReference)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

// BeginInferenceClosure atomically persists the checkpoint and its admission block.
func (r *Repository) BeginInferenceClosure(ctx context.Context, sessionID, operationID, incarnationID, executionID string) (json.RawMessage, error) {
	next := models.InferenceClosureCheckpoint{Version: 1, OperationID: operationID, SessionID: sessionID,
		IncarnationID: incarnationID, SourceExecutionID: executionID, CandidateExecutionID: uuid.NewString(), Stage: models.InferenceClosureFenced}
	if !next.Valid() {
		return nil, models.ErrInferenceClosureConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	currentExecution, currentIncarnation, metadata, err := r.lockInferenceClosureSession(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	priorRaw := metadata[models.InferenceClosureMetadataKey]
	replay, err := existingInferenceClosure(priorRaw, operationID, incarnationID, executionID)
	if err != nil {
		return nil, err
	}
	if replay {
		return priorRaw, tx.Commit()
	}
	if currentExecution != executionID || currentIncarnation != incarnationID {
		return nil, models.ErrInferenceClosureConflict
	}
	var open int
	if err := tx.GetContext(ctx, &open, r.db.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = ? AND consumer_reference = ? AND state = ?`), sessionID, models.InferenceClosureConsumer, models.RecoveryBlockOpen); err != nil {
		return nil, err
	}
	if open != 0 {
		return nil, models.ErrInferenceClosureConflict
	}
	block := &models.SessionRecoveryBlock{ID: operationID, SessionID: sessionID, IncarnationID: incarnationID,
		Reason: models.InferenceClosureConsumer + ":" + operationID, State: models.RecoveryBlockOpen, ConsumerReference: models.InferenceClosureConsumer}
	if err := r.upsertSessionRecoveryBlockTx(ctx, tx, block); err != nil {
		return nil, err
	}
	raw, err := r.saveInferenceClosureTx(ctx, tx, metadata, next)
	if err != nil {
		return nil, err
	}
	return raw, tx.Commit()
}

func (r *Repository) ReadInferenceClosure(ctx context.Context, sessionID string) (json.RawMessage, error) {
	var metadataJSON sql.NullString
	if err := r.db.QueryRowContext(ctx, r.db.Rebind("SELECT metadata FROM task_sessions WHERE id = ?"), sessionID).Scan(&metadataJSON); err != nil {
		return nil, err
	}
	metadata, err := inferenceClosureMetadata(metadataJSON)
	if err != nil {
		return nil, err
	}
	raw := metadata[models.InferenceClosureMetadataKey]
	var checkpoint models.InferenceClosureCheckpoint
	if len(raw) == 0 {
		return nil, sql.ErrNoRows
	}
	if json.Unmarshal(raw, &checkpoint) != nil || !checkpoint.Valid() || checkpoint.SessionID != sessionID {
		return nil, models.ErrInferenceClosureConflict
	}
	return raw, nil
}

// AdvanceInferenceClosure keeps the fence until the exact allocated successor
// has actually consumed the journal receipt. SQLite and journal commits replay separately.
func (r *Repository) AdvanceInferenceClosure(ctx context.Context, sessionID, operationID string, expected, nextRaw json.RawMessage) (bool, error) {
	prior, next, err := decodeInferenceClosureTransition(sessionID, operationID, expected, nextRaw)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	execution, incarnation, metadata, err := r.lockInferenceClosureSession(ctx, tx, sessionID)
	if err != nil {
		return false, err
	}
	if incarnation != prior.IncarnationID || !sameClosureJSON(metadata[models.InferenceClosureMetadataKey], expected) {
		return false, nil
	}
	wantedExecution := prior.SourceExecutionID
	if next.Stage == models.InferenceClosureConsumed {
		wantedExecution = prior.CandidateExecutionID
	}
	if execution != wantedExecution && (next.Stage == models.InferenceClosureConsumed || execution != "") {
		return false, models.ErrInferenceClosureConflict
	}
	var count int
	if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks WHERE id = ? AND session_id = ? AND consumer_reference = ? AND state = ?`), operationID, sessionID, models.InferenceClosureConsumer, models.RecoveryBlockOpen); err != nil {
		return false, err
	}
	if count != 1 {
		return false, models.ErrInferenceClosureConflict
	}
	if _, err := r.saveInferenceClosureTx(ctx, tx, metadata, *next); err != nil {
		return false, err
	}
	if next.Stage == models.InferenceClosureConsumed {
		now := r.nowUTC()
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks SET state = ?, authorized_action = ?, resolved_at = ?, updated_at = ? WHERE id = ? AND state = ?`),
			models.RecoveryBlockResolved, "inference_closure_adopted", now, now, operationID, models.RecoveryBlockOpen); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (r *Repository) lockInferenceClosureSession(ctx context.Context, tx *sqlx.Tx, sessionID string) (string, string, map[string]json.RawMessage, error) {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return "", "", nil, err
	}
	query := "SELECT COALESCE(er.agent_execution_id, ''), ts.queue_incarnation_id, ts.metadata FROM task_sessions ts LEFT JOIN executors_running er ON er.session_id = ts.id WHERE ts.id = ?"
	if dialect.IsPostgres(r.db.DriverName()) {
		query += lockInferenceClosureSessionRow
	}
	var execution, incarnation string
	var metadataJSON sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), sessionID).Scan(&execution, &incarnation, &metadataJSON); err != nil {
		return "", "", nil, err
	}
	metadata, err := inferenceClosureMetadata(metadataJSON)
	return execution, incarnation, metadata, err
}

func inferenceClosureMetadata(value sql.NullString) (map[string]json.RawMessage, error) {
	metadata := map[string]json.RawMessage{}
	if value.Valid && value.String != "" && value.String != jsonNull {
		if err := json.Unmarshal([]byte(value.String), &metadata); err != nil {
			return nil, fmt.Errorf("decode inference closure metadata: %w", err)
		}
	}
	return metadata, nil
}

func (r *Repository) saveInferenceClosureTx(ctx context.Context, tx *sqlx.Tx, metadata map[string]json.RawMessage, checkpoint models.InferenceClosureCheckpoint) (json.RawMessage, error) {
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, err
	}
	metadata[models.InferenceClosureMetadataKey] = raw
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	if err := r.updateSessionMetadataJSON(ctx, tx, checkpoint.SessionID, string(encoded), r.nowUTC()); err != nil {
		return nil, err
	}
	return raw, nil
}

func sameClosureJSON(a, b []byte) bool {
	var first, second any
	if json.Unmarshal(a, &first) != nil || json.Unmarshal(b, &second) != nil {
		return false
	}
	left, _ := json.Marshal(first)
	right, _ := json.Marshal(second)
	return bytes.Equal(left, right)
}

func existingInferenceClosure(raw json.RawMessage, operationID, incarnationID, executionID string) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	var saved models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &saved) != nil {
		return false, models.ErrInferenceClosureConflict
	}
	if saved.OperationID != operationID {
		return false, nil
	}
	if !saved.Valid() || saved.IncarnationID != incarnationID || saved.SourceExecutionID != executionID {
		return false, models.ErrInferenceClosureConflict
	}
	return true, nil
}

func decodeInferenceClosureTransition(sid, operation string, expected, nextRaw json.RawMessage) (*models.InferenceClosureCheckpoint, *models.InferenceClosureCheckpoint, error) {
	var prior, next models.InferenceClosureCheckpoint
	if json.Unmarshal(expected, &prior) != nil || json.Unmarshal(nextRaw, &next) != nil || !prior.CanAdvance(next) || prior.SessionID != sid || prior.OperationID != operation {
		return nil, nil, models.ErrInferenceClosureConflict
	}
	return &prior, &next, nil
}
