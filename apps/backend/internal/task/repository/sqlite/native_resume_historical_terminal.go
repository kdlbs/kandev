package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const historicalTerminalAttemptReason = "source_terminal_settled"

type historicalTerminalSettlement struct {
	payloadHash string
	state       models.DeliverySubmissionState
	outcome     string
	streamID    string
	sequence    int64
}

// HasResolvedHistoricalTerminalRecovery classifies a settled notice as
// historical only when its terminal source is fully proved and a distinct
// unresolved submission exists in the same saved native conversation.
func (r *Repository) HasResolvedHistoricalTerminalRecovery(
	ctx context.Context,
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
	currentNativeSessionID string,
) (bool, error) {
	tx, err := r.ro.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	return r.hasResolvedHistoricalTerminalRecoveryTx(ctx, tx, recovery, currentGeneration, currentNativeSessionID)
}

func (r *Repository) hasResolvedHistoricalTerminalRecoveryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
	currentNativeSessionID string,
) (bool, error) {
	if !validHistoricalTerminalRecoveryIdentity(recovery, currentGeneration, currentNativeSessionID) {
		return false, nil
	}
	currentNative, found, err := r.historicalTerminalCurrentNativeSessionTx(ctx, tx, recovery, currentGeneration)
	if err != nil || !found || currentNative != currentNativeSessionID {
		return false, err
	}
	settlement, valid, err := r.loadHistoricalTerminalSettlementTx(ctx, tx, recovery, currentNative, "", "")
	if err != nil || !valid {
		return false, err
	}
	later, err := r.hasLaterInterruptedSubmissionTx(ctx, tx, recovery, currentGeneration)
	if err != nil || !later {
		return false, err
	}
	return settlement.streamID == recovery.StreamID, nil
}

func validHistoricalTerminalRecoveryIdentity(
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
	currentNativeSessionID string,
) bool {
	return recovery.Phase == models.AgentDeliveryRecoverySettled && recovery.HistoryGap == nil &&
		recovery.SessionID != "" && recovery.IncarnationID != "" && recovery.HarnessGeneration > 0 &&
		recovery.HarnessGeneration <= currentGeneration && recovery.SubmissionID != "" &&
		recovery.StreamID != "" && recovery.AgentExecutionID != "" && currentNativeSessionID != ""
}

func (r *Repository) historicalTerminalCurrentNativeSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
) (string, bool, error) {
	var currentNative string
	err := tx.GetContext(ctx, &currentNative, r.db.Rebind(`SELECT native_session_id FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ? AND generation = ?`), recovery.SessionID,
		recovery.IncarnationID, currentGeneration)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return currentNative, true, nil
}

func (r *Repository) hasLaterInterruptedSubmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	currentGeneration int64,
) (bool, error) {
	var later bool
	err := tx.GetContext(ctx, &later, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM agent_delivery_submissions s
		WHERE s.session_id = ? AND s.incarnation_id = ? AND s.id <> ?
		  AND s.harness_generation <= ? AND s.state IN ('dispatching', 'interrupted_unknown')
	)`), recovery.SessionID, recovery.IncarnationID, recovery.SubmissionID, currentGeneration)
	return later, err
}

func (r *Repository) preparedHistoricalTerminalAttemptMatchesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	attempt *models.RestoreAttempt,
	proof *models.NativeResumeDeliveryEvidence,
	blocks []models.SessionRecoveryBlock,
) (bool, error) {
	recovery, sourceBlock, nativeSessionID, valid := preparedHistoricalTerminalAttemptSource(attempt)
	if !valid || nativeSessionID != proof.NativeSessionID || len(blocks) == 0 ||
		!nativeResumeHasLaterInterruptedIncident(proof, recovery.SubmissionID, blocks) {
		return false, nil
	}
	settlement, valid, err := r.loadHistoricalTerminalSettlementTx(ctx, tx, recovery, nativeSessionID, sourceBlock.ID, sourceBlock.Reason)
	if err != nil || !valid {
		return false, err
	}
	row, found := nativeResumeSubmissionEvidence(proof, recovery.SubmissionID)
	if found && !historicalTerminalEvidenceMatches(row, recovery, settlement) {
		return false, nil
	}
	return true, nil
}

func (r *Repository) historicalTerminalNoticeMatchesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	proof *models.NativeResumeDeliveryEvidence,
	blocks []models.SessionRecoveryBlock,
) (bool, error) {
	if !validHistoricalTerminalNotice(recovery, proof, blocks) {
		return false, nil
	}
	settlement, valid, err := r.loadHistoricalTerminalSettlementTx(ctx, tx, recovery, proof.NativeSessionID, "", "")
	if err != nil || !valid {
		return false, err
	}
	row, found := nativeResumeSubmissionEvidence(proof, recovery.SubmissionID)
	if found && !historicalTerminalEvidenceMatches(row, recovery, settlement) {
		return false, nil
	}
	return true, nil
}

func validHistoricalTerminalNotice(
	recovery models.AgentDeliveryRecovery,
	proof *models.NativeResumeDeliveryEvidence,
	blocks []models.SessionRecoveryBlock,
) bool {
	return recovery.Phase == models.AgentDeliveryRecoverySettled && recovery.HistoryGap == nil &&
		recovery.SessionID == proof.SessionID && recovery.IncarnationID == proof.IncarnationID &&
		recovery.HarnessGeneration > 0 && recovery.HarnessGeneration <= proof.HarnessGeneration &&
		recovery.SubmissionID != "" && recovery.StreamID != "" && recovery.AgentExecutionID != "" &&
		len(blocks) > 0 && nativeResumeHasLaterInterruptedIncident(proof, recovery.SubmissionID, blocks)
}

func preparedHistoricalTerminalAttemptSource(
	attempt *models.RestoreAttempt,
) (models.AgentDeliveryRecovery, models.SessionRecoveryBlock, string, bool) {
	if !validPreparedTerminalAttempt(attempt) {
		return models.AgentDeliveryRecovery{}, models.SessionRecoveryBlock{}, "", false
	}
	checkpoint := attempt.Checkpoint
	recovery, block, generation := checkpoint.SourceRecovery, checkpoint.SourceBlock, checkpoint.SourceGeneration
	if !validPreparedTerminalAttemptIdentity(attempt, recovery, generation) ||
		!validPreparedTerminalAttemptBlock(attempt, recovery, block) || !validPreparedTerminalAttemptRecovery(recovery) {
		return models.AgentDeliveryRecovery{}, models.SessionRecoveryBlock{}, "", false
	}
	return recovery, block, generation.NativeSessionID, true
}

func validPreparedTerminalAttempt(attempt *models.RestoreAttempt) bool {
	return attempt != nil && attempt.Checkpoint != nil && attempt.Authorized && attempt.Action == models.SilentRestoreAction &&
		attempt.Outcome == models.SilentRestoreOutcomePending && attempt.CompletedAt == nil
}

func validPreparedTerminalAttemptIdentity(
	attempt *models.RestoreAttempt,
	recovery models.AgentDeliveryRecovery,
	generation models.HarnessSessionGeneration,
) bool {
	checkpoint := attempt.Checkpoint
	return checkpoint.Version == 1 && checkpoint.Stage == models.SilentRestoreStagePrepared && checkpoint.CandidateExecutionID == "" &&
		attempt.SessionID == recovery.SessionID && attempt.IncarnationID == recovery.IncarnationID &&
		attempt.ExpectedGeneration == generation.Generation && recovery.HarnessGeneration == generation.Generation &&
		generation.SessionID == attempt.SessionID && generation.IncarnationID == attempt.IncarnationID && generation.NativeSessionID != ""
}

func validPreparedTerminalAttemptBlock(
	attempt *models.RestoreAttempt,
	recovery models.AgentDeliveryRecovery,
	block models.SessionRecoveryBlock,
) bool {
	return block.ID != "" && block.SessionID == attempt.SessionID && block.IncarnationID == attempt.IncarnationID &&
		block.ExpectedGeneration == attempt.ExpectedGeneration && validPreparedTerminalAttemptBlockState(block) &&
		validPreparedTerminalAttemptBlockDelivery(block, recovery)
}

func validPreparedTerminalAttemptBlockState(block models.SessionRecoveryBlock) bool {
	return block.State == models.RecoveryBlockOpen && block.ConsumerReference == reconstructionDeliveryConsumer &&
		(block.Reason == deliveryUnknownPromptReason || block.Reason == deliveryUnresolvedWorkReason)
}

func validPreparedTerminalAttemptBlockDelivery(block models.SessionRecoveryBlock, recovery models.AgentDeliveryRecovery) bool {
	return block.DeliverySubmissionID != "" && block.DeliverySubmissionID == recovery.SubmissionID &&
		(block.DeliveryStreamID == "" || block.DeliveryStreamID == recovery.StreamID) &&
		block.DeliverySequence == 0 && block.DeliveryTurnID == "" &&
		(block.DeliveryOutcome == "" || block.DeliveryOutcome == reconstructionUnknownOutcome)
}

func validPreparedTerminalAttemptRecovery(recovery models.AgentDeliveryRecovery) bool {
	return recovery.SessionID != "" && recovery.IncarnationID != "" && recovery.HarnessGeneration > 0 &&
		recovery.SubmissionID != "" && recovery.StreamID != "" && recovery.AgentExecutionID != "" && recovery.HistoryGap == nil
}

func nativeResumeHasLaterInterruptedIncident(
	proof *models.NativeResumeDeliveryEvidence,
	sourceSubmissionID string,
	blocks []models.SessionRecoveryBlock,
) bool {
	_, found := nativeResumeLaterInterruptedIncident(proof, sourceSubmissionID, blocks)
	return found
}

func nativeResumeLaterInterruptedIncident(
	proof *models.NativeResumeDeliveryEvidence,
	sourceSubmissionID string,
	blocks []models.SessionRecoveryBlock,
) (models.NativeResumeSubmissionEvidence, bool) {
	for _, row := range proof.Submissions {
		if !validLaterInterruptedIncidentRow(proof, sourceSubmissionID, row) {
			continue
		}
		for _, block := range blocks {
			if laterInterruptedIncidentBlockMatches(proof, row, block) {
				return row, true
			}
		}
	}
	return models.NativeResumeSubmissionEvidence{}, false
}

func validLaterInterruptedIncidentRow(
	proof *models.NativeResumeDeliveryEvidence,
	sourceSubmissionID string,
	row models.NativeResumeSubmissionEvidence,
) bool {
	return row.ID != sourceSubmissionID && row.ID != "" && row.StreamID != "" &&
		row.HarnessGeneration > 0 && row.HarnessGeneration <= proof.HarnessGeneration &&
		(row.State == "" || row.State == models.DeliverySubmissionInterruptedUnknown)
}

func laterInterruptedIncidentBlockMatches(
	proof *models.NativeResumeDeliveryEvidence,
	row models.NativeResumeSubmissionEvidence,
	block models.SessionRecoveryBlock,
) bool {
	return block.IncarnationID == proof.IncarnationID && block.ExpectedGeneration == row.HarnessGeneration &&
		(block.DeliverySubmissionID == "" || block.DeliverySubmissionID == row.ID) &&
		(block.DeliveryStreamID == "" || block.DeliveryStreamID == row.StreamID)
}

func nativeResumeSubmissionEvidence(
	proof *models.NativeResumeDeliveryEvidence,
	submissionID string,
) (models.NativeResumeSubmissionEvidence, bool) {
	for _, row := range proof.Submissions {
		if row.ID == submissionID {
			return row, true
		}
	}
	return models.NativeResumeSubmissionEvidence{}, false
}

func (r *Repository) loadHistoricalTerminalSettlementTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	nativeSessionID, sourceBlockID, sourceBlockReason string,
) (*historicalTerminalSettlement, bool, error) {
	if recovery.SessionID == "" || recovery.IncarnationID == "" || recovery.HarnessGeneration <= 0 ||
		recovery.SubmissionID == "" || recovery.StreamID == "" || nativeSessionID == "" {
		return nil, false, nil
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
SELECT s.payload_hash, s.state, s.outcome, s.terminal_stream_id, s.terminal_sequence
FROM agent_delivery_submissions s
JOIN agent_delivery_effects terminal
  ON terminal.stream_id = s.terminal_stream_id AND terminal.sequence = s.terminal_sequence
 AND terminal.effect_type = ? AND terminal.effect_key = 'agent_delivery.terminal:' || terminal.stream_id || ':' || CAST(terminal.sequence AS TEXT)
JOIN agent_delivery_cursors cursor ON cursor.stream_id = terminal.stream_id
JOIN harness_session_generations generation
  ON generation.session_id = s.session_id AND generation.incarnation_id = s.incarnation_id
 AND generation.generation = s.harness_generation
JOIN session_recovery_blocks block
  ON block.session_id = s.session_id AND block.incarnation_id = s.incarnation_id
 AND block.expected_generation = s.harness_generation AND block.delivery_submission_id = s.id
WHERE s.id = ? AND s.session_id = ? AND s.incarnation_id = ? AND s.harness_generation = ?
  AND s.payload_hash <> '' AND s.state IN (?, ?, ?) AND s.outcome = s.state
  AND s.fully_settled_at IS NOT NULL AND s.terminal_stream_id = ? AND s.terminal_sequence > 0
  AND s.terminal_turn_id <> '' AND terminal.stream_id = s.terminal_stream_id
  AND terminal.sequence = s.terminal_sequence AND terminal.turn_id = s.terminal_turn_id
  AND terminal.session_id = s.session_id AND terminal.incarnation_id = s.incarnation_id
  AND terminal.harness_generation = s.harness_generation AND terminal.submission_id = s.id
  AND terminal.outcome = s.outcome AND terminal.state = ? AND terminal.completed_at IS NOT NULL
  AND cursor.session_id = s.session_id AND cursor.incarnation_id = s.incarnation_id
  AND cursor.harness_generation = s.harness_generation
  AND cursor.received_sequence = cursor.projected_sequence
  AND cursor.projected_sequence >= s.terminal_sequence
  AND cursor.retired_sequence >= 0 AND cursor.retired_sequence <= cursor.projected_sequence
  AND generation.native_session_id = ?
  AND block.reason IN (?, ?) AND block.consumer_reference = ? AND block.state = ?
  AND block.delivery_stream_id = s.terminal_stream_id
  AND block.delivery_sequence = s.terminal_sequence AND block.delivery_turn_id = s.terminal_turn_id
  AND block.delivery_outcome = s.outcome
  AND block.authorized_action = 'durable_delivery_' || s.outcome
  AND ((? = '' AND block.reason IN (?, ?)) OR (block.id = ? AND block.reason = ?))
ORDER BY block.id LIMIT 2`),
		models.DeliveryTerminalEffectType, recovery.SubmissionID, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration,
		models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled,
		recovery.StreamID, models.DeliveryEffectCompleted, nativeSessionID,
		deliveryUnknownPromptReason, deliveryUnresolvedWorkReason, reconstructionDeliveryConsumer, models.RecoveryBlockResolved,
		sourceBlockID, deliveryUnknownPromptReason, deliveryUnresolvedWorkReason, sourceBlockID, sourceBlockReason)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	var matches []historicalTerminalSettlement
	for rows.Next() {
		var settlement historicalTerminalSettlement
		if err := rows.Scan(&settlement.payloadHash, &settlement.state, &settlement.outcome, &settlement.streamID, &settlement.sequence); err != nil {
			return nil, false, err
		}
		matches = append(matches, settlement)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(matches) != 1 {
		return nil, false, nil
	}
	return &matches[0], true, nil
}

