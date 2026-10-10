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
	silentRestoreConsumerReference = "agent_delivery"
	silentRestoreActiveRouteState  = worktreeRepoStatusActive
)

func marshalSilentRestoreCheckpoint(checkpoint *models.SilentRestoreCheckpoint) (string, error) {
	if checkpoint == nil {
		return "", nil
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return "", fmt.Errorf("encode silent restore checkpoint: %w", err)
	}
	return string(encoded), nil
}

func unmarshalSilentRestoreCheckpoint(encoded string) (*models.SilentRestoreCheckpoint, error) {
	if encoded == "" {
		return nil, nil
	}
	var checkpoint models.SilentRestoreCheckpoint
	if err := json.Unmarshal([]byte(encoded), &checkpoint); err != nil {
		return nil, fmt.Errorf("decode silent restore checkpoint: %w", err)
	}
	return &checkpoint, nil
}

// ListSilentRestoreCandidates returns an ordered bounded page of eligible
// sessions fenced by an open delivery recovery block.
func (r *Repository) ListSilentRestoreCandidates(
	ctx context.Context,
	afterSessionID string,
	limit int,
) ([]models.SilentRestoreCandidate, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("silent restore candidate limit must be between 1 and 200")
	}
	sessionID := dialect.ByteOrderedText(r.db.DriverName(), "ts.id")
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT ts.task_id, ts.id, t.workspace_id, ts.state, ts.metadata
		FROM task_sessions ts
		JOIN tasks t ON t.id = ts.task_id
		WHERE %s > ?
		  AND t.archived_at IS NULL
		  AND ts.state IN (?, ?, ?, ?)
		  AND EXISTS (
			SELECT 1 FROM session_recovery_blocks b
			WHERE b.session_id = ts.id
			  AND b.incarnation_id = ts.queue_incarnation_id
			  AND b.state = ?
			  AND b.consumer_reference = 'agent_delivery'
		  )
		ORDER BY %s
		LIMIT ?`, sessionID, sessionID)),
		afterSessionID,
		models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateFailed,
		models.RecoveryBlockOpen, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]models.SilentRestoreCandidate, 0, limit)
	for rows.Next() {
		var candidate models.SilentRestoreCandidate
		var metadata sql.NullString
		if err := rows.Scan(&candidate.TaskID, &candidate.SessionID, &candidate.WorkspaceID, &candidate.State, &metadata); err != nil {
			return nil, err
		}
		candidate.Metadata = make(map[string]interface{})
		if metadata.Valid && metadata.String != "" && metadata.String != jsonNull {
			if err := json.Unmarshal([]byte(metadata.String), &candidate.Metadata); err != nil {
				candidate.Metadata = nil
			}
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

// GetRestoreAttempt returns the attempt and its restore-only checkpoint.
func (r *Repository) GetRestoreAttempt(ctx context.Context, id string) (*models.RestoreAttempt, error) {
	return scanRestoreAttempt(r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, incarnation_id, expected_generation, action, outcome, reason,
		       target_workspace, authorized, created_at, completed_at, checkpoint_json
		FROM session_restore_attempts WHERE id = ?`), id))
}

func scanRestoreAttempt(row *sqlx.Row) (*models.RestoreAttempt, error) {
	attempt, _, err := scanRestoreAttemptWithJSON(row)
	return attempt, err
}

func scanRestoreAttemptWithJSON(row *sqlx.Row) (*models.RestoreAttempt, string, error) {
	var attempt models.RestoreAttempt
	var authorized int
	var checkpointJSON string
	if err := row.Scan(
		&attempt.ID, &attempt.SessionID, &attempt.IncarnationID, &attempt.ExpectedGeneration,
		&attempt.Action, &attempt.Outcome, &attempt.Reason, &attempt.TargetWorkspace,
		&authorized, &attempt.CreatedAt, &attempt.CompletedAt, &checkpointJSON,
	); err != nil {
		return nil, "", err
	}
	attempt.Authorized = authorized != 0
	checkpoint, err := unmarshalSilentRestoreCheckpoint(checkpointJSON)
	if err != nil {
		return nil, "", err
	}
	attempt.Checkpoint = checkpoint
	return &attempt, checkpointJSON, nil
}

