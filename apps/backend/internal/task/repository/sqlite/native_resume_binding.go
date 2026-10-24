package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	deliveryUnknownPromptReason  = "unknown_prompt_outcome"
	deliveryUnresolvedWorkReason = "unresolved_durable_work"
)

// ResolveSessionRecoveryBlockWithNativeEvidence commits only after authenticated
// native retirement. All incident fences are verified before any becomes usable.
func (r *Repository) ResolveSessionRecoveryBlockWithNativeEvidence(ctx context.Context, id string, proof *models.NativeResumeDeliveryEvidence, completedAt time.Time) (bool, error) {
	if !validNativeResumeProof(proof) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockNativeResumeProofTx(ctx, tx, proof); err != nil {
		return false, err
	}
	valid, err := r.verifyNativeResumeSnapshotTx(ctx, tx, proof)
	if err != nil || !valid {
		return false, err
	}
	blocks, err := r.nativeResumeIncidentBlocksTx(ctx, tx, id, proof)
	if err != nil || len(blocks) == 0 {
		return false, err
	}
	valid, err = r.finishPreparedNativeAttemptTx(ctx, tx, proof, blocks, completedAt)
	if err != nil || !valid {
		return false, err
	}
	valid, err = r.restoreAuthenticatedNativePresentationTx(ctx, tx, proof, completedAt)
	if err != nil || !valid {
		return false, err
	}
	if err := r.advanceRecoverySessionRevisionTx(ctx, tx, proof.SessionID); err != nil {
		return false, err
	}
	changed, err := r.resolveNativeIncidentBlocksTx(ctx, tx, blocks, proof, completedAt)
	if err != nil || !changed {
		return false, err
	}
	return true, tx.Commit()
}

// VerifyResolvedSessionRecoveryBlockWithNativeEvidence confirms completed recovery
// without changing metadata or releasing parked work again.
func (r *Repository) VerifyResolvedSessionRecoveryBlockWithNativeEvidence(ctx context.Context, id string, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	if !validNativeResumeProof(proof) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockNativeResumeProofTx(ctx, tx, proof); err != nil {
		return false, err
	}
	valid, err := r.verifyNativeResumeSnapshotTx(ctx, tx, proof)
	if err != nil || !valid {
		return false, err
	}
	resolved, err := r.nativeResumeIncidentAlreadyResolvedTx(ctx, tx, id, proof)
	if err != nil || !resolved {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) nativeResumeIncidentAlreadyResolvedTx(ctx context.Context, tx *sqlx.Tx, id string, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	var resolved bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks b
 WHERE b.id = ? AND b.session_id = ? AND b.incarnation_id = ? AND b.expected_generation = ?
 AND b.consumer_reference = 'agent_delivery' AND b.state = 'resolved' AND b.authorized_action = 'resume'
 AND NOT EXISTS (SELECT 1 FROM session_recovery_blocks open_block WHERE open_block.session_id = b.session_id AND open_block.state = 'open')
 )`), id, proof.SessionID, proof.IncarnationID, proof.HarnessGeneration).Scan(&resolved)
	return resolved, err
}

func (r *Repository) verifyNativeResumeSnapshotTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	var valid bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM task_sessions ts JOIN harness_session_generations g ON g.session_id = ts.id AND g.incarnation_id = ts.queue_incarnation_id
 JOIN executors_running e ON e.session_id = ts.id
 WHERE ts.id = ? AND ts.queue_incarnation_id = ? AND ts.state <> 'CANCELLED'
 AND g.generation = ? AND g.native_session_id = ? AND e.agent_execution_id = ?
 AND NOT EXISTS (SELECT 1 FROM harness_session_generations newer WHERE newer.session_id = ts.id AND newer.incarnation_id = g.incarnation_id AND newer.generation > g.generation)
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_cursors WHERE session_id = ts.id AND incarnation_id = g.incarnation_id AND harness_generation = g.generation AND received_sequence <> projected_sequence)
 )`), proof.SessionID, proof.IncarnationID, proof.HarnessGeneration, proof.NativeSessionID, proof.ExecutionID).Scan(&valid)
	if err != nil || !valid {
		return false, err
	}
	valid, err = r.verifyNativeResumeInventoryTx(ctx, tx, proof)
	if err != nil || !valid {
		return false, err
	}
	return r.verifyNativeResumeCursorTx(ctx, tx, proof)
}

