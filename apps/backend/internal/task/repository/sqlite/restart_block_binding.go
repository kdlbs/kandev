package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

// RepairRestartRecoveryBlock fills an observed partial fence after the caller
// has authenticated journal ownership and proved the historical owner dead.
func (r *Repository) RepairRestartRecoveryBlock(ctx context.Context, taskID string, recovery models.AgentDeliveryRecovery, block models.SessionRecoveryBlock) (bool, error) {
	if !restartBlockBindingRepairMatches(recovery, block) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), recovery.SessionID); err != nil {
		return false, err
	}
	_, _, valid, err := readSilentRestoreSessionTx(ctx, tx, r.db.DriverName(), models.SilentRestoreCheckpoint{TaskID: taskID, SourceRecovery: recovery})
	if err != nil || !valid {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks
 SET delivery_submission_id = ?, delivery_stream_id = ?, updated_at = ?
 WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
 AND state = ? AND consumer_reference = ? AND reason = ? AND updated_at = ?
 AND delivery_submission_id = ? AND delivery_stream_id = ?
 AND expected_generation = (SELECT MAX(generation) FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?)
 AND 1 = (SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ? AND state = ?)`),
		recovery.SubmissionID, recovery.StreamID, time.Now().UTC(), block.ID, block.SessionID, block.IncarnationID, block.ExpectedGeneration,
		block.State, block.ConsumerReference, block.Reason, block.UpdatedAt, block.DeliverySubmissionID, block.DeliveryStreamID,
		recovery.SessionID, recovery.IncarnationID, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration, models.RecoveryBlockOpen)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	return true, tx.Commit()
}

func restartBlockBindingRepairMatches(recovery models.AgentDeliveryRecovery, block models.SessionRecoveryBlock) bool {
	return recovery.SubmissionID != "" && recovery.StreamID != "" &&
		block.ConsumerReference == reconstructionDeliveryConsumer && block.State == models.RecoveryBlockOpen &&
		block.SessionID == recovery.SessionID && block.IncarnationID == recovery.IncarnationID && block.ExpectedGeneration == recovery.HarnessGeneration &&
		(block.DeliverySubmissionID == "" || block.DeliverySubmissionID == recovery.SubmissionID) &&
		(block.DeliveryStreamID == "" || block.DeliveryStreamID == recovery.StreamID)
}