// EnsureRestoreAttempt creates the stable restore attempt once and returns the
// existing row on retries. Reusing its ID for another source is a conflict.
func (r *Repository) EnsureRestoreAttempt(
	ctx context.Context,
	attempt *models.RestoreAttempt,
) (*models.RestoreAttempt, bool, error) {
	if err := validateSilentRestoreAttempt(attempt); err != nil {
		return nil, false, err
	}
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = r.nowUTC()
	}
	checkpointJSON, err := marshalSilentRestoreCheckpoint(attempt.Checkpoint)
	if err != nil {
		return nil, false, err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_restore_attempts
		(id, session_id, incarnation_id, expected_generation, action, outcome, reason,
		 target_workspace, authorized, checkpoint_json, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`),
		attempt.ID, attempt.SessionID, attempt.IncarnationID, attempt.ExpectedGeneration,
		attempt.Action, attempt.Outcome, attempt.Reason, attempt.TargetWorkspace,
		boolToInt(attempt.Authorized), checkpointJSON, attempt.CreatedAt, attempt.CompletedAt)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	stored, err := r.GetRestoreAttempt(ctx, attempt.ID)
	if err != nil {
		return nil, false, err
	}
	if stored.Action != attempt.Action || stored.SessionID != attempt.SessionID ||
		stored.IncarnationID != attempt.IncarnationID || stored.ExpectedGeneration != attempt.ExpectedGeneration ||
		stored.Checkpoint == nil || !sameSilentRestoreCheckpointSource(*stored.Checkpoint, *attempt.Checkpoint) {
		return nil, false, repoerrors.ErrSilentRestoreAttemptConflict
	}
	return stored, rows == 1, nil
}

func validateSilentRestoreAttempt(attempt *models.RestoreAttempt) error {
	if !validSilentRestoreAttemptEnvelope(attempt) {
		return fmt.Errorf("complete silent restore attempt identity is required")
	}
	checkpoint := attempt.Checkpoint
	if !validSilentRestoreAttemptCheckpoint(attempt, checkpoint) {
		return fmt.Errorf("silent restore attempt checkpoint does not match its source identity")
	}
	return nil
}

func validSilentRestoreAttemptEnvelope(attempt *models.RestoreAttempt) bool {
	return attempt != nil && attempt.ID != "" && attempt.SessionID != "" && attempt.IncarnationID != "" &&
		attempt.Action == models.SilentRestoreAction && attempt.Outcome == models.SilentRestoreOutcomePending &&
		attempt.ExpectedGeneration > 0 && attempt.Checkpoint != nil
}

func validSilentRestoreAttemptCheckpoint(
	attempt *models.RestoreAttempt,
	checkpoint *models.SilentRestoreCheckpoint,
) bool {
	return checkpoint.Version == 1 && checkpoint.Stage == models.SilentRestoreStagePrepared &&
		checkpoint.CandidateExecutionID == "" && validSilentRestoreCheckpointRecovery(attempt, checkpoint) &&
		validSilentRestoreCheckpointGeneration(attempt, checkpoint) &&
		validSilentRestoreCheckpointBlock(attempt, checkpoint) &&
		attempt.TargetWorkspace == checkpoint.WorkspaceID && checkpoint.WorkspaceID != "" && checkpoint.TaskID != ""
}

func validSilentRestoreCheckpointRecovery(
	attempt *models.RestoreAttempt,
	checkpoint *models.SilentRestoreCheckpoint,
) bool {
	recovery := checkpoint.SourceRecovery
	return recovery.SessionID == attempt.SessionID && recovery.IncarnationID == attempt.IncarnationID &&
		recovery.HarnessGeneration == attempt.ExpectedGeneration && recovery.SubmissionID != "" &&
		recovery.StreamID != "" && (recovery.Phase == models.AgentDeliveryRecoveryUncertain ||
		recovery.Phase == models.AgentDeliveryRecoveryReconnecting)
}

func validSilentRestoreCheckpointGeneration(
	attempt *models.RestoreAttempt,
	checkpoint *models.SilentRestoreCheckpoint,
) bool {
	generation := checkpoint.SourceGeneration
	return generation.SessionID == attempt.SessionID && generation.IncarnationID == attempt.IncarnationID &&
		generation.Generation == attempt.ExpectedGeneration && generation.NativeSessionID != ""
}

func validSilentRestoreCheckpointBlock(
	attempt *models.RestoreAttempt,
	checkpoint *models.SilentRestoreCheckpoint,
) bool {
	block := checkpoint.SourceBlock
	return block.ID != "" && block.SessionID == attempt.SessionID &&
		block.IncarnationID == attempt.IncarnationID && block.ExpectedGeneration == attempt.ExpectedGeneration &&
		block.State == models.RecoveryBlockOpen && block.ConsumerReference == silentRestoreConsumerReference &&
		block.DeliverySubmissionID == checkpoint.SourceRecovery.SubmissionID &&
		block.DeliveryStreamID == checkpoint.SourceRecovery.StreamID
}

// CompareAndSwapSilentRestoreCheckpoint advances the candidate lifecycle
// without changing its pinned source identity.
func (r *Repository) CompareAndSwapSilentRestoreCheckpoint(
	ctx context.Context,
	id string,
	expected models.SilentRestoreCheckpoint,
	next models.SilentRestoreCheckpoint,
) (bool, error) {
	query := `SELECT id, session_id, incarnation_id, expected_generation, action, outcome, reason,
		target_workspace, authorized, created_at, completed_at, checkpoint_json
		FROM session_restore_attempts WHERE id = ?`
	attempt, checkpointJSON, err := scanRestoreAttemptWithJSON(
		r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), id),
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if attempt.Action != models.SilentRestoreAction || attempt.Checkpoint == nil ||
		!sameSilentRestoreCheckpoint(*attempt.Checkpoint, expected) || !validSilentRestoreCheckpointTransition(expected, next) {
		return false, nil
	}
	nextJSON, err := marshalSilentRestoreCheckpoint(&next)
	if err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_restore_attempts SET checkpoint_json = ?
		WHERE id = ? AND action = ? AND session_id = ? AND incarnation_id = ?
		  AND expected_generation = ? AND checkpoint_json = ?`),
		nextJSON, id, models.SilentRestoreAction, attempt.SessionID, attempt.IncarnationID,
		attempt.ExpectedGeneration, checkpointJSON)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func validSilentRestoreCheckpointTransition(expected, next models.SilentRestoreCheckpoint) bool {
	if !sameSilentRestoreCheckpointSource(expected, next) || next.CandidateExecutionID == "" {
		return false
	}
	switch expected.Stage {
	case models.SilentRestoreStagePrepared:
		return expected.CandidateExecutionID == "" && next.Stage == models.SilentRestoreStageCandidateAllocated
	case models.SilentRestoreStageCandidateAllocated:
		return expected.CandidateExecutionID != "" && next.Stage == models.SilentRestoreStageCandidateLaunching &&
			next.CandidateExecutionID == expected.CandidateExecutionID
	case models.SilentRestoreStageCandidateLaunching:
		return expected.CandidateExecutionID != "" && next.Stage == models.SilentRestoreStageCandidateDead &&
			next.CandidateExecutionID == expected.CandidateExecutionID
	case models.SilentRestoreStageCandidateDead:
		return expected.CandidateExecutionID != "" && next.Stage == models.SilentRestoreStageCandidateAllocated &&
			next.CandidateExecutionID != expected.CandidateExecutionID
	default:
		return false
	}
}

