package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// UpsertAgentDeliveryRecovery stores a revisioned delivery-recovery view only
// while the session still belongs to the event's execution and incarnation.
// It rejects older prompt and harness generations and phase regressions.
func (r *Repository) UpsertAgentDeliveryRecovery(
	ctx context.Context,
	recovery *models.AgentDeliveryRecovery,
	block *models.SessionRecoveryBlock,
) (bool, error) {
	if err := validateAgentDeliveryRecovery(recovery); err != nil {
		return false, err
	}
	if recovery.Phase != models.AgentDeliveryRecoveryReconnecting && recovery.Phase != models.AgentDeliveryRecoveryUncertain {
		return false, fmt.Errorf("phase %q cannot open delivery recovery", recovery.Phase)
	}
	if err := validateBoundAgentDeliveryRecoveryBlock(recovery, block); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	owner, valid, err := r.lockAgentDeliveryRecoveryOwnerTx(ctx, tx, recovery)
	if err != nil || !valid {
		return false, err
	}
	if owner.exists && !agentDeliveryRecoveryMayAdvance(owner.current, *recovery) {
		return false, nil
	}
	updated, err := r.persistAgentDeliveryRecoveryMetadataTx(
		ctx, tx, recovery, owner, "serialize delivery recovery metadata",
	)
	if err != nil || !updated {
		return false, err
	}
	if err := r.upsertSessionRecoveryBlockTx(ctx, tx, block); err != nil {
		return false, fmt.Errorf("persist bound delivery recovery block: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type agentDeliveryRecoveryTxOwner struct {
	metadata      map[string]json.RawMessage
	metadataValue sql.NullString
	incarnationID string
	current       models.AgentDeliveryRecovery
	exists        bool
}

func (r *Repository) lockAgentDeliveryRecoveryOwnerTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery *models.AgentDeliveryRecovery,
) (*agentDeliveryRecoveryTxOwner, bool, error) {
	submissionQuery := `SELECT session_id, incarnation_id, harness_generation, state
		FROM agent_delivery_submissions WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		submissionQuery += forUpdateClause
	}
	var sessionID, incarnationID string
	var generation int64
	var state models.DeliverySubmissionState
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(submissionQuery), recovery.SubmissionID).Scan(
		&sessionID, &incarnationID, &generation, &state,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if sessionID != recovery.SessionID || incarnationID != recovery.IncarnationID ||
		generation != recovery.HarnessGeneration || isTerminalAgentDeliverySubmission(state) {
		return nil, false, nil
	}

	sessionQuery := `SELECT COALESCE(er.agent_execution_id, ''), ts.queue_incarnation_id, ts.metadata
		FROM task_sessions ts LEFT JOIN executors_running er ON er.session_id = ts.id WHERE ts.id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		sessionQuery += forUpdateClause
	}
	var executionID string
	owner := &agentDeliveryRecoveryTxOwner{}
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(sessionQuery), recovery.SessionID).Scan(
		&executionID, &owner.incarnationID, &owner.metadataValue,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if executionID != recovery.AgentExecutionID || owner.incarnationID != recovery.IncarnationID {
		return nil, false, nil
	}
	metadata, err := decodeAgentDeliveryMetadata(owner.metadataValue)
	if err != nil {
		return nil, false, err
	}
	current, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil {
		return nil, false, err
	}
	owner.metadata = metadata
	owner.current = current
	owner.exists = exists
	return owner, true, nil
}

