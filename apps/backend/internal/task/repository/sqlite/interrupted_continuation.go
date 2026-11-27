package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// CommitInterruptedContinuation resolves only the observed delivery fence after
// native restore. The interrupted submission and its queue ownership remain intact.
func (r *Repository) CommitInterruptedContinuation(ctx context.Context, commit *models.InterruptedContinuationCommit) (bool, error) {
	if !validInterruptedCommit(commit) {
		return false, errors.New("interrupted continuation identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockSessionTurnWrites(ctx, tx, r.db.DriverName(), commit.Recovery.SessionID); err != nil {
		return false, err
	}
	metadata, previous, valid, err := r.interruptedContinuationOwner(ctx, tx, commit)
	if err != nil || !valid {
		return false, err
	}
	if err = insertGenerationTx(ctx, tx.Tx, r.db.Rebind, &commit.Generation); err != nil {
		return false, err
	}
	changed, err := r.updateInterruptedContinuationMetadata(ctx, tx, commit, metadata, previous)
	if err != nil || !changed {
		return false, err
	}
	changed, err = r.completeInterruptedContinuationFences(ctx, tx, commit)
	if err != nil || !changed {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) updateInterruptedContinuationMetadata(ctx context.Context, tx *sqlx.Tx, commit *models.InterruptedContinuationCommit, metadata map[string]json.RawMessage, previous sql.NullString) (bool, error) {
	recovery := commit.Recovery
	recovery.Phase = models.AgentDeliveryRecoveryContinued
	recovery.Revision++
	recovery.UpdatedAt = r.nowUTC()
	encoded, err := json.Marshal(recovery)
	if err != nil {
		return false, err
	}
	metadata[models.SessionMetaKeyAgentDeliveryRecovery] = encoded
	next, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET metadata = ?, updated_at = ? WHERE id = ? AND queue_incarnation_id = ? AND COALESCE(metadata, '') = ?`), string(next), recovery.UpdatedAt, recovery.SessionID, recovery.IncarnationID, nullableMetadataValue(previous))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	return true, nil
}

func (r *Repository) completeInterruptedContinuationFences(ctx context.Context, tx *sqlx.Tx, commit *models.InterruptedContinuationCommit) (bool, error) {
	recovery := commit.Recovery
	recovery.UpdatedAt = r.nowUTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks SET state = ?, authorized_action = ?, updated_at = ?, resolved_at = ? WHERE id = ? AND state = ? AND consumer_reference = 'agent_delivery' AND delivery_submission_id = ? AND delivery_stream_id = ? AND expected_generation = ?`), models.RecoveryBlockResolved, "resume_interrupted", recovery.UpdatedAt, recovery.UpdatedAt, commit.BlockID, models.RecoveryBlockOpen, recovery.SubmissionID, recovery.StreamID, recovery.HarnessGeneration)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	result, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_continuation_snapshots SET status = ?, resolved_at = ? WHERE id = ? AND session_id = ? AND status = ? AND content_hash = ?`), "restored", recovery.UpdatedAt, commit.SnapshotID, recovery.SessionID, models.ContinuitySnapshotPrepared, commit.ContentHash)
	if err != nil {
		return false, err
	}
	rows, err = result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	return true, nil
}

func (r *Repository) interruptedContinuationOwner(ctx context.Context, tx *sqlx.Tx, commit *models.InterruptedContinuationCommit) (map[string]json.RawMessage, sql.NullString, bool, error) {
	recovery := commit.Recovery
	gen := commit.Generation
	if !interruptedGenerationMatches(recovery, gen) {
		return nil, sql.NullString{}, false, nil
	}
	var raw sql.NullString
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT ts.metadata FROM task_sessions ts JOIN tasks t ON t.id = ts.task_id WHERE ts.id = ? AND ts.queue_incarnation_id = ? AND ts.state != ? AND t.archived_at IS NULL AND EXISTS (SELECT 1 FROM executors_running er WHERE er.session_id = ts.id AND er.agent_execution_id = ?)`), recovery.SessionID, recovery.IncarnationID, models.TaskSessionStateCompleted, commit.CandidateExecutionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, raw, false, nil
	}
	if err != nil {
		return nil, raw, false, err
	}
	metadata, err := decodeAgentDeliveryMetadata(raw)
	if err != nil {
		return nil, raw, false, err
	}
	current, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil {
		return nil, raw, false, err
	}
	if !exists || !reflect.DeepEqual(current, recovery) || (current.Phase != models.AgentDeliveryRecoveryUncertain && current.Phase != models.AgentDeliveryRecoveryReconnecting) {
		return nil, raw, false, nil
	}
	var generation int64
	var nativeID string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT generation, native_session_id FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ? ORDER BY generation DESC LIMIT 1`), recovery.SessionID, recovery.IncarnationID).Scan(&generation, &nativeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, raw, false, nil
	}
	if err != nil {
		return nil, raw, false, err
	}
	if generation != recovery.HarnessGeneration || nativeID != gen.NativeSessionID {
		return nil, raw, false, nil
	}
	valid, err := r.interruptedContinuationFences(ctx, tx, commit)
	return metadata, raw, valid, err
}

func (r *Repository) interruptedContinuationFences(ctx context.Context, tx *sqlx.Tx, commit *models.InterruptedContinuationCommit) (bool, error) {
	recovery := commit.Recovery
	var matching, others int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = ? AND incarnation_id = ? AND state = ? AND id != ?`), recovery.SessionID, recovery.IncarnationID, models.RecoveryBlockOpen, commit.BlockID).Scan(&others)
	if err != nil || others != 0 {
		return false, err
	}
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ? AND state = ? AND consumer_reference = 'agent_delivery' AND delivery_submission_id = ? AND delivery_stream_id = ?`), commit.BlockID, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration, models.RecoveryBlockOpen, recovery.SubmissionID, recovery.StreamID).Scan(&matching)
	if err != nil || matching != 1 {
		return false, err
	}
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM session_continuation_snapshots WHERE id = ? AND session_id = ? AND target_generation = ? AND content_hash = ? AND status = ? AND submission_id != ''`), commit.SnapshotID, recovery.SessionID, commit.Generation.Generation, commit.ContentHash, models.ContinuitySnapshotPrepared).Scan(&matching)
	if err != nil || matching != 1 {
		return false, err
	}
	var state models.DeliverySubmissionState
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT state FROM agent_delivery_submissions WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?`), recovery.SubmissionID, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read interrupted submission: %w", err)
	}
	return !isTerminalAgentDeliverySubmission(state), nil
}

func interruptedGenerationMatches(recovery models.AgentDeliveryRecovery, gen models.HarnessSessionGeneration) bool {
	return gen.SessionID == recovery.SessionID && gen.IncarnationID == recovery.IncarnationID && gen.Generation == recovery.HarnessGeneration+1 && gen.PredecessorGeneration == recovery.HarnessGeneration && gen.NativeSessionID != ""
}

func validInterruptedCommit(commit *models.InterruptedContinuationCommit) bool {
	return commit != nil && commit.CandidateExecutionID != "" && commit.BlockID != "" && commit.SnapshotID != "" && commit.ContentHash != ""
}