func sameSilentRestoreCheckpointSource(a, b models.SilentRestoreCheckpoint) bool {
	return a.Version == b.Version && sameAgentDeliveryRecovery(a.SourceRecovery, b.SourceRecovery) &&
		sameHarnessSessionGeneration(a.SourceGeneration, b.SourceGeneration) &&
		sameSessionRecoveryBlock(a.SourceBlock, b.SourceBlock) && a.TaskID == b.TaskID &&
		a.WorkspaceID == b.WorkspaceID && a.WorkspaceOwnerID == b.WorkspaceOwnerID &&
		a.WorkspaceOrgID == b.WorkspaceOrgID
}

func sameSilentRestoreCheckpoint(a, b models.SilentRestoreCheckpoint) bool {
	return sameSilentRestoreCheckpointSource(a, b) && a.Stage == b.Stage &&
		a.CandidateExecutionID == b.CandidateExecutionID
}

func sameAgentDeliveryRecovery(a, b models.AgentDeliveryRecovery) bool {
	return a.OriginalRuntime == b.OriginalRuntime && a.Phase == b.Phase && a.Revision == b.Revision &&
		a.SessionID == b.SessionID && a.AgentExecutionID == b.AgentExecutionID &&
		a.SubmissionID == b.SubmissionID && a.StreamID == b.StreamID &&
		a.IncarnationID == b.IncarnationID && a.HarnessGeneration == b.HarnessGeneration &&
		a.PromptGeneration == b.PromptGeneration && a.Message == b.Message && a.UpdatedAt.Equal(b.UpdatedAt)
}

