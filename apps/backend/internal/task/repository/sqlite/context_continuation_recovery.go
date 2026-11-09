package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const contextContinuationOutcome = "context_continued"

const resolvedContextContinuationNoticeQuery = `SELECT EXISTS (
 SELECT 1 FROM harness_session_generations g JOIN task_sessions ts ON ts.id = g.session_id
 JOIN session_recovery_blocks b ON b.session_id = g.session_id AND b.incarnation_id = g.incarnation_id AND b.expected_generation = g.generation
 JOIN agent_delivery_submissions s ON s.id = b.delivery_submission_id
 WHERE g.session_id = ? AND g.incarnation_id = ? AND g.generation = ? AND g.predecessor_generation = ?
 AND ts.queue_incarnation_id = g.incarnation_id AND g.creation_reason = 'context_continued' AND g.native_session_id <> ''
 AND NOT EXISTS (SELECT 1 FROM harness_session_generations newer WHERE newer.session_id = g.session_id AND newer.incarnation_id = g.incarnation_id AND newer.generation > g.generation)
 AND b.state = 'resolved' AND b.authorized_action = 'continue_from_history'
 AND b.reason = 'unknown_prompt_outcome' AND b.consumer_reference = 'agent_delivery'
 AND b.delivery_submission_id = ? AND b.delivery_stream_id = ?
 AND b.delivery_sequence = 0 AND b.delivery_turn_id = '' AND b.delivery_outcome IN ('', 'unknown')
 AND s.session_id = b.session_id AND s.incarnation_id = b.incarnation_id AND s.harness_generation = g.predecessor_generation
 AND s.state = 'interrupted_unknown'
 )`

// Context continuation retires only the predecessor's presentation. Its unknown
// submission and the carried admission fence remain authoritative and unchanged.
func (r *Repository) retireContextContinuationNoticeTx(ctx context.Context, tx *sql.Tx, generation *models.HarnessSessionGeneration, expectedGeneration int64) error {
	if generation.CreationReason != contextContinuationOutcome || generation.Generation != expectedGeneration+1 ||
		generation.PredecessorGeneration != expectedGeneration || generation.NativeSessionID == "" {
		return nil
	}
	var raw sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT metadata FROM task_sessions WHERE id = ?`), generation.SessionID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeAgentDeliveryMetadata(raw)
	if err != nil {
		return err
	}
	recovery, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil || !exists {
		return err
	}
	if !contextContinuationNoticeMatches(recovery, generation.SessionID, generation.IncarnationID, expectedGeneration) {
		return nil
	}
	var owned bool
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks b JOIN agent_delivery_submissions s ON s.id = b.delivery_submission_id
 WHERE b.session_id = ? AND b.incarnation_id = ? AND b.expected_generation = ? AND b.state = 'open'
 AND b.reason = 'unknown_prompt_outcome' AND b.consumer_reference = 'agent_delivery'
 AND b.delivery_submission_id = ? AND b.delivery_stream_id = ?
 AND b.delivery_sequence = 0 AND b.delivery_turn_id = '' AND b.delivery_outcome IN ('', 'unknown')
 AND s.session_id = b.session_id AND s.incarnation_id = b.incarnation_id AND s.harness_generation = ?
 AND s.state = 'interrupted_unknown'
 )`), recovery.SessionID, recovery.IncarnationID, expectedGeneration, recovery.SubmissionID, recovery.StreamID, expectedGeneration).Scan(&owned)
	if err != nil || !owned {
		return err
	}
	return r.writeContinuedRecoveryNoticeTx(ctx, tx, metadata, raw, recovery, generation.CommittedAt)
}

func contextContinuationNoticeMatches(recovery models.AgentDeliveryRecovery, sessionID, incarnationID string, generation int64) bool {
	return sessionID != "" && incarnationID != "" && generation > 0 && recovery.SessionID == sessionID && recovery.IncarnationID == incarnationID && recovery.HarnessGeneration == generation &&
		recovery.SubmissionID != "" && recovery.StreamID != "" && recovery.Revision > 0 && recovery.HistoryGap == nil &&
		(recovery.Phase == models.AgentDeliveryRecoveryUncertain || recovery.Phase == models.AgentDeliveryRecoveryReconnecting ||
			recovery.Phase == models.AgentDeliveryRecoveryContinued)
}