func (r *Repository) persistAgentDeliveryRecoveryMetadataTx(
	ctx context.Context,
	tx *sqlx.Tx,
	recovery *models.AgentDeliveryRecovery,
	owner *agentDeliveryRecoveryTxOwner,
	serializeError string,
) (bool, error) {
	recovery.Revision = 1
	if owner.exists {
		recovery.Revision = owner.current.Revision + 1
	}
	recovery.UpdatedAt = r.nowUTC()
	encodedRecovery, err := json.Marshal(recovery)
	if err != nil {
		return false, fmt.Errorf("%s: %w", serializeError, err)
	}
	owner.metadata[models.SessionMetaKeyAgentDeliveryRecovery] = encodedRecovery
	encodedMetadata, err := json.Marshal(owner.metadata)
	if err != nil {
		return false, fmt.Errorf("serialize session metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET metadata = ?, updated_at = ?
		WHERE id = ? AND queue_incarnation_id = ?
		  AND EXISTS (SELECT 1 FROM executors_running er WHERE er.session_id = task_sessions.id AND er.agent_execution_id = ?)
		  AND COALESCE(metadata, '') = ?
	`), string(encodedMetadata), recovery.UpdatedAt, recovery.SessionID,
		recovery.IncarnationID, recovery.AgentExecutionID, nullableMetadataValue(owner.metadataValue))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	return true, nil
}

func validateBoundAgentDeliveryRecoveryBlock(
	recovery *models.AgentDeliveryRecovery,
	block *models.SessionRecoveryBlock,
) error {
	if block == nil || block.SessionID != recovery.SessionID || block.IncarnationID != recovery.IncarnationID ||
		block.ExpectedGeneration != recovery.HarnessGeneration || block.ConsumerReference != "agent_delivery" ||
		block.DeliverySubmissionID != recovery.SubmissionID || block.DeliveryStreamID != recovery.StreamID ||
		block.State != models.RecoveryBlockOpen || block.Reason != "unknown_prompt_outcome" {
		return errors.New("delivery recovery block must match the active prompt identity")
	}
	return nil
}

func isTerminalAgentDeliverySubmission(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionCompleted || state == models.DeliverySubmissionFailed ||
		state == models.DeliverySubmissionCancelled
}

func validateAgentDeliveryRecovery(recovery *models.AgentDeliveryRecovery) error {
	if recovery == nil || recovery.SessionID == "" || recovery.AgentExecutionID == "" ||
		recovery.SubmissionID == "" || recovery.StreamID == "" || recovery.IncarnationID == "" ||
		recovery.HarnessGeneration <= 0 || recovery.PromptGeneration == 0 {
		return errors.New("complete durable delivery recovery identity is required")
	}
	if recovery.Phase != models.AgentDeliveryRecoveryReconnecting && recovery.Phase != models.AgentDeliveryRecoveryUncertain &&
		recovery.Phase != models.AgentDeliveryRecoveryRecovered && recovery.Phase != models.AgentDeliveryRecoverySettled {
		return fmt.Errorf("unsupported durable delivery recovery phase %q", recovery.Phase)
	}
	return nil
}

func decodeAgentDeliveryMetadata(value sql.NullString) (map[string]json.RawMessage, error) {
	metadata := map[string]json.RawMessage{}
	if !value.Valid || value.String == "" || value.String == jsonNull {
		return metadata, nil
	}
	if err := json.Unmarshal([]byte(value.String), &metadata); err != nil {
		return nil, fmt.Errorf("decode session metadata for delivery recovery: %w", err)
	}
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	return metadata, nil
}

func nullableMetadataValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func agentDeliveryRecoveryFromMetadata(
	metadata map[string]json.RawMessage,
) (models.AgentDeliveryRecovery, bool, error) {
	raw, ok := metadata[models.SessionMetaKeyAgentDeliveryRecovery]
	if !ok || string(raw) == jsonNull {
		return models.AgentDeliveryRecovery{}, false, nil
	}
	var recovery models.AgentDeliveryRecovery
	if err := json.Unmarshal(raw, &recovery); err != nil {
		return models.AgentDeliveryRecovery{}, false, fmt.Errorf("decode current durable delivery recovery: %w", err)
	}
	return recovery, true, nil
}

func agentDeliveryRecoveryMayAdvance(current, incoming models.AgentDeliveryRecovery) bool {
	if incoming.HarnessGeneration < current.HarnessGeneration {
		return false
	}
	if incoming.HarnessGeneration > current.HarnessGeneration {
		return true
	}
	if incoming.PromptGeneration < current.PromptGeneration {
		return false
	}
	if incoming.PromptGeneration > current.PromptGeneration {
		return true
	}
	if incoming.SubmissionID != current.SubmissionID || incoming.StreamID != current.StreamID {
		return false
	}
	return recoveryPhaseRank(incoming.Phase) > recoveryPhaseRank(current.Phase)
}

func recoveryPhaseRank(phase string) int {
	if phase == models.AgentDeliveryRecoverySettled {
		return 4
	}
	if phase == models.AgentDeliveryRecoveryRecovered {
		return 3
	}
	if phase == models.AgentDeliveryRecoveryUncertain {
		return 2
	}
	if phase == models.AgentDeliveryRecoveryReconnecting {
		return 1
	}
	return 0
}

// ClearAgentDeliveryRecovery marks the notice settled only if its immutable
// prompt identity still matches. A delayed terminal for an older submission
// cannot overwrite a newer recovery state.
func (r *Repository) ClearAgentDeliveryRecovery(
	ctx context.Context,
	sessionID, incarnationID, submissionID string,
	harnessGeneration int64,
) (bool, error) {
	session, err := r.GetTaskSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return false, nil
		}
		return false, err
	}
	if session.QueueIncarnationID != incarnationID {
		return false, nil
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !ok || recovery.SessionID != sessionID || recovery.IncarnationID != incarnationID || recovery.SubmissionID != submissionID ||
		recovery.HarnessGeneration != harnessGeneration {
		return false, nil
	}
	if recovery.Phase == models.AgentDeliveryRecoverySettled {
		return false, nil
	}
	settled := recovery
	settled.Phase = models.AgentDeliveryRecoverySettled
	settled.Revision++
	settled.UpdatedAt = r.nowUTC()
	return r.SetSessionMetadataKeyIfJSONValue(ctx, sessionID,
		models.SessionMetaKeyAgentDeliveryRecovery, recovery, settled)
}

// ResolveAgentDeliveryRecovery moves one live prompt notice to recovered and
// releases only the matching admission block in the same transaction.
func (r *Repository) ResolveAgentDeliveryRecovery(
	ctx context.Context,
	recovery *models.AgentDeliveryRecovery,
	action string,
) (bool, error) {
	if err := validateAgentDeliveryRecovery(recovery); err != nil {
		return false, err
	}
	if recovery.Phase != models.AgentDeliveryRecoveryRecovered {
		return false, fmt.Errorf("recovery phase %q cannot resolve a live delivery", recovery.Phase)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	owner, valid, err := r.lockAgentDeliveryRecoveryOwnerTx(ctx, tx, recovery)
	if err != nil || !valid {
		return false, err
	}
	if owner.exists && !agentDeliveryRecoveryMayAdvance(owner.current, *recovery) {
		return false, nil
	}
	updated, err := r.persistAgentDeliveryRecoveryMetadataTx(
		ctx, tx, recovery, owner, "serialize recovered delivery metadata",
	)
	if err != nil || !updated {
		return false, err
	}
	if action == "" {
		action = "retry_connection"
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_recovery_blocks SET state = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
		WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ? AND state = ?
		  AND consumer_reference = 'agent_delivery' AND delivery_submission_id = ? AND delivery_stream_id = ?
	`), models.RecoveryBlockResolved, action, recovery.UpdatedAt, recovery.UpdatedAt,
		recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration,
		models.RecoveryBlockOpen, recovery.SubmissionID, recovery.StreamID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