func sameHarnessSessionGeneration(a, b models.HarnessSessionGeneration) bool {
	return a.SessionID == b.SessionID && a.IncarnationID == b.IncarnationID &&
		a.Generation == b.Generation && a.PredecessorGeneration == b.PredecessorGeneration &&
		a.NativeSessionID == b.NativeSessionID && a.AgentType == b.AgentType &&
		a.AdapterVersion == b.AdapterVersion && a.OriginalWorkspace == b.OriginalWorkspace &&
		a.CurrentWorkspace == b.CurrentWorkspace && a.NativeStateReference == b.NativeStateReference &&
		a.CreationReason == b.CreationReason && a.CreatedAt.Equal(b.CreatedAt) && a.CommittedAt.Equal(b.CommittedAt)
}

func sameSessionRecoveryBlock(a, b models.SessionRecoveryBlock) bool {
	return sameSessionRecoveryBlockIdentity(a, b) && sameSessionRecoveryBlockDelivery(a, b) &&
		a.AuthorizedAction == b.AuthorizedAction && a.CreatedAt.Equal(b.CreatedAt) &&
		a.UpdatedAt.Equal(b.UpdatedAt) && sameOptionalTime(a.ResolvedAt, b.ResolvedAt)
}

func sameSessionRecoveryBlockIdentity(a, b models.SessionRecoveryBlock) bool {
	return a.ID == b.ID && a.SessionID == b.SessionID && a.IncarnationID == b.IncarnationID &&
		a.ExpectedGeneration == b.ExpectedGeneration && a.Reason == b.Reason && a.State == b.State
}

