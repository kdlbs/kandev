package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) finishPreparedNativeAttemptTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence, blocks []models.SessionRecoveryBlock, completedAt time.Time) (bool, error) {
	var ids []string
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(`SELECT id FROM session_restore_attempts WHERE session_id = ? AND completed_at IS NULL`), proof.SessionID); err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return true, nil
	}
	if len(ids) != 1 {
		return false, nil
	}
	attempt, encoded, err := readSilentRestoreAttemptCheckpointTx(ctx, tx, r.db.DriverName(), ids[0])
	if err != nil {
		return false, err
	}
	if !preparedNativeAttemptMatches(attempt, proof, blocks) {
		return false, nil
	}
	checkpoint := attempt.Checkpoint
	var ownerMatches bool
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM task_sessions s JOIN tasks t ON t.id = s.task_id JOIN workspaces w ON w.id = t.workspace_id WHERE s.id = ? AND t.id = ? AND w.id = ? AND w.owner_id = ? AND w.org_id = ?)`), proof.SessionID, checkpoint.TaskID, checkpoint.WorkspaceID, checkpoint.WorkspaceOwnerID, checkpoint.WorkspaceOrgID).Scan(&ownerMatches)
	if err != nil || !ownerMatches {
		return false, err
	}
	return completeSilentRestoreAttemptTx(ctx, tx, r.db.Rebind, attempt.ID, attempt, encoded, *checkpoint, completedAt)
}

func preparedNativeAttemptMatches(attempt *models.RestoreAttempt, proof *models.NativeResumeDeliveryEvidence, blocks []models.SessionRecoveryBlock) bool {
	if attempt == nil || attempt.Checkpoint == nil || !attempt.Authorized || attempt.Action != models.SilentRestoreAction || attempt.Outcome != models.SilentRestoreOutcomePending || attempt.SessionID != proof.SessionID || attempt.IncarnationID != proof.IncarnationID || attempt.ExpectedGeneration != proof.HarnessGeneration {
		return false
	}
	checkpoint := attempt.Checkpoint
	if !preparedNativeCheckpointMatches(checkpoint, proof) {
		return false
	}
	for _, block := range blocks {
		if block.ID == checkpoint.SourceBlock.ID {
			return true
		}
	}
	return false
}

func preparedNativeCheckpointMatches(checkpoint *models.SilentRestoreCheckpoint, proof *models.NativeResumeDeliveryEvidence) bool {
	source := checkpoint.SourceGeneration
	return checkpoint.Version == 1 && checkpoint.Stage == models.SilentRestoreStagePrepared && checkpoint.CandidateExecutionID == "" &&
		source.SessionID == proof.SessionID && source.IncarnationID == proof.IncarnationID && source.Generation == proof.HarnessGeneration && source.NativeSessionID == proof.NativeSessionID &&
		checkpoint.SourceRecovery.SessionID == proof.SessionID && checkpoint.SourceRecovery.IncarnationID == proof.IncarnationID && checkpoint.SourceRecovery.HarnessGeneration == proof.HarnessGeneration &&
		checkpoint.SourceRecovery.AgentExecutionID != "" && checkpoint.SourceRecovery.AgentExecutionID != proof.ExecutionID && nativeResumeProofCoversNotice(proof, checkpoint.SourceRecovery)
}
