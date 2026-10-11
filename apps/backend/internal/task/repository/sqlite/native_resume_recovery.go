package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const nativeResumeRecoveryAction = "resume"

// HasResolvedNativeResumeRecovery binds the restored snapshot to the exact
// resolution committed with it. A generic resolved timestamp is insufficient.
func (r *Repository) HasResolvedNativeResumeRecovery(ctx context.Context, recovery models.AgentDeliveryRecovery) (bool, error) {
	var found bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (
		SELECT 1 FROM session_recovery_blocks WHERE session_id = ? AND incarnation_id = ?
		AND expected_generation = ? AND consumer_reference = ? AND state = ?
		AND authorized_action = ? AND delivery_submission_id = ? AND delivery_stream_id = ?
		AND resolved_at = ?)`), recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration,
		"agent_delivery", models.RecoveryBlockResolved, nativeResumeRecoveryAction, recovery.SubmissionID, recovery.StreamID,
		recovery.UpdatedAt).Scan(&found)
	return found, err
}

// Native resume's authenticated retirement check precedes this transaction.
// The matching notice and admission fence must become usable in one commit.
func (r *Repository) restoreNativeResumeMetadataTx(ctx context.Context, tx *sqlx.Tx, blockID string, completedAt time.Time) (bool, error) {
	query := `SELECT ts.task_id, ts.id, ts.queue_incarnation_id, ts.state, ts.metadata,
		b.incarnation_id, b.expected_generation, b.delivery_submission_id, b.delivery_stream_id, b.consumer_reference
		FROM task_sessions ts JOIN session_recovery_blocks b ON b.session_id = ts.id
		WHERE b.id = ? AND b.state = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE OF ts, b"
	}
	var taskID, sessionID, incarnationID, blockIncarnation, submissionID, streamID string
	var state models.TaskSessionState
	var generation int64
	var consumer string
	var metadataRaw sql.NullString
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), blockID, models.RecoveryBlockOpen).Scan(
		&taskID, &sessionID, &incarnationID, &state, &metadataRaw, &blockIncarnation, &generation, &submissionID, &streamID, &consumer)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	metadata, err := decodeAgentDeliveryMetadata(metadataRaw)
	if err != nil {
		return false, err
	}
	recovery, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil {
		return false, err
	}
	if !exists {
		return consumer != reconstructionDeliveryConsumer, nil
	}
	if recovery.SessionID != sessionID || recovery.IncarnationID != incarnationID || incarnationID != blockIncarnation ||
		recovery.HarnessGeneration != generation || recovery.SubmissionID != submissionID || recovery.StreamID != streamID ||
		(recovery.Phase != models.AgentDeliveryRecoveryUncertain && recovery.Phase != models.AgentDeliveryRecoveryReconnecting) {
		return false, nil
	}
	return writeSilentRestoreRecoveryTx(ctx, tx, r.db.Rebind, &silentRestoreSessionRow{
		taskID: taskID, state: state, metadata: metadata, current: recovery,
	}, metadataRaw, completedAt)
}