func sameSessionRecoveryBlockDelivery(a, b models.SessionRecoveryBlock) bool {
	return a.ConsumerReference == b.ConsumerReference && a.DeliverySubmissionID == b.DeliverySubmissionID &&
		a.DeliveryStreamID == b.DeliveryStreamID && a.DeliverySequence == b.DeliverySequence &&
		a.DeliveryTurnID == b.DeliveryTurnID && a.DeliveryOutcome == b.DeliveryOutcome
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// CommitSilentRestore atomically publishes the restored native generation,
// recovery metadata, exact source-block resolution, and terminal checkpoint.
func (r *Repository) CommitSilentRestore(ctx context.Context, commit *models.SilentRestoreCommit) (bool, error) {
	if !validSilentRestoreCommit(commit) {
		return false, fmt.Errorf("complete silent restore commit identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), commit.Checkpoint.SourceRecovery.SessionID); err != nil {
		return false, err
	}
	evidence, valid, err := validateSilentRestoreCommitTx(ctx, tx, r.db.DriverName(), commit)
	if err != nil || !valid {
		return false, err
	}

	now := commit.CompletedAt
	if now.IsZero() {
		now = r.nowUTC()
	}
	generation := commit.Generation
	generation.CreatedAt = now
	generation.CommittedAt = now
	if err := insertGenerationTx(ctx, tx.Tx, r.db.Rebind, &generation); err != nil {
		return false, err
	}
	if valid, err := writeSilentRestoreRecoveryTx(ctx, tx, r.db.Rebind, evidence.session, evidence.metadataRaw, now); err != nil || !valid {
		return false, err
	}
	if valid, err := resolveSilentRestoreBlockTx(ctx, tx, r.db.Rebind, commit.Checkpoint, now); err != nil || !valid {
		return false, err
	}
	if valid, err := completeSilentRestoreAttemptTx(
		ctx, tx, r.db.Rebind, commit.AttemptID, evidence.attempt, evidence.checkpointJSON, commit.Checkpoint, now,
	); err != nil || !valid {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type silentRestoreCommitEvidence struct {
	attempt        *models.RestoreAttempt
	checkpointJSON string
	session        *silentRestoreSessionRow
	metadataRaw    sql.NullString
}

func validateSilentRestoreCommitTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	commit *models.SilentRestoreCommit,
) (*silentRestoreCommitEvidence, bool, error) {
	attempt, checkpointJSON, err := readSilentRestoreAttemptCheckpointTx(ctx, tx, driver, commit.AttemptID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	checkpoint := commit.Checkpoint
	if !validSilentRestoreCommitAttempt(attempt, checkpoint) {
		return nil, false, nil
	}
	session, metadataRaw, valid, err := validateSilentRestoreCommitSourceTx(ctx, tx, driver, commit)
	if err != nil || !valid {
		return nil, false, err
	}
	valid, err = validateSilentRestoreCommitRecordsTx(ctx, tx, driver, commit.Checkpoint)
	if err != nil || !valid {
		return nil, false, err
	}
	return &silentRestoreCommitEvidence{
		attempt: attempt, checkpointJSON: checkpointJSON, session: session, metadataRaw: metadataRaw,
	}, true, nil
}

func validSilentRestoreCommitAttempt(
	attempt *models.RestoreAttempt,
	checkpoint models.SilentRestoreCheckpoint,
) bool {
	return attempt.Action == models.SilentRestoreAction && attempt.Outcome == models.SilentRestoreOutcomePending &&
		attempt.SessionID == checkpoint.SourceRecovery.SessionID &&
		attempt.IncarnationID == checkpoint.SourceRecovery.IncarnationID &&
		attempt.ExpectedGeneration == checkpoint.SourceGeneration.Generation && attempt.Checkpoint != nil &&
		sameSilentRestoreCheckpoint(*attempt.Checkpoint, checkpoint) &&
		checkpoint.Stage == models.SilentRestoreStageCandidateLaunching && checkpoint.CandidateExecutionID != ""
}

func validateSilentRestoreCommitSourceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	commit *models.SilentRestoreCommit,
) (*silentRestoreSessionRow, sql.NullString, bool, error) {
	checkpoint := commit.Checkpoint
	session, metadataRaw, valid, err := readSilentRestoreSessionTx(ctx, tx, driver, checkpoint)
	if err != nil || !valid {
		return nil, metadataRaw, false, err
	}
	if _, valid, err = readSilentRestoreWorkspaceTx(ctx, tx, driver, checkpoint); err != nil || !valid {
		return nil, metadataRaw, false, err
	}
	currentGeneration, err := readSilentRestoreGenerationTx(
		ctx, tx, driver, checkpoint.SourceGeneration.SessionID, checkpoint.SourceGeneration.IncarnationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, metadataRaw, false, nil
	}
	if err != nil {
		return nil, metadataRaw, false, err
	}
	if !sameHarnessSessionGeneration(currentGeneration, checkpoint.SourceGeneration) ||
		!validSilentRestoreSuccessor(checkpoint, commit.Generation) {
		return nil, metadataRaw, false, nil
	}
	return session, metadataRaw, true, nil
}

func validateSilentRestoreCommitRecordsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (bool, error) {
	for _, validate := range []func(context.Context, *sqlx.Tx, string, models.SilentRestoreCheckpoint) (bool, error){
		validSilentRestoreBlockTx,
		validSilentRestoreCandidateExecutionTx,
		validSilentRestoreSubmissionTx,
	} {
		valid, err := validate(ctx, tx, driver, checkpoint)
		if err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

func validSilentRestoreCommit(commit *models.SilentRestoreCommit) bool {
	return commit != nil && commit.AttemptID != "" &&
		commit.Checkpoint.SourceRecovery.SessionID != "" &&
		commit.Checkpoint.SourceGeneration.Generation > 0 &&
		commit.Checkpoint.CandidateExecutionID != ""
}

func validSilentRestoreSuccessor(
	checkpoint models.SilentRestoreCheckpoint,
	generation models.HarnessSessionGeneration,
) bool {
	source := checkpoint.SourceGeneration
	return generation.SessionID == source.SessionID && generation.IncarnationID == source.IncarnationID &&
		generation.Generation == source.Generation+1 && generation.PredecessorGeneration == source.Generation &&
		generation.NativeSessionID == source.NativeSessionID
}

func readSilentRestoreAttemptCheckpointTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	id string,
) (*models.RestoreAttempt, string, error) {
	query := `SELECT id, session_id, incarnation_id, expected_generation, action, outcome, reason,
		target_workspace, authorized, created_at, completed_at, checkpoint_json
		FROM session_restore_attempts WHERE id = ?`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	return scanRestoreAttemptWithJSON(tx.QueryRowxContext(ctx, tx.Rebind(query), id))
}

type silentRestoreSessionRow struct {
	taskID   string
	state    models.TaskSessionState
	metadata map[string]json.RawMessage
	current  models.AgentDeliveryRecovery
}

func readSilentRestoreSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (*silentRestoreSessionRow, sql.NullString, bool, error) {
	query := `SELECT task_id, queue_incarnation_id, state, route_state, COALESCE(is_passthrough, 0), metadata
		FROM task_sessions WHERE id = ?`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	var taskID, incarnationID, routeState string
	var state models.TaskSessionState
	var passthrough int
	var metadataRaw sql.NullString
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), checkpoint.SourceRecovery.SessionID).Scan(
		&taskID, &incarnationID, &state, &routeState, &passthrough, &metadataRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, metadataRaw, false, nil
		}
		return nil, metadataRaw, false, err
	}
	if !validSilentRestoreSessionRow(taskID, incarnationID, state, passthrough, routeState, checkpoint) {
		return nil, metadataRaw, false, nil
	}
	metadata, err := decodeAgentDeliveryMetadata(metadataRaw)
	if err != nil {
		return nil, metadataRaw, false, err
	}
	recovery, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil {
		return nil, metadataRaw, false, err
	}
	if !validSilentRestoreSessionRecovery(recovery, exists, checkpoint) {
		return nil, metadataRaw, false, nil
	}
	return &silentRestoreSessionRow{taskID: taskID, state: state, metadata: metadata, current: recovery}, metadataRaw, true, nil
}

func validSilentRestoreSessionRow(
	taskID, incarnationID string,
	state models.TaskSessionState,
	passthrough int,
	routeState string,
	checkpoint models.SilentRestoreCheckpoint,
) bool {
	return taskID == checkpoint.TaskID && incarnationID == checkpoint.SourceRecovery.IncarnationID &&
		silentRestoreSessionState(state) && passthrough == 0 &&
		(routeState == "" || routeState == silentRestoreActiveRouteState)
}

func validSilentRestoreSessionRecovery(
	recovery models.AgentDeliveryRecovery,
	exists bool,
	checkpoint models.SilentRestoreCheckpoint,
) bool {
	return exists && sameAgentDeliveryRecovery(recovery, checkpoint.SourceRecovery) &&
		(recovery.Phase == models.AgentDeliveryRecoveryUncertain ||
			recovery.Phase == models.AgentDeliveryRecoveryReconnecting)
}

func silentRestoreSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateFailed:
		return true
	default:
		return false
	}
}