// Authenticated native retirement needs an exact resolved source fence or a
// complete authorized continuation ancestry before crossing historical generations.
func (r *Repository) hasResolvedContextContinuationNoticeTx(ctx context.Context, tx *sql.Tx, recovery models.AgentDeliveryRecovery, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	if recovery.HarnessGeneration >= proof.HarnessGeneration || !contextContinuationNoticeMatches(recovery, proof.SessionID, proof.IncarnationID, recovery.HarnessGeneration) ||
		!nativeResumeProofAdmitsHistoricalNotice(proof, recovery) {
		return false, nil
	}
	var valid bool
	err := tx.QueryRowContext(ctx, r.db.Rebind(resolvedContextContinuationNoticeQuery), proof.SessionID, proof.IncarnationID,
		proof.HarnessGeneration, recovery.HarnessGeneration, recovery.SubmissionID, recovery.StreamID).Scan(&valid)
	if err != nil || valid {
		return valid, err
	}
	return r.hasCompletedContextAncestryTx(ctx, tx, recovery, proof.HarnessGeneration)
}

// A retained peer record must match the notice exactly. A record the peer no
// longer retains is absent from the proof; the inventory check has already
// required such SQL history to have a consumed continuation ancestry, which
// the SQL checks below verify again for this notice.
func nativeResumeProofAdmitsHistoricalNotice(proof *models.NativeResumeDeliveryEvidence, recovery models.AgentDeliveryRecovery) bool {
	if nativeResumeProofCoversNotice(proof, recovery) {
		return true
	}
	for _, row := range proof.Submissions {
		if row.ID == recovery.SubmissionID {
			return false
		}
	}
	return true
}

// HasResolvedContextContinuationRecovery verifies the historical notice without
// treating the predecessor submission as completed or releasing any current fence.
func (r *Repository) HasResolvedContextContinuationRecovery(ctx context.Context, recovery models.AgentDeliveryRecovery) (bool, error) {
	if recovery.Phase != models.AgentDeliveryRecoveryContinued ||
		!contextContinuationNoticeMatches(recovery, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration) {
		return false, nil
	}
	tx, err := r.ro.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var target int64
	if err := tx.QueryRowContext(ctx, r.ro.Rebind(`SELECT COALESCE(MAX(generation), 0) FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?`), recovery.SessionID, recovery.IncarnationID).Scan(&target); err != nil {
		return false, err
	}
	var valid bool
	err = tx.QueryRowContext(ctx, r.ro.Rebind(resolvedContextContinuationNoticeQuery), recovery.SessionID, recovery.IncarnationID,
		target, recovery.HarnessGeneration, recovery.SubmissionID, recovery.StreamID).Scan(&valid)
	if err != nil || valid {
		return valid, err
	}
	return r.hasCompletedContextAncestryTx(ctx, tx, recovery, target)
}

func (r *Repository) writeContinuedRecoveryNoticeTx(ctx context.Context, tx *sql.Tx, metadata map[string]json.RawMessage, previous sql.NullString, recovery models.AgentDeliveryRecovery, completedAt time.Time) error {
	if recovery.Phase == models.AgentDeliveryRecoveryContinued {
		return nil
	}
	recovery.Phase = models.AgentDeliveryRecoveryContinued
	recovery.Revision++
	recovery.UpdatedAt = completedAt
	encoded, err := json.Marshal(recovery)
	if err != nil {
		return err
	}
	metadata[models.SessionMetaKeyAgentDeliveryRecovery] = encoded
	next, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET metadata = ?, updated_at = ? WHERE id = ? AND queue_incarnation_id = ? AND COALESCE(metadata, '') = ?`),
		string(next), completedAt, recovery.SessionID, recovery.IncarnationID, nullableMetadataValue(previous))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("context continuation recovery metadata changed")
	}
	return err
}
