package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) initSessionContinuitySchema() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS harness_session_generations (
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			generation BIGINT NOT NULL,
			predecessor_generation BIGINT NOT NULL DEFAULT 0,
			native_session_id TEXT NOT NULL,
			agent_type TEXT NOT NULL DEFAULT '',
			adapter_version TEXT NOT NULL DEFAULT '',
			original_workspace TEXT NOT NULL DEFAULT '',
			current_workspace TEXT NOT NULL DEFAULT '',
			native_state_reference TEXT NOT NULL DEFAULT '',
			creation_reason TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			committed_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, incarnation_id, generation),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_harness_generations_current
			ON harness_session_generations(session_id, incarnation_id, generation DESC);

		CREATE TABLE IF NOT EXISTS session_restore_attempts (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			expected_generation BIGINT NOT NULL,
			action TEXT NOT NULL,
			outcome TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			target_workspace TEXT NOT NULL DEFAULT '',
			authorized INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_restore_attempts_session
			ON session_restore_attempts(session_id, incarnation_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS session_continuation_snapshots (
			id TEXT PRIMARY KEY,
			attempt_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			target_generation BIGINT NOT NULL DEFAULT 0,
			submission_id TEXT NOT NULL DEFAULT '',
			source_message_id TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			byte_count INTEGER NOT NULL,
			omitted_messages INTEGER NOT NULL DEFAULT 0,
			truncated INTEGER NOT NULL DEFAULT 0,
			content_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			UNIQUE (attempt_id),
			FOREIGN KEY (attempt_id) REFERENCES session_restore_attempts(id) ON DELETE CASCADE,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_continuation_snapshots_session
			ON session_continuation_snapshots(session_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS session_recovery_blocks (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			expected_generation BIGINT NOT NULL,
			reason TEXT NOT NULL,
			state TEXT NOT NULL,
			consumer_reference TEXT NOT NULL DEFAULT '',
			authorized_action TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			UNIQUE (session_id, incarnation_id, expected_generation, reason),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_session_recovery_blocks_open
			ON session_recovery_blocks(session_id, incarnation_id, state);
	`)
	return err
}

func (r *Repository) CreateHarnessSessionGeneration(ctx context.Context, generation *models.HarnessSessionGeneration) error {
	if generation == nil {
		return fmt.Errorf("harness generation is required")
	}
	if generation.CreatedAt.IsZero() {
		generation.CreatedAt = time.Now().UTC()
	}
	if generation.CommittedAt.IsZero() {
		generation.CommittedAt = generation.CreatedAt
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO harness_session_generations
		(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
		 agent_type, adapter_version, original_workspace, current_workspace,
		 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		generation.SessionID, generation.IncarnationID, generation.Generation,
		generation.PredecessorGeneration, generation.NativeSessionID, generation.AgentType,
		generation.AdapterVersion, generation.OriginalWorkspace, generation.CurrentWorkspace,
		generation.NativeStateReference, generation.CreationReason, generation.CreatedAt,
		generation.CommittedAt)
	return err
}

func (r *Repository) GetCurrentHarnessSessionGeneration(ctx context.Context, sessionID, incarnationID string) (*models.HarnessSessionGeneration, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT session_id, incarnation_id, generation, predecessor_generation, native_session_id,
		       agent_type, adapter_version, original_workspace, current_workspace,
		       native_state_reference, creation_reason, created_at, committed_at
		FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`), sessionID, incarnationID)
	var generation models.HarnessSessionGeneration
	if err := row.Scan(
		&generation.SessionID, &generation.IncarnationID, &generation.Generation,
		&generation.PredecessorGeneration, &generation.NativeSessionID, &generation.AgentType,
		&generation.AdapterVersion, &generation.OriginalWorkspace, &generation.CurrentWorkspace,
		&generation.NativeStateReference, &generation.CreationReason, &generation.CreatedAt,
		&generation.CommittedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrTaskSessionNotFound
		}
		return nil, err
	}
	return &generation, nil
}

func (r *Repository) CommitHarnessSessionGeneration(ctx context.Context, generation *models.HarnessSessionGeneration, expectedGeneration int64) (bool, error) {
	if generation == nil {
		return false, fmt.Errorf("harness generation is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var current int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT generation FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`), generation.SessionID, generation.IncarnationID).Scan(&current)
	if err == sql.ErrNoRows {
		current = 0
	} else if err != nil {
		return false, err
	}
	if current != expectedGeneration {
		return false, nil
	}
	if err := insertGenerationTx(ctx, tx, r.db.Rebind, generation); err != nil {
		return false, err
	}
	// A context continuation publishes a replacement generation while the
	// operator's recovery block is still the admission fence. Move that fence
	// with the generation in the same transaction so a successful commit cannot
	// leave an open block stranded on the predecessor generation.
	if generation.Generation == expectedGeneration+1 {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE session_recovery_blocks
			SET expected_generation = ?, updated_at = ?
			WHERE session_id = ? AND incarnation_id = ?
			  AND expected_generation = ? AND state = ?`),
			generation.Generation, generation.CommittedAt,
			generation.SessionID, generation.IncarnationID,
			expectedGeneration, models.RecoveryBlockOpen); err != nil {
			return false, fmt.Errorf("move open recovery blocks with generation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func insertGenerationTx(ctx context.Context, tx *sql.Tx, rebind func(string) string, generation *models.HarnessSessionGeneration) error {
	if generation.CreatedAt.IsZero() {
		generation.CreatedAt = time.Now().UTC()
	}
	if generation.CommittedAt.IsZero() {
		generation.CommittedAt = generation.CreatedAt
	}
	_, err := tx.ExecContext(ctx, rebind(`
		INSERT INTO harness_session_generations
		(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
		 agent_type, adapter_version, original_workspace, current_workspace,
		 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		generation.SessionID, generation.IncarnationID, generation.Generation,
		generation.PredecessorGeneration, generation.NativeSessionID, generation.AgentType,
		generation.AdapterVersion, generation.OriginalWorkspace, generation.CurrentWorkspace,
		generation.NativeStateReference, generation.CreationReason, generation.CreatedAt,
		generation.CommittedAt)
	return err
}

func (r *Repository) CreateRestoreAttempt(ctx context.Context, attempt *models.RestoreAttempt) error {
	if attempt == nil {
		return fmt.Errorf("restore attempt is required")
	}
	if attempt.ID == "" {
		attempt.ID = uuid.NewString()
	}
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_restore_attempts
		(id, session_id, incarnation_id, expected_generation, action, outcome, reason,
		 target_workspace, authorized, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		attempt.ID, attempt.SessionID, attempt.IncarnationID, attempt.ExpectedGeneration,
		attempt.Action, attempt.Outcome, attempt.Reason, attempt.TargetWorkspace,
		boolToInt(attempt.Authorized), attempt.CreatedAt, attempt.CompletedAt)
	return err
}

func (r *Repository) CompleteRestoreAttempt(ctx context.Context, id, outcome string, completedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_restore_attempts SET outcome = ?, completed_at = ? WHERE id = ?`), outcome, completedAt, id)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) CreateContinuationSnapshot(ctx context.Context, snapshot *models.ContinuationSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("continuation snapshot is required")
	}
	if snapshot.ID == "" {
		snapshot.ID = uuid.NewString()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	if snapshot.ByteCount == 0 {
		snapshot.ByteCount = len([]byte(snapshot.Content))
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_continuation_snapshots
		(id, attempt_id, session_id, target_generation, submission_id, source_message_id, content, byte_count,
		 omitted_messages, truncated, content_hash, status, created_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		snapshot.ID, snapshot.AttemptID, snapshot.SessionID, snapshot.TargetGeneration, snapshot.SubmissionID, snapshot.SourceMessageID,
		snapshot.Content, snapshot.ByteCount, snapshot.OmittedMessages, boolToInt(snapshot.Truncated),
		snapshot.ContentHash, snapshot.Status, snapshot.CreatedAt, snapshot.ResolvedAt)
	return err
}

func (r *Repository) GetContinuationSnapshot(ctx context.Context, id string) (*models.ContinuationSnapshot, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, attempt_id, session_id, target_generation, submission_id, source_message_id, content, byte_count,
		       omitted_messages, truncated, content_hash, status, created_at, resolved_at
		FROM session_continuation_snapshots WHERE id = ?`), id)
	var snapshot models.ContinuationSnapshot
	var truncated int
	if err := row.Scan(&snapshot.ID, &snapshot.AttemptID, &snapshot.SessionID,
		&snapshot.TargetGeneration, &snapshot.SubmissionID, &snapshot.SourceMessageID, &snapshot.Content, &snapshot.ByteCount,
		&snapshot.OmittedMessages, &truncated, &snapshot.ContentHash, &snapshot.Status,
		&snapshot.CreatedAt, &snapshot.ResolvedAt); err != nil {
		return nil, err
	}
	snapshot.Truncated = truncated != 0
	return &snapshot, nil
}

func (r *Repository) CompleteContinuationSnapshot(ctx context.Context, id, status string, resolvedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_continuation_snapshots
		SET status = ?, resolved_at = ?
		WHERE id = ?`), status, resolvedAt, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) UpsertSessionRecoveryBlock(ctx context.Context, block *models.SessionRecoveryBlock) error {
	if block == nil {
		return fmt.Errorf("session recovery block is required")
	}
	if block.ID == "" {
		block.ID = uuid.NewString()
	}
	if block.CreatedAt.IsZero() {
		block.CreatedAt = time.Now().UTC()
	}
	block.UpdatedAt = time.Now().UTC()
	query := `
		INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state,
		 consumer_reference, authorized_action, created_at, updated_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, incarnation_id, expected_generation, reason) DO UPDATE SET
		 state = excluded.state,
			consumer_reference = excluded.consumer_reference,
			authorized_action = excluded.authorized_action,
			updated_at = excluded.updated_at,
			resolved_at = excluded.resolved_at
		RETURNING id`
	var canonicalID string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(query), block.ID, block.SessionID,
		block.IncarnationID, block.ExpectedGeneration, block.Reason, block.State,
		block.ConsumerReference, block.AuthorizedAction, block.CreatedAt, block.UpdatedAt,
		block.ResolvedAt).Scan(&canonicalID)
	if err == nil {
		block.ID = canonicalID
	}
	return err
}

func (r *Repository) GetOpenSessionRecoveryBlock(ctx context.Context, sessionID, incarnationID string, expectedGeneration int64) (*models.SessionRecoveryBlock, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		       consumer_reference, authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks
		WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ? AND state = ?
		ORDER BY created_at LIMIT 1`), sessionID, incarnationID, expectedGeneration, models.RecoveryBlockOpen)
	var block models.SessionRecoveryBlock
	if err := row.Scan(&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration,
		&block.Reason, &block.State, &block.ConsumerReference, &block.AuthorizedAction,
		&block.CreatedAt, &block.UpdatedAt, &block.ResolvedAt); err != nil {
		return nil, err
	}
	return &block, nil
}

// GetSessionRecoveryBlock returns a block by stable identity, including
// resolved blocks that autonomous consumers use to release their parked work.
func (r *Repository) GetSessionRecoveryBlock(ctx context.Context, id string) (*models.SessionRecoveryBlock, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, incarnation_id, expected_generation, reason, state,
		       consumer_reference, authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks WHERE id = ?`), id)
	var block models.SessionRecoveryBlock
	if err := row.Scan(&block.ID, &block.SessionID, &block.IncarnationID, &block.ExpectedGeneration,
		&block.Reason, &block.State, &block.ConsumerReference, &block.AuthorizedAction,
		&block.CreatedAt, &block.UpdatedAt, &block.ResolvedAt); err != nil {
		return nil, err
	}
	return &block, nil
}

func (r *Repository) ResolveSessionRecoveryBlock(ctx context.Context, id, action string, resolvedAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_recovery_blocks SET state = ?, authorized_action = ?, updated_at = ?, resolved_at = ?
		WHERE id = ? AND state = ?`), models.RecoveryBlockResolved, action, resolvedAt, resolvedAt, id, models.RecoveryBlockOpen)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