type silentRestoreWorkspaceRow struct {
	taskID, workspaceID, origin, ownerID, orgID string
}

func readSilentRestoreWorkspaceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (*silentRestoreWorkspaceRow, bool, error) {
	query := `SELECT t.id, t.workspace_id, t.archived_at, t.origin, w.owner_id, w.org_id
		FROM tasks t JOIN workspaces w ON w.id = t.workspace_id WHERE t.id = ?`
	if dialect.IsPostgres(driver) {
		query += ` FOR UPDATE OF t, w`
	}
	var row silentRestoreWorkspaceRow
	var archivedAt sql.NullTime
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), checkpoint.TaskID).Scan(
		&row.taskID, &row.workspaceID, &archivedAt, &row.origin, &row.ownerID, &row.orgID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if archivedAt.Valid || row.taskID != checkpoint.TaskID || row.workspaceID != checkpoint.WorkspaceID ||
		models.IsAutomationTaskOrigin(row.origin) ||
		row.ownerID != checkpoint.WorkspaceOwnerID || row.orgID != checkpoint.WorkspaceOrgID {
		return nil, false, nil
	}
	return &row, true, nil
}

func readSilentRestoreGenerationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver, sessionID, incarnationID string,
) (models.HarnessSessionGeneration, error) {
	query := `SELECT session_id, incarnation_id, generation, predecessor_generation, native_session_id,
		agent_type, adapter_version, original_workspace, current_workspace, native_state_reference,
		creation_reason, created_at, committed_at
		FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	var generation models.HarnessSessionGeneration
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), sessionID, incarnationID).Scan(
		&generation.SessionID, &generation.IncarnationID, &generation.Generation,
		&generation.PredecessorGeneration, &generation.NativeSessionID, &generation.AgentType,
		&generation.AdapterVersion, &generation.OriginalWorkspace, &generation.CurrentWorkspace,
		&generation.NativeStateReference, &generation.CreationReason, &generation.CreatedAt,
		&generation.CommittedAt,
	)
	return generation, err
}

func validSilentRestoreBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (bool, error) {
	var count int
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT COUNT(*) FROM session_recovery_blocks
		WHERE session_id = ? AND incarnation_id = ? AND state = ?`),
		checkpoint.SourceRecovery.SessionID, checkpoint.SourceRecovery.IncarnationID,
		models.RecoveryBlockOpen).Scan(&count); err != nil || count != 1 {
		return false, err
	}
	query := `SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		consumer_reference, delivery_submission_id, delivery_stream_id, delivery_sequence,
		delivery_turn_id, delivery_outcome, authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks WHERE id = ?`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	var block models.SessionRecoveryBlock
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), checkpoint.SourceBlock.ID).Scan(
		&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration,
		&block.Reason, &block.State, &block.ConsumerReference, &block.DeliverySubmissionID,
		&block.DeliveryStreamID, &block.DeliverySequence, &block.DeliveryTurnID,
		&block.DeliveryOutcome, &block.AuthorizedAction, &block.CreatedAt, &block.UpdatedAt,
		&block.ResolvedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return sameSessionRecoveryBlock(block, checkpoint.SourceBlock) &&
		block.State == models.RecoveryBlockOpen && block.ConsumerReference == silentRestoreConsumerReference &&
		block.DeliverySubmissionID == checkpoint.SourceRecovery.SubmissionID &&
		block.DeliveryStreamID == checkpoint.SourceRecovery.StreamID &&
		block.ExpectedGeneration == checkpoint.SourceRecovery.HarnessGeneration, nil
}

func validSilentRestoreCandidateExecutionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (bool, error) {
	query := `SELECT session_id FROM executors_running
		WHERE session_id = ? AND task_id = ? AND agent_execution_id = ?`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	var sessionID string
	err := tx.QueryRowxContext(ctx, tx.Rebind(query),
		checkpoint.SourceRecovery.SessionID, checkpoint.TaskID, checkpoint.CandidateExecutionID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && sessionID == checkpoint.SourceRecovery.SessionID, err
}

func validSilentRestoreSubmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	driver string,
	checkpoint models.SilentRestoreCheckpoint,
) (bool, error) {
	query := `SELECT state FROM agent_delivery_submissions
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?`
	if dialect.IsPostgres(driver) {
		query += forUpdateClause
	}
	var state models.DeliverySubmissionState
	err := tx.QueryRowxContext(ctx, tx.Rebind(query),
		checkpoint.SourceRecovery.SubmissionID, checkpoint.SourceRecovery.SessionID,
		checkpoint.SourceRecovery.IncarnationID, checkpoint.SourceRecovery.HarnessGeneration).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && !isTerminalAgentDeliverySubmission(state), err
}

func writeSilentRestoreRecoveryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	session *silentRestoreSessionRow,
	metadataRaw sql.NullString,
	completedAt time.Time,
) (bool, error) {
	recovery := session.current
	recovery.Phase = models.AgentDeliveryRecoveryRestored
	recovery.Revision++
	recovery.UpdatedAt = completedAt
	encodedRecovery, err := json.Marshal(recovery)
	if err != nil {
		return false, err
	}
	clearSilentRestoreUncertainError(session.metadata, session.current)
	session.metadata[models.SessionMetaKeyAgentDeliveryRecovery] = encodedRecovery
	encodedMetadata, err := json.Marshal(session.metadata)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, rebind(`UPDATE task_sessions SET metadata = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ? AND state = ? AND COALESCE(metadata, '') = ?`),
		string(encodedMetadata), completedAt, recovery.SessionID, session.taskID, recovery.IncarnationID,
		session.state, nullableMetadataValue(metadataRaw))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