func historicalTerminalEvidenceMatches(
	row models.NativeResumeSubmissionEvidence,
	recovery models.AgentDeliveryRecovery,
	settlement *historicalTerminalSettlement,
) bool {
	return settlement != nil && row.ID == recovery.SubmissionID && row.HarnessGeneration == recovery.HarnessGeneration &&
		row.StreamID == recovery.StreamID && row.PayloadHash == settlement.payloadHash &&
		row.State == settlement.state && row.Outcome == settlement.outcome &&
		row.TerminalStreamID == settlement.streamID && row.TerminalSequence == settlement.sequence &&
		row.TerminalSequence > 0 && row.TerminalStreamID == row.StreamID
}

func (r *Repository) retirePreparedTerminalAttemptsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	effect *models.AgentDeliveryEffect,
	completedAt time.Time,
) error {
	var ids []string
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(`SELECT id FROM session_restore_attempts WHERE session_id = ? AND completed_at IS NULL`), effect.SessionID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.retirePreparedTerminalAttemptTx(ctx, tx, effect, id, completedAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) retirePreparedTerminalAttemptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	effect *models.AgentDeliveryEffect,
	id string,
	completedAt time.Time,
) error {
	attempt, encoded, err := readSilentRestoreAttemptCheckpointTx(ctx, tx, r.db.DriverName(), id)
	if err != nil {
		return err
	}
	recovery, sourceBlock, nativeSessionID, valid := preparedHistoricalTerminalAttemptSource(attempt)
	if !preparedTerminalAttemptMatchesEffect(effect, recovery, nativeSessionID, valid) {
		return nil
	}
	settlement, settled, err := r.loadHistoricalTerminalSettlementTx(ctx, tx, recovery, nativeSessionID, sourceBlock.ID, sourceBlock.Reason)
	if err != nil || !settled || !terminalSettlementMatchesEffect(settlement, effect) {
		return err
	}
	_, err = completeSupersededTerminalAttemptTx(ctx, tx, r.db.Rebind, attempt, encoded, completedAt)
	return err
}