func (r *Repository) verifyNativeResumeInventoryTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	retired, valid := nativeResumeSubmissionMap(proof)
	if !valid {
		return false, nil
	}
	terminalCount, valid, err := r.verifyNativeResumeTerminalsTx(ctx, tx, proof)
	if err != nil || !valid {
		return false, err
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`SELECT id, incarnation_id, harness_generation, payload_hash, state, terminal_stream_id, terminal_sequence FROM agent_delivery_submissions WHERE session_id = ? AND state IN ('prepared', 'accepted', 'dispatching', 'interrupted_unknown')`), proof.SessionID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id, incarnation, hash, stream string
		var generation, sequence int64
		var state models.DeliverySubmissionState
		if err := rows.Scan(&id, &incarnation, &generation, &hash, &state, &stream, &sequence); err != nil {
			return false, err
		}
		row, found := retired[id]
		if !found || state != models.DeliverySubmissionInterruptedUnknown || (row.State != "" && row.State != state) || incarnation != proof.IncarnationID ||
			generation != row.HarnessGeneration || hash != row.PayloadHash || sequence != row.TerminalSequence || stream != row.TerminalStreamID {
			return false, nil
		}
		count++
	}
	return count+terminalCount == len(retired), rows.Err()
}

func (r *Repository) verifyNativeResumeCursorTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence) (bool, error) {
	var cursor models.AgentDeliveryCursor
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, retired_sequence FROM agent_delivery_cursors WHERE stream_id = ?`), proof.StreamID).Scan(
		&cursor.SessionID, &cursor.IncarnationID, &cursor.HarnessGeneration, &cursor.ReceivedSequence, &cursor.ProjectedSequence, &cursor.RetiredSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return proof.Cursor == nil, nil
	}
	if err != nil {
		return false, err
	}
	pinned := proof.Cursor
	return pinned != nil && pinned.StreamID == proof.StreamID && cursor.SessionID == proof.SessionID && cursor.IncarnationID == proof.IncarnationID &&
		cursor.HarnessGeneration == proof.HarnessGeneration && cursor.SessionID == pinned.SessionID && cursor.IncarnationID == pinned.IncarnationID &&
		cursor.HarnessGeneration == pinned.HarnessGeneration && cursor.ReceivedSequence == pinned.ReceivedSequence &&
		cursor.ProjectedSequence == pinned.ProjectedSequence && cursor.RetiredSequence == pinned.RetiredSequence, nil
}

func (r *Repository) nativeResumeIncidentBlocksTx(ctx context.Context, tx *sqlx.Tx, id string, proof *models.NativeResumeDeliveryEvidence) ([]models.SessionRecoveryBlock, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`SELECT id, incarnation_id, expected_generation, reason, consumer_reference, delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome FROM session_recovery_blocks WHERE session_id = ? AND state = 'open'`), proof.SessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	found := false
	var blocks []models.SessionRecoveryBlock
	for rows.Next() {
		var block models.SessionRecoveryBlock
		if err := rows.Scan(&block.ID, &block.IncarnationID, &block.ExpectedGeneration, &block.Reason, &block.ConsumerReference, &block.DeliverySubmissionID,
			&block.DeliveryStreamID, &block.DeliverySequence, &block.DeliveryTurnID, &block.DeliveryOutcome); err != nil {
			return nil, err
		}
		if !nativeResumeIncidentBlockMatches(block, proof) {
			return nil, nil
		}
		found = found || block.ID == id
		blocks = append(blocks, block)
	}
	if !found {
		return nil, rows.Err()
	}
	return blocks, rows.Err()
}

func nativeResumeIncidentBlockMatches(block models.SessionRecoveryBlock, proof *models.NativeResumeDeliveryEvidence) bool {
	if !nativeResumeIncidentOwnerMatches(block, proof) {
		return false
	}
	if block.DeliverySubmissionID == "" {
		return block.Reason == deliveryUnresolvedWorkReason && block.DeliveryStreamID == ""
	}
	if block.DeliveryStreamID != "" && block.DeliveryStreamID != proof.StreamID {
		return false
	}
	for _, row := range proof.Submissions {
		if row.ID == block.DeliverySubmissionID && row.HarnessGeneration == proof.HarnessGeneration && row.StreamID == proof.StreamID {
			return true
		}
	}
	return false
}

func (r *Repository) restoreAuthenticatedNativeNoticeTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence, completedAt time.Time) (bool, error) {
	var taskID string
	var state models.TaskSessionState
	var metadataRaw sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT task_id, state, metadata FROM task_sessions WHERE id = ?`), proof.SessionID).Scan(&taskID, &state, &metadataRaw); err != nil {
		return false, err
	}
	metadata, err := decodeAgentDeliveryMetadata(metadataRaw)
	if err != nil {
		return false, err
	}
	var acp struct {
		SessionID string `json:"session_id"`
	}
	if raw := metadata["acp"]; len(raw) != 0 && (json.Unmarshal(raw, &acp) != nil || acp.SessionID != proof.NativeSessionID) {
		return false, nil
	}
	recovery, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil || !exists {
		return !exists, err
	}
	if recovery.HistoryGap != nil || recovery.SessionID != proof.SessionID || recovery.IncarnationID != proof.IncarnationID {
		return false, nil
	}
	if recovery.Phase == models.AgentDeliveryRecoveryRestored {
		resolved, err := r.hasResolvedNativeResumeRecoveryTx(ctx, tx, recovery)
		if err != nil || resolved {
			return resolved, err
		}
		return nativeResumeProofCoversNotice(proof, recovery), nil
	}
	if !nativeResumeNoticeMatches(proof, recovery) {
		return false, nil
	}
	recovery.StreamID = proof.StreamID
	return writeSilentRestoreRecoveryTx(ctx, tx, r.db.Rebind, &silentRestoreSessionRow{taskID: taskID, state: state, metadata: metadata, current: recovery}, metadataRaw, completedAt, "")
}

