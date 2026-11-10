package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Only an owned prepared historical attempt with exact fence or completed
// canonical continuation authority may retire during authenticated recovery.
func (r *Repository) preparedContextContinuationAttemptMatchesTx(ctx context.Context, tx *sqlx.Tx, attempt *models.RestoreAttempt, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	if !preparedContextContinuationCheckpointMatches(attempt, proof) {
		return false, nil
	}
	checkpoint := attempt.Checkpoint
	valid, err := r.hasResolvedContextContinuationNoticeTx(ctx, tx.Tx, checkpoint.SourceRecovery, proof)
	if err != nil || !valid {
		return false, err
	}
	completed, err := r.hasCompletedContextAncestryTx(ctx, tx.Tx, checkpoint.SourceRecovery, proof.HarnessGeneration)
	if err != nil {
		return false, err
	}
	var owned bool
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks b JOIN harness_session_generations source
 ON source.session_id = b.session_id AND source.incarnation_id = b.incarnation_id
 WHERE b.id = ? AND b.session_id = ? AND b.incarnation_id = ? AND b.expected_generation = ?
 AND b.state = 'resolved' AND b.authorized_action = 'continue_from_history'
 AND b.reason = 'unknown_prompt_outcome' AND b.consumer_reference = 'agent_delivery'
 AND b.delivery_submission_id = ? AND b.delivery_stream_id = ?
 AND source.generation = ? AND source.native_session_id = ?
 )`), checkpoint.SourceBlock.ID, proof.SessionID, proof.IncarnationID, proof.HarnessGeneration,
		checkpoint.SourceRecovery.SubmissionID, checkpoint.SourceRecovery.StreamID,
		checkpoint.SourceGeneration.Generation, checkpoint.SourceGeneration.NativeSessionID).Scan(&owned)
	if err != nil || owned || !completed {
		return owned, err
	}
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM harness_session_generations
 WHERE session_id = ? AND incarnation_id = ? AND generation = ? AND native_session_id = ?)`),
		proof.SessionID, proof.IncarnationID, checkpoint.SourceGeneration.Generation, checkpoint.SourceGeneration.NativeSessionID).Scan(&owned)
	return owned, err
}

func preparedContextContinuationCheckpointMatches(attempt *models.RestoreAttempt, proof *models.NativeResumeDeliveryEvidence) bool {
	if attempt == nil || attempt.Checkpoint == nil || !attempt.Authorized || attempt.Action != models.SilentRestoreAction ||
		attempt.Outcome != models.SilentRestoreOutcomePending || attempt.SessionID != proof.SessionID ||
		attempt.IncarnationID != proof.IncarnationID || attempt.ExpectedGeneration < 1 || attempt.ExpectedGeneration >= proof.HarnessGeneration {
		return false
	}
	checkpoint := attempt.Checkpoint
	return checkpoint.Version == 1 && checkpoint.Stage == models.SilentRestoreStagePrepared && checkpoint.CandidateExecutionID == "" &&
		checkpoint.SourceGeneration.Generation == attempt.ExpectedGeneration &&
		contextContinuationCheckpointSourceMatches(checkpoint, proof) && contextContinuationCheckpointBlockMatches(checkpoint)
}

func contextContinuationCheckpointSourceMatches(checkpoint *models.SilentRestoreCheckpoint, proof *models.NativeResumeDeliveryEvidence) bool {
	source, recovery := checkpoint.SourceGeneration, checkpoint.SourceRecovery
	return source.SessionID == proof.SessionID && source.IncarnationID == proof.IncarnationID && source.Generation > 0 && source.Generation < proof.HarnessGeneration && source.NativeSessionID != "" &&
		contextContinuationNoticeMatches(recovery, proof.SessionID, proof.IncarnationID, source.Generation) &&
		recovery.AgentExecutionID != "" && recovery.AgentExecutionID != proof.ExecutionID && recovery.PromptGeneration > 0
}

func contextContinuationCheckpointBlockMatches(checkpoint *models.SilentRestoreCheckpoint) bool {
	recovery, block := checkpoint.SourceRecovery, checkpoint.SourceBlock
	return block.ID != "" && block.SessionID == recovery.SessionID && block.IncarnationID == recovery.IncarnationID && block.ExpectedGeneration == recovery.HarnessGeneration &&
		block.State == models.RecoveryBlockOpen && block.ConsumerReference == "agent_delivery" && (block.Reason == deliveryUnknownPromptReason || block.Reason == deliveryUnresolvedWorkReason) &&
		block.DeliverySubmissionID == recovery.SubmissionID && block.DeliveryStreamID == recovery.StreamID &&
		block.DeliverySequence == 0 && block.DeliveryTurnID == "" && (block.DeliveryOutcome == "" || block.DeliveryOutcome == reconstructionUnknownOutcome)
}

// A superseded attempt is blocked, never restored. The checkpoint and any
// original failure reason remain available; no original submission is changed.
func completeSupersededContextAttemptTx(ctx context.Context, tx *sqlx.Tx, rebind func(string) string, attempt *models.RestoreAttempt, previous string, completedAt time.Time) (bool, error) {
	terminal := *attempt.Checkpoint
	terminal.Stage = models.SilentRestoreStageTerminal
	encoded, err := marshalSilentRestoreCheckpoint(&terminal)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, rebind(`UPDATE session_restore_attempts
 SET outcome = 'blocked', reason = CASE WHEN reason = '' THEN 'superseded_by_context_continuation' ELSE reason END,
 completed_at = ?, checkpoint_json = ?
 WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
 AND action = ? AND outcome = ? AND completed_at IS NULL AND checkpoint_json = ?`),
		completedAt, encoded, attempt.ID, attempt.SessionID, attempt.IncarnationID, attempt.ExpectedGeneration,
		models.SilentRestoreAction, models.SilentRestoreOutcomePending, previous)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