func preparedTerminalAttemptMatchesEffect(
	effect *models.AgentDeliveryEffect,
	recovery models.AgentDeliveryRecovery,
	nativeSessionID string,
	valid bool,
) bool {
	return valid && nativeSessionID != "" && recovery.SessionID == effect.SessionID &&
		recovery.IncarnationID == effect.IncarnationID && recovery.HarnessGeneration == effect.HarnessGeneration &&
		recovery.SubmissionID == effect.SubmissionID && recovery.StreamID == effect.StreamID
}

func terminalSettlementMatchesEffect(settlement *historicalTerminalSettlement, effect *models.AgentDeliveryEffect) bool {
	return settlement != nil && settlement.state == models.DeliverySubmissionState(effect.Outcome) &&
		settlement.streamID == effect.StreamID && settlement.sequence == effect.Sequence
}

func completeSupersededTerminalAttemptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	attempt *models.RestoreAttempt,
	previous string,
	completedAt time.Time,
) (bool, error) {
	terminal := *attempt.Checkpoint
	terminal.Stage = models.SilentRestoreStageTerminal
	encoded, err := marshalSilentRestoreCheckpoint(&terminal)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, rebind(`UPDATE session_restore_attempts
 SET outcome = ?, reason = CASE WHEN reason = '' THEN ? ELSE reason END,
 completed_at = ?, checkpoint_json = ?
 WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
 AND action = ? AND outcome = ? AND completed_at IS NULL AND checkpoint_json = ?`),
		models.SilentRestoreOutcomeBlocked, historicalTerminalAttemptReason, completedAt, encoded,
		attempt.ID, attempt.SessionID, attempt.IncarnationID, attempt.ExpectedGeneration,
		models.SilentRestoreAction, models.SilentRestoreOutcomePending, previous)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