func nativeResumeProofCoversNotice(proof *models.NativeResumeDeliveryEvidence, recovery models.AgentDeliveryRecovery) bool {
	for _, row := range proof.Submissions {
		if row.ID == recovery.SubmissionID && row.HarnessGeneration == recovery.HarnessGeneration && row.StreamID != "" && row.StreamID == recovery.StreamID {
			return true
		}
	}
	return false
}

func validNativeResumeProof(proof *models.NativeResumeDeliveryEvidence) bool {
	return proof != nil && proof.SessionID != "" && proof.IncarnationID != "" && proof.HarnessGeneration > 0 && proof.NativeSessionID != "" && proof.StreamID != "" && proof.ExecutionID != ""
}
func (r *Repository) resolveNativeIncidentBlocksTx(ctx context.Context, tx *sqlx.Tx, blocks []models.SessionRecoveryBlock, proof *models.NativeResumeDeliveryEvidence, completedAt time.Time) (bool, error) {
	for _, block := range blocks {
		stream := block.DeliveryStreamID
		if block.DeliverySubmissionID != "" && stream == "" {
			stream = proof.StreamID
		}
		result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = ?, state = ?, authorized_action = ?, updated_at = ?, resolved_at = ? WHERE id = ? AND state = ?`),
			stream, models.RecoveryBlockResolved, nativeResumeRecoveryAction, completedAt, completedAt, block.ID, models.RecoveryBlockOpen)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count != 1 {
			return false, nil
		}
	}
	return true, nil
}

func nativeResumeSubmissionMap(proof *models.NativeResumeDeliveryEvidence) (map[string]models.NativeResumeSubmissionEvidence, bool) {
	retired := make(map[string]models.NativeResumeSubmissionEvidence, len(proof.Submissions))
	for _, row := range proof.Submissions {
		if row.ID == "" || row.PayloadHash == "" || row.StreamID == "" || row.HarnessGeneration <= 0 || row.HarnessGeneration > proof.HarnessGeneration ||
			(row.HarnessGeneration == proof.HarnessGeneration && row.StreamID != proof.StreamID) {
			return nil, false
		}
		if _, duplicate := retired[row.ID]; duplicate {
			return nil, false
		}
		retired[row.ID] = row
	}
	return retired, true
}

func nativeResumeIncidentOwnerMatches(block models.SessionRecoveryBlock, proof *models.NativeResumeDeliveryEvidence) bool {
	if block.IncarnationID != proof.IncarnationID || block.ExpectedGeneration != proof.HarnessGeneration || block.ConsumerReference != reconstructionDeliveryConsumer ||
		(block.Reason != deliveryUnknownPromptReason && block.Reason != deliveryUnresolvedWorkReason) ||
		block.DeliverySequence != 0 || block.DeliveryTurnID != "" || (block.DeliveryOutcome != "" && block.DeliveryOutcome != reconstructionUnknownOutcome) {
		return false
	}
	return true
}

// A recovered notice can outlive its execution after a later readiness
// failure; authenticated native retirement restores it with its historical owner.
func nativeResumeNoticeMatches(proof *models.NativeResumeDeliveryEvidence, recovery models.AgentDeliveryRecovery) bool {
	if recovery.HarnessGeneration != proof.HarnessGeneration || (recovery.StreamID != "" && recovery.StreamID != proof.StreamID) ||
		(recovery.Phase != models.AgentDeliveryRecoveryUncertain && recovery.Phase != models.AgentDeliveryRecoveryReconnecting &&
			recovery.Phase != models.AgentDeliveryRecoveryRecovered) {
		return false
	}
	found := false
	for _, row := range proof.Submissions {
		found = found || row.ID == recovery.SubmissionID && row.HarnessGeneration == recovery.HarnessGeneration
	}
	return found
}

func (r *Repository) lockNativeResumeProofTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence) error {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), proof.SessionID); err != nil {
		return err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{proof.SessionID}, []string{proof.StreamID}); err != nil {
		return err
	}
	return nil
}
