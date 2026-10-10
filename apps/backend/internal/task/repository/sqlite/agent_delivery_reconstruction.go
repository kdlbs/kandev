package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	reconstructionDeliveryConsumer           = "agent_delivery"
	reconstructionUnknownOutcome             = "unknown"
	agentDeliveryReconstructionLockNamespace = "agent-delivery-reconstruction:"
	agentDeliveryReconstructionPayloadLimit  = 1 << 20
)

// ReconstructAgentDeliverySubmission restores one verified retained prompt and
// its admission fence in the same transaction.
func (r *Repository) ReconstructAgentDeliverySubmission(
	ctx context.Context,
	request *models.AgentDeliveryReconstructionRequest,
) (*models.AgentDeliveryReconstructionResult, error) {
	if err := validateAgentDeliveryReconstructionRequest(request); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockAgentDeliveryReconstructionTx(ctx, tx, request); err != nil {
		return nil, err
	}
	owner, block, err := r.loadReconstructionOwnerAndBlockTx(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	rows, err := r.lockReconstructionSubmissionsTx(ctx, tx, request.SessionID)
	if err != nil {
		return nil, err
	}
	for id, other := range rows {
		if id != request.Submission.ID && isUnresolvedReconstructionSubmission(other.State) {
			return nil, repoerrors.ErrAgentDeliveryReconstructionConflict
		}
	}
	if existing, exists := rows[request.Submission.ID]; exists {
		return r.reconcileExistingReconstructionTx(ctx, tx, owner, existing, block, request)
	}
	if owner.exists {
		return nil, repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	if err := r.insertReconstructedSubmissionTx(ctx, tx, request); err != nil {
		return nil, err
	}
	recovery, err := r.persistReconstructedRecoveryTx(ctx, tx, request, owner, r.nowUTC())
	if err != nil {
		return nil, err
	}
	boundBlock, err := r.bindReconstructedBlockTx(ctx, tx, request, block, recovery.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &models.AgentDeliveryReconstructionResult{
		Inserted: true, Recovery: recovery, Block: boundBlock,
	}, nil
}

func (r *Repository) loadReconstructionOwnerAndBlockTx(ctx context.Context, tx *sqlx.Tx, request *models.AgentDeliveryReconstructionRequest) (*agentDeliveryReconstructionOwner, *models.SessionRecoveryBlock, error) {
	owner, valid, err := r.lockAgentDeliveryReconstructionOwnerTx(ctx, tx, request)
	if err != nil || !valid {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, repoerrors.ErrAgentDeliveryReconstructionStale
	}
	if err := r.validateReconstructionMessageTx(ctx, tx, request); err != nil {
		return nil, nil, err
	}
	block, valid, err := r.loadReconstructionBlockTx(ctx, tx, request)
	if err != nil || !valid {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, repoerrors.ErrAgentDeliveryReconstructionStale
	}
	if err := r.validateReconstructionCursorTx(ctx, tx, request); err != nil {
		return nil, nil, err
	}
	return owner, block, nil
}

func (r *Repository) reconcileExistingReconstructionTx(ctx context.Context, tx *sqlx.Tx, owner *agentDeliveryReconstructionOwner, existing models.AgentDeliverySubmission, block *models.SessionRecoveryBlock, request *models.AgentDeliveryReconstructionRequest) (*models.AgentDeliveryReconstructionResult, error) {
	if reconstructionAlreadyCommitted(owner, existing, block, request) {
		return commitIdempotentReconstruction(tx, owner.current, *block)
	}
	if canEnrichReconstructedIdentity(owner, existing, block, request) {
		recovery, writeErr := r.persistReconstructedRecoveryTx(ctx, tx, request, owner, r.nowUTC())
		if writeErr != nil {
			return nil, writeErr
		}
		return commitIdempotentReconstruction(tx, recovery, *block)
	}
	return nil, repoerrors.ErrAgentDeliveryReconstructionConflict
}

func (r *Repository) lockAgentDeliveryReconstructionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) error {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), request.SessionID); err != nil {
		return err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			agentDeliveryReconstructionLockNamespace+request.SessionID); err != nil {
			return fmt.Errorf("lock retained delivery reconstruction: %w", err)
		}
		var archivedAt sql.NullTime
		query := `SELECT archived_at FROM tasks WHERE id = ? FOR UPDATE`
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), request.TaskID).Scan(&archivedAt); err != nil {
			return repoerrors.ErrAgentDeliveryReconstructionStale
		}
		if archivedAt.Valid {
			return repoerrors.ErrAgentDeliveryReconstructionStale
		}
		return nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET updated_at = updated_at WHERE id = ?`), request.SessionID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return nil
}

type agentDeliveryReconstructionOwner struct {
	metadata      map[string]json.RawMessage
	metadataValue sql.NullString
	current       models.AgentDeliveryRecovery
	exists        bool
	workspacePath string
}

func (r *Repository) lockAgentDeliveryReconstructionOwnerTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) (*agentDeliveryReconstructionOwner, bool, error) {
	query := `SELECT ts.task_id, ts.queue_incarnation_id, ts.state,
		COALESCE(er.agent_execution_id, ''), COALESCE(NULLIF(te.workspace_path, ''), ts.workspace_path, ''), ts.metadata
		FROM task_sessions ts JOIN tasks t ON t.id = ts.task_id
		LEFT JOIN executors_running er ON er.session_id = ts.id
		LEFT JOIN task_environments te ON te.id = ts.task_environment_id
		WHERE ts.id = ? AND t.archived_at IS NULL`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF ts`
	}
	owner := &agentDeliveryReconstructionOwner{}
	var taskID, incarnationID, executionID string
	var sessionState models.TaskSessionState
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), request.SessionID).Scan(
		&taskID, &incarnationID, &sessionState, &executionID, &owner.workspacePath, &owner.metadataValue,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if taskID != request.TaskID || incarnationID != request.IncarnationID || executionID != request.ExpectedExecutionID ||
		sessionState != request.ExpectedSessionState || isTerminalReconstructionSession(sessionState) ||
		owner.workspacePath != request.ExpectedWorkspacePath {
		return owner, false, nil
	}
	if err := validateReconstructionGenerationTx(ctx, tx, r.db, request); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return owner, false, nil
		}
		return nil, false, err
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

func validateReconstructionGenerationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	request *models.AgentDeliveryReconstructionRequest,
) error {
	query := `SELECT generation, native_session_id, current_workspace
		FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var generation int64
	var nativeSessionID, workspace string
	if err := tx.QueryRowxContext(ctx, db.Rebind(query), request.SessionID, request.IncarnationID).Scan(
		&generation, &nativeSessionID, &workspace,
	); err != nil {
		return err
	}
	if generation != request.HarnessGeneration || nativeSessionID != request.NativeSessionID ||
		(request.ExpectedWorkspacePath != "" && workspace != request.ExpectedWorkspacePath) {
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return nil
}

func isTerminalReconstructionSession(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCompleted || state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}

func (r *Repository) validateReconstructionMessageTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) error {
	query := `SELECT task_id, task_session_id, author_type FROM task_session_messages WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR KEY SHARE`
	}
	var taskID, sessionID string
	var authorType models.MessageAuthorType
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), request.MessageID).Scan(&taskID, &sessionID, &authorType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repoerrors.ErrAgentDeliveryReconstructionStale
		}
		return err
	}
	if taskID != request.TaskID || sessionID != request.SessionID || authorType != models.MessageAuthorUser {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	return nil
}

func (r *Repository) loadReconstructionBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) (*models.SessionRecoveryBlock, bool, error) {
	query := `SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		consumer_reference, delivery_submission_id, delivery_stream_id, delivery_sequence,
		delivery_turn_id, delivery_outcome, authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	block := new(models.SessionRecoveryBlock)
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), request.ExpectedBlock.ID).Scan(
		&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration, &block.Reason,
		&block.State, &block.ConsumerReference, &block.DeliverySubmissionID, &block.DeliveryStreamID,
		&block.DeliverySequence, &block.DeliveryTurnID, &block.DeliveryOutcome, &block.AuthorizedAction,
		&block.CreatedAt, &block.UpdatedAt, &block.ResolvedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !sameReconstructionBlockForReconstruction(*block, request.ExpectedBlock, request) ||
		(block.DeliverySubmissionID != "" && block.DeliverySubmissionID != request.Submission.ID) ||
		(block.DeliveryStreamID != "" && block.DeliveryStreamID != request.Recovery.StreamID) {
		return block, false, nil
	}
	var competingDeliveryBlocks int
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks
		WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ? AND state = ?
		AND consumer_reference = 'agent_delivery' AND id != ?`),
		request.SessionID, request.IncarnationID, request.HarnessGeneration,
		models.RecoveryBlockOpen, block.ID).Scan(&competingDeliveryBlocks); err != nil {
		return nil, false, err
	}
	return block, competingDeliveryBlocks == 0, nil
}

func sameReconstructionBlockForReconstruction(
	left, right models.SessionRecoveryBlock,
	request *models.AgentDeliveryReconstructionRequest,
) bool {
	return left.ID == right.ID && left.SessionID == right.SessionID && left.IncarnationID == right.IncarnationID &&
		left.ExpectedGeneration == right.ExpectedGeneration && left.Reason == right.Reason &&
		left.State == right.State && left.ConsumerReference == right.ConsumerReference &&
		reconstructionBlockBindingMatches(left, right, request) && reconstructionBlockLifecycleMatches(left, right, request)
}

func reconstructionBlockBindingMatches(left, right models.SessionRecoveryBlock, request *models.AgentDeliveryReconstructionRequest) bool {
	return (left.DeliverySubmissionID == right.DeliverySubmissionID ||
		(right.DeliverySubmissionID == "" && left.DeliverySubmissionID == request.Submission.ID)) &&
		(left.DeliveryStreamID == right.DeliveryStreamID ||
			(right.DeliveryStreamID == "" && left.DeliveryStreamID == request.Recovery.StreamID)) &&
		left.DeliverySequence == right.DeliverySequence && left.DeliveryTurnID == right.DeliveryTurnID &&
		(left.DeliveryOutcome == right.DeliveryOutcome || (right.DeliveryOutcome == "" && left.DeliveryOutcome == reconstructionUnknownOutcome))
}

func reconstructionBlockLifecycleMatches(left, right models.SessionRecoveryBlock, request *models.AgentDeliveryReconstructionRequest) bool {
	return left.AuthorizedAction == right.AuthorizedAction && left.CreatedAt.Equal(right.CreatedAt) &&
		(left.UpdatedAt.Equal(right.UpdatedAt) || (right.DeliverySubmissionID == "" && left.DeliverySubmissionID == request.Submission.ID)) &&
		((left.ResolvedAt == nil && right.ResolvedAt == nil) || (left.ResolvedAt != nil && right.ResolvedAt != nil && left.ResolvedAt.Equal(*right.ResolvedAt)))
}

func (r *Repository) validateReconstructionCursorTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) error {
	query := `SELECT session_id, incarnation_id, harness_generation, received_sequence,
		projected_sequence, remote_high_water, updated_at FROM agent_delivery_cursors WHERE stream_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var current models.AgentDeliveryCursor
	current.StreamID = request.Recovery.StreamID
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), request.Recovery.StreamID).Scan(
		&current.SessionID, &current.IncarnationID, &current.HarnessGeneration, &current.ReceivedSequence,
		&current.ProjectedSequence, &current.RemoteHighWater, &current.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if request.ExpectedCursor == nil && request.SourceStreamFirstRetained == 1 {
			return nil
		}
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	if err != nil {
		return err
	}
	if request.ExpectedCursor == nil || !sameReconstructionCursor(current, *request.ExpectedCursor) {
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return nil
}

func sameReconstructionCursor(left, right models.AgentDeliveryCursor) bool {
	return left.SessionID == right.SessionID && left.IncarnationID == right.IncarnationID &&
		left.HarnessGeneration == right.HarnessGeneration && left.StreamID == right.StreamID &&
		left.ReceivedSequence == right.ReceivedSequence && left.ProjectedSequence == right.ProjectedSequence &&
		left.RemoteHighWater == right.RemoteHighWater && left.UpdatedAt.Equal(right.UpdatedAt)
}

func (r *Repository) lockReconstructionSubmissionsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (map[string]models.AgentDeliverySubmission, error) {
	query := `SELECT id, session_id, incarnation_id, harness_generation, owner_generation,
		dispatch_attempt_id, payload_hash, payload, state, outcome, created_at, updated_at
		FROM agent_delivery_submissions WHERE session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	submissions := make(map[string]models.AgentDeliverySubmission)
	for rows.Next() {
		var submission models.AgentDeliverySubmission
		if err := rows.Scan(&submission.ID, &submission.SessionID, &submission.IncarnationID,
			&submission.HarnessGeneration, &submission.OwnerGeneration, &submission.DispatchAttemptID,
			&submission.PayloadHash, &submission.Payload, &submission.State, &submission.Outcome,
			&submission.CreatedAt, &submission.UpdatedAt); err != nil {
			return nil, err
		}
		submissions[submission.ID] = submission
	}
	return submissions, rows.Err()
}

func reconstructionAlreadyCommitted(
	owner *agentDeliveryReconstructionOwner,
	submission models.AgentDeliverySubmission,
	block *models.SessionRecoveryBlock,
	request *models.AgentDeliveryReconstructionRequest,
) bool {
	if !owner.exists || block == nil || owner.current.Reconstruction == nil || owner.current.AgentExecutionID != request.Recovery.AgentExecutionID ||
		owner.current.PromptGeneration != request.Recovery.PromptGeneration || owner.current.OriginalRuntime != request.Recovery.OriginalRuntime ||
		owner.current.SubmissionID != request.Submission.ID || owner.current.StreamID != request.Recovery.StreamID ||
		owner.current.IncarnationID != request.IncarnationID || owner.current.HarnessGeneration != request.HarnessGeneration ||
		!reconstructionProvenanceMatches(*owner.current.Reconstruction, *request.Recovery.Reconstruction) ||
		block.DeliverySubmissionID != request.Submission.ID || block.DeliveryStreamID != request.Recovery.StreamID {
		return false
	}
	return reconstructionSubmissionMatches(submission, request.Submission)
}

func reconstructionSubmissionMatches(submission, expected models.AgentDeliverySubmission) bool {
	return submission.SessionID == expected.SessionID && submission.IncarnationID == expected.IncarnationID &&
		submission.HarnessGeneration == expected.HarnessGeneration && submission.OwnerGeneration == expected.OwnerGeneration &&
		submission.DispatchAttemptID == expected.DispatchAttemptID && submission.PayloadHash == expected.PayloadHash &&
		submission.State == expected.State && submission.Outcome == expected.Outcome &&
		string(submission.Payload) == string(expected.Payload) && submission.CreatedAt.Equal(expected.CreatedAt) &&
		submission.UpdatedAt.Equal(expected.UpdatedAt)
}

func reconstructionProvenanceMatches(left, right models.AgentDeliveryReconstructionProvenance) bool {
	return left.SourceSessionID == right.SourceSessionID && left.SourceIncarnationID == right.SourceIncarnationID &&
		left.SourceHarnessGeneration == right.SourceHarnessGeneration && left.SourceStreamID == right.SourceStreamID &&
		left.PayloadHash == right.PayloadHash && left.ObservedState == right.ObservedState &&
		left.SourceStreamHighWater == right.SourceStreamHighWater &&
		left.SourceStreamAcknowledged == right.SourceStreamAcknowledged &&
		left.SourceStreamFirstRetained == right.SourceStreamFirstRetained &&
		left.ProcessIdentityKnown == right.ProcessIdentityKnown &&
		((left.ProcessTerminated == nil && right.ProcessTerminated == nil) ||
			(left.ProcessTerminated != nil && right.ProcessTerminated != nil && *left.ProcessTerminated == *right.ProcessTerminated))
}

func commitIdempotentReconstruction(
	tx *sqlx.Tx,
	recovery models.AgentDeliveryRecovery,
	block models.SessionRecoveryBlock,
) (*models.AgentDeliveryReconstructionResult, error) {
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &models.AgentDeliveryReconstructionResult{Recovery: recovery, Block: block}, nil
}

func isUnresolvedReconstructionSubmission(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionPrepared || state == models.DeliverySubmissionAccepted ||
		state == models.DeliverySubmissionDispatching || state == models.DeliverySubmissionInterruptedUnknown
}

func (r *Repository) insertReconstructedSubmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
) error {
	submission := request.Submission
	result, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO agent_delivery_submissions
		(id, session_id, incarnation_id, harness_generation, owner_generation, dispatch_attempt_id,
		 payload_hash, payload, state, outcome, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`),
		submission.ID, submission.SessionID, submission.IncarnationID, submission.HarnessGeneration,
		submission.OwnerGeneration, submission.DispatchAttemptID, submission.PayloadHash, submission.Payload,
		submission.State, submission.Outcome, submission.CreatedAt, submission.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert reconstructed delivery submission: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != 1 {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	return nil
}

func (r *Repository) persistReconstructedRecoveryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
	owner *agentDeliveryReconstructionOwner,
	now time.Time,
) (models.AgentDeliveryRecovery, error) {
	recovery := request.Recovery
	recovery.Revision = 1
	if owner.exists {
		recovery.Revision = owner.current.Revision + 1
	}
	recovery.UpdatedAt = now
	provenance := *recovery.Reconstruction
	recovery.Reconstruction = &provenance
	recovery.Reconstruction.ReconstructedAt = now
	encodedRecovery, err := json.Marshal(recovery)
	if err != nil {
		return models.AgentDeliveryRecovery{}, fmt.Errorf("serialize reconstructed delivery recovery: %w", err)
	}
	owner.metadata[models.SessionMetaKeyAgentDeliveryRecovery] = encodedRecovery
	encodedMetadata, err := json.Marshal(owner.metadata)
	if err != nil {
		return models.AgentDeliveryRecovery{}, fmt.Errorf("serialize reconstructed session metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET metadata = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ?`),
		string(encodedMetadata), now, request.SessionID, request.TaskID, request.IncarnationID)
	if err != nil {
		return models.AgentDeliveryRecovery{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return models.AgentDeliveryRecovery{}, repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return recovery, nil
}

func (r *Repository) bindReconstructedBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	request *models.AgentDeliveryReconstructionRequest,
	block *models.SessionRecoveryBlock,
	updatedAt time.Time,
) (models.SessionRecoveryBlock, error) {
	query := `UPDATE session_recovery_blocks SET
		delivery_submission_id = ?, delivery_stream_id = ?, delivery_outcome = ?, updated_at = ?
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
		AND state = ? AND consumer_reference = 'agent_delivery'
		AND (delivery_submission_id = '' OR delivery_submission_id = ?)
		AND (delivery_stream_id = '' OR delivery_stream_id = ?)`
	result, err := tx.ExecContext(ctx, r.db.Rebind(query),
		request.Submission.ID, request.Recovery.StreamID, reconstructionUnknownOutcome, updatedAt,
		block.ID, request.SessionID, request.IncarnationID, request.HarnessGeneration,
		models.RecoveryBlockOpen, request.Submission.ID, request.Recovery.StreamID)
	if err != nil {
		return models.SessionRecoveryBlock{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return models.SessionRecoveryBlock{}, repoerrors.ErrAgentDeliveryReconstructionStale
	}
	block.DeliverySubmissionID = request.Submission.ID
	block.DeliveryStreamID = request.Recovery.StreamID
	block.DeliveryOutcome = reconstructionUnknownOutcome
	block.UpdatedAt = updatedAt
	return *block, nil
}

var _ interface {
	ReconstructAgentDeliverySubmission(context.Context, *models.AgentDeliveryReconstructionRequest) (*models.AgentDeliveryReconstructionResult, error)
} = (*Repository)(nil)