const durableDeliveryUncertainErrorCode = "DURABLE_DELIVERY_UNCERTAIN"

func clearSilentRestoreUncertainError(metadata map[string]json.RawMessage, recovery models.AgentDeliveryRecovery) {
	raw, exists := metadata[models.SessionMetaKeyLastAgentError]
	if !exists {
		return
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return
	}
	lastError, exists := models.LoadLastAgentError(map[string]interface{}{
		models.SessionMetaKeyLastAgentError: decoded,
	})
	if !exists || lastError.Code != durableDeliveryUncertainErrorCode ||
		lastError.Details != recovery.SubmissionID ||
		(lastError.AgentExecutionID != recovery.AgentExecutionID && lastError.ExecutionID != recovery.AgentExecutionID) {
		return
	}
	delete(metadata, models.SessionMetaKeyLastAgentError)
}

func resolveSilentRestoreBlockTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	checkpoint models.SilentRestoreCheckpoint,
	completedAt time.Time,
) (bool, error) {
	block := checkpoint.SourceBlock
	result, err := tx.ExecContext(ctx, rebind(`UPDATE session_recovery_blocks
		SET state = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
		  AND reason = ? AND state = ? AND consumer_reference = ?
		  AND delivery_submission_id = ? AND delivery_stream_id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM session_recovery_blocks other
			WHERE other.session_id = ? AND other.incarnation_id = ?
			  AND other.state = ? AND other.id != ?
		  )`),
		models.RecoveryBlockResolved, models.SilentRestoreAction, completedAt, completedAt,
		block.ID, block.SessionID, block.IncarnationID, block.ExpectedGeneration, block.Reason,
		models.RecoveryBlockOpen, silentRestoreConsumerReference, checkpoint.SourceRecovery.SubmissionID,
		checkpoint.SourceRecovery.StreamID, block.SessionID, block.IncarnationID,
		models.RecoveryBlockOpen, block.ID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func completeSilentRestoreAttemptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	id string,
	attempt *models.RestoreAttempt,
	checkpointJSON string,
	checkpoint models.SilentRestoreCheckpoint,
	completedAt time.Time,
) (bool, error) {
	terminal := checkpoint
	terminal.Stage = models.SilentRestoreStageTerminal
	terminalJSON, err := marshalSilentRestoreCheckpoint(&terminal)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, rebind(`UPDATE session_restore_attempts
		SET outcome = ?, completed_at = ?, checkpoint_json = ?
		WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation = ?
		  AND action = ? AND outcome = ? AND checkpoint_json = ?`),
		models.SilentRestoreOutcomeRestored, completedAt, terminalJSON, id,
		attempt.SessionID, attempt.IncarnationID, attempt.ExpectedGeneration,
		models.SilentRestoreAction, models.SilentRestoreOutcomePending, checkpointJSON)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
