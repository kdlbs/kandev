// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

type taskSessionExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// Turn operations

// CreateTurn creates a new turn
func (r *Repository) CreateTurn(ctx context.Context, turn *models.Turn) error {
	if turn.ID == "" {
		turn.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if turn.StartedAt.IsZero() {
		turn.StartedAt = now
	}
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = now
	}
	turn.UpdatedAt = now

	metadataJSON := "{}"
	if turn.Metadata != nil {
		metadataBytes, err := json.Marshal(turn.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize turn metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), turn.ID, turn.TaskSessionID, turn.TaskID, turn.StartedAt, turn.CompletedAt, metadataJSON, turn.CreatedAt, turn.UpdatedAt)

	return err
}

func scanTurnRow(row *sql.Row) (*models.Turn, error) {
	turn := &models.Turn{}
	var metadataJSON string
	var completedAt sql.NullTime
	err := row.Scan(&turn.ID, &turn.TaskSessionID, &turn.TaskID, &turn.StartedAt, &completedAt, &metadataJSON, &turn.CreatedAt, &turn.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		turn.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &turn.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize turn metadata: %w", err)
		}
	}
	return turn, nil
}

// GetTurn retrieves a turn by ID
func (r *Repository) GetTurn(ctx context.Context, id string) (*models.Turn, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns WHERE id = ?
	`), id)
	return scanTurnRow(row)
}

// GetActiveTurnBySessionID gets the currently active (non-completed) turn for a session
func (r *Repository) GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns
		WHERE task_session_id = ? AND completed_at IS NULL
		ORDER BY started_at DESC LIMIT 1
	`), sessionID)
	return scanTurnRow(row)
}

// UpdateTurn updates an existing turn
func (r *Repository) UpdateTurn(ctx context.Context, turn *models.Turn) error {
	turn.UpdatedAt = time.Now().UTC()

	metadataJSON := "{}"
	if turn.Metadata != nil {
		metadataBytes, err := json.Marshal(turn.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize turn metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_turns
		SET completed_at = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`), turn.CompletedAt, metadataJSON, turn.UpdatedAt, turn.ID)

	return err
}

// CompleteTurn marks a turn as completed with the current time
func (r *Repository) CompleteTurn(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_turns
		SET completed_at = ?, updated_at = ?
		WHERE id = ?
	`), now, now, id)
	return err
}

// AbandonTurn marks a turn as completed with completed_at = started_at, giving it
// zero duration. Used when a turn was orphaned by an interruption (backend
// restart, agent crash) and the previous "running" window was not real work —
// recording `now` would inflate analytics and the UI's last-turn duration with
// hours of dead time.
func (r *Repository) AbandonTurn(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_turns
		SET completed_at = started_at, updated_at = ?
		WHERE id = ? AND completed_at IS NULL
	`), now, id)
	return err
}

// ListTurnsBySession returns all turns for a session ordered by start time
func (r *Repository) ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTurnsBySession")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns WHERE task_session_id = ? ORDER BY started_at ASC
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Turn
	for rows.Next() {
		turn := &models.Turn{}
		var metadataJSON string
		var completedAt sql.NullTime
		err := rows.Scan(&turn.ID, &turn.TaskSessionID, &turn.TaskID, &turn.StartedAt, &completedAt, &metadataJSON, &turn.CreatedAt, &turn.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if completedAt.Valid {
			turn.CompletedAt = &completedAt.Time
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &turn.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize turn metadata: %w", err)
			}
		}
		result = append(result, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// taskSessionSelectCols is the column list used by every SELECT that scans
// into a *models.TaskSession via scanTaskSession / scanTaskSessionRow. Centralised
// so columns can be added in one place. Order MUST match the scan helpers.
//
// agent_execution_id and container_id come from executors_running (the single
// source of truth for active execution state) via LEFT JOIN. All other columns
// come from task_sessions (aliased ts).
//
// ADR 0005: agent_profile_id is the single column for both kanban (FK to a
// shallow profile) and office (FK to a per-workspace rich profile) sessions —
// the two column names that used to live here have collapsed into one.
const taskSessionSelectCols = `ts.id, ts.task_id,
	COALESCE(er.agent_execution_id, ''), COALESCE(er.container_id, ''),
	ts.agent_profile_id, ts.execution_profile_id, ts.executor_id, ts.executor_profile_id, ts.environment_id,
	ts.repository_id, ts.base_branch, ts.base_commit_sha, ts.workspace_path,
	ts.agent_profile_snapshot, ts.executor_snapshot, ts.environment_snapshot, ts.repository_snapshot,
	ts.state, ts.error_message, ts.metadata, ts.started_at, ts.completed_at, ts.updated_at,
	ts.is_primary, ts.review_status, ts.is_passthrough, ts.task_environment_id, ts.name`

// taskSessionFromClause is the FROM clause that pairs with taskSessionSelectCols.
// Always reference task_sessions as `ts` and executors_running as `er` in WHERE/ORDER.
const taskSessionFromClause = `FROM task_sessions ts
	LEFT JOIN executors_running er ON er.session_id = ts.id`

// Task Session operations

// CreateTaskSession creates a new agent session
func (r *Repository) CreateTaskSession(ctx context.Context, session *models.TaskSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	// Only default StartedAt / UpdatedAt when the caller hasn't supplied
	// one. The test harness backdates StartedAt so completed sessions
	// have a non-zero duration (e.g. "Agent worked for 30s"); blowing
	// those values away here defeats that.
	if session.StartedAt.IsZero() {
		session.StartedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	if session.State == "" {
		session.State = models.TaskSessionStateCreated
	}

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize agent session metadata: %w", err)
	}
	agentProfileSnapshotJSON, err := json.Marshal(session.AgentProfileSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize agent profile snapshot: %w", err)
	}
	executorSnapshotJSON, err := json.Marshal(session.ExecutorSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize executor snapshot: %w", err)
	}
	environmentSnapshotJSON, err := json.Marshal(session.EnvironmentSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize environment snapshot: %w", err)
	}
	repositorySnapshotJSON, err := json.Marshal(session.RepositorySnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize repository snapshot: %w", err)
	}
	// agent_profile_id is NULL-able. Empty string would defeat the partial
	// unique index since SQLite treats two empty strings as equal — store NULL
	// for kanban / quick-chat rows and a real value only for office sessions
	// (per ADR 0005, kanban and office now share the same column).
	var agentProfileID interface{}
	if session.AgentProfileID != "" {
		agentProfileID = session.AgentProfileID
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_sessions (
			id, task_id, agent_profile_id, execution_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha, workspace_path,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id, name
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), session.ID, session.TaskID, agentProfileID,
		session.ExecutionProfileID, session.ExecutorID, session.ExecutorProfileID, session.EnvironmentID, session.RepositoryID, session.BaseBranch, session.BaseCommitSHA, session.WorkspacePath,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.ErrorMessage, string(metadataJSON),
		session.StartedAt, session.CompletedAt, session.UpdatedAt,
		dialect.BoolToInt(session.IsPrimary), session.ReviewStatus,
		dialect.BoolToInt(session.IsPassthrough), session.TaskEnvironmentID, session.Name)

	if err != nil && strings.Contains(err.Error(), "uniq_office_task_session") {
		// Two callers raced past their SELECT-then-INSERT for the same
		// (task_id, agent_profile_id) — surface a typed sentinel so callers
		// can classify with errors.Is rather than driver-message matching.
		return fmt.Errorf("%w: %w", ErrOfficeSessionRaceConflict, err)
	}
	return err
}

// unmarshalSessionJSON deserializes a JSON string into dest, skipping empty/placeholder values.
func unmarshalSessionJSON(jsonStr string, dest interface{}, fieldDesc string) error {
	if jsonStr == "" || jsonStr == "{}" {
		return nil
	}
	if err := json.Unmarshal([]byte(jsonStr), dest); err != nil {
		return fmt.Errorf("failed to deserialize %s: %w", fieldDesc, err)
	}
	return nil
}

func (r *Repository) scanTaskSession(ctx context.Context, row *sql.Row, noRowsErr string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime
	var isPrimary int
	var isPassthrough int
	var reviewStatus sql.NullString
	// agent_profile_id is nullable (kanban / quick-chat rows store NULL); decode
	// via NullString so the empty case maps to "" on the model.
	var agentProfileID sql.NullString
	var name sql.NullString

	err := row.Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &agentProfileID,
		&session.ExecutionProfileID, &session.ExecutorID, &session.ExecutorProfileID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch, &session.BaseCommitSHA, &session.WorkspacePath,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		&isPrimary, &reviewStatus, &isPassthrough, &session.TaskEnvironmentID, &name,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", models.ErrTaskSessionNotFound, noRowsErr)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
	session.IsPrimary = isPrimary == 1
	session.IsPassthrough = isPassthrough == 1
	if reviewStatus.Valid {
		session.ReviewStatus = models.ReviewStatus(reviewStatus.String)
	}
	if agentProfileID.Valid {
		session.AgentProfileID = agentProfileID.String
	}
	if name.Valid {
		session.Name = name.String
	}
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if err := unmarshalSessionJSON(metadataJSON, &session.Metadata, "agent session metadata"); err != nil {
		return nil, err
	}
	if err := unmarshalSessionJSON(agentProfileSnapshotJSON, &session.AgentProfileSnapshot, "agent profile snapshot"); err != nil {
		return nil, err
	}
	if err := unmarshalSessionJSON(executorSnapshotJSON, &session.ExecutorSnapshot, "executor snapshot"); err != nil {
		return nil, err
	}
	if err := unmarshalSessionJSON(environmentSnapshotJSON, &session.EnvironmentSnapshot, "environment snapshot"); err != nil {
		return nil, err
	}
	if err := unmarshalSessionJSON(repositorySnapshotJSON, &session.RepositorySnapshot, "repository snapshot"); err != nil {
		return nil, err
	}

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// GetTaskSession retrieves an agent session by ID
func (r *Repository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.id = ?`,
	), id)
	return r.scanTaskSession(ctx, row, fmt.Sprintf("agent session not found: %s", id))
}

// GetTaskSessionByTaskID retrieves the most recent agent session for a task
func (r *Repository) GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.task_id = ? ORDER BY ts.started_at DESC LIMIT 1`,
	), taskID)
	return r.scanTaskSession(ctx, row, fmt.Sprintf("agent session not found for task: %s", taskID))
}

// GetActiveTaskSessionByTaskID retrieves the active (running/waiting) agent session for a task
func (r *Repository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+`
		 WHERE ts.task_id = ? AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		 ORDER BY ts.started_at DESC LIMIT 1`,
	), taskID)
	return r.scanTaskSession(ctx, row, fmt.Sprintf("no active agent session for task: %s", taskID))
}

// GetTaskSessionByTaskAndAgent retrieves the office task session for the given
// (task_id, agent_profile_id) pair. The pair is unique across non-NULL
// agent_profile_id rows, so at most one row matches. Returns nil, nil when
// no session exists for the pair.
func (r *Repository) GetTaskSessionByTaskAndAgent(ctx context.Context, taskID, agentInstanceID string) (*models.TaskSession, error) {
	if taskID == "" || agentInstanceID == "" {
		return nil, nil
	}
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+`
		 WHERE ts.task_id = ? AND ts.agent_profile_id = ?
		 ORDER BY ts.started_at DESC LIMIT 1`,
	), taskID, agentInstanceID)
	session, err := r.scanTaskSession(ctx, row, "task_sessions: no matching row")
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, nil
	}
	return session, err
}

// ListNonTerminalSessionsByAgentInstance returns every office task_session row
// for the given agent_profile_id whose state is NOT terminal
// (CREATED / STARTING / RUNNING / IDLE / WAITING_FOR_INPUT). Used by the
// agent-instance deletion cascade in office, which must terminate all of an
// agent's live sessions across every task.
func (r *Repository) ListNonTerminalSessionsByAgentInstance(ctx context.Context, agentInstanceID string) ([]*models.TaskSession, error) {
	if agentInstanceID == "" {
		return nil, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+`
		 WHERE ts.agent_profile_id = ?
		   AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT')`,
	), agentInstanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTaskSessions(ctx, rows)
}

// UpdateTaskSession updates an existing agent session.
// Note: metadata is NOT updated here to prevent clobbering concurrent writes
// from UpdateSessionMetadata callers (e.g. setSessionPlanMode, persistPrepareResult).
// Use UpdateSessionMetadata for metadata changes.
func (r *Repository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	session.UpdatedAt = time.Now().UTC()
	return r.updateTaskSession(ctx, r.db, session)
}

// UpdateTaskSessionIfCurrentState persists a full session row only while the
// stored state still matches expected. Runtime launch/resume paths use this to
// retain their non-state fields without reviving a concurrently cancelled
// session from a stale snapshot.
func (r *Repository) UpdateTaskSessionIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	session.UpdatedAt = time.Now().UTC()
	return r.updateTaskSessionWithStateGuard(ctx, r.db, session, &expected)
}

// UpdateTaskSessionIfCurrentStateRemovingMetadataKeys persists a full session
// row and removes provider-owned metadata atomically while the stored state
// still matches expected. JSON removal preserves unrelated concurrent keys.
func (r *Repository) UpdateTaskSessionIfCurrentStateRemovingMetadataKeys(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
	keys []string,
) (bool, error) {
	session.UpdatedAt = time.Now().UTC()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := r.updateTaskSessionWithStateGuard(ctx, tx, session, &expected)
	if err != nil || !changed {
		return changed, err
	}
	if err := r.removeSessionMetadataKeys(ctx, tx, session.ID, keys, session.UpdatedAt); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) removeSessionMetadataKeys(
	ctx context.Context,
	exec taskSessionExecutor,
	sessionID string,
	keys []string,
	updatedAt time.Time,
) error {
	if len(keys) == 0 {
		return nil
	}
	driver := r.db.DriverName()
	var query string
	args := make([]interface{}, 0, len(keys)+2)
	if dialect.IsPostgres(driver) {
		expr := "(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END)"
		for range keys {
			expr = fmt.Sprintf("(%s #- ARRAY[?]::text[])", expr)
		}
		for _, key := range keys {
			args = append(args, key)
		}
		query = fmt.Sprintf("UPDATE task_sessions SET metadata = %s::text, updated_at = ? WHERE id = ?", expr)
	} else {
		paths := make([]string, len(keys))
		for i, key := range keys {
			paths[i] = "?"
			args = append(args, "$."+key)
		}
		query = fmt.Sprintf(
			"UPDATE task_sessions SET metadata = json_remove(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, %s), updated_at = ? WHERE id = ?",
			strings.Join(paths, ", "),
		)
	}
	args = append(args, updatedAt, sessionID)
	result, err := exec.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	return nil
}

// UpdateTaskSessionWithMetadata updates the session row and metadata column in
// one transaction so callers cannot observe a partially-applied update.
func (r *Repository) UpdateTaskSessionWithMetadata(
	ctx context.Context,
	session *models.TaskSession,
	metadata map[string]interface{},
) error {
	session.UpdatedAt = time.Now().UTC()
	metadataJSON, err := marshalSessionMetadata(metadata)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.updateTaskSession(ctx, tx, session); err != nil {
		return err
	}
	if err := r.updateSessionMetadataJSON(ctx, tx, session.ID, metadataJSON, session.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) updateTaskSession(
	ctx context.Context,
	exec taskSessionExecutor,
	session *models.TaskSession,
) error {
	changed, err := r.updateTaskSessionWithStateGuard(ctx, exec, session, nil)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, session.ID)
	}
	return nil
}

func (r *Repository) updateTaskSessionWithStateGuard(
	ctx context.Context,
	exec taskSessionExecutor,
	session *models.TaskSession,
	expected *models.TaskSessionState,
) (bool, error) {
	agentProfileSnapshotJSON, err := json.Marshal(session.AgentProfileSnapshot)
	if err != nil {
		return false, fmt.Errorf("failed to serialize agent profile snapshot: %w", err)
	}
	executorSnapshotJSON, err := json.Marshal(session.ExecutorSnapshot)
	if err != nil {
		return false, fmt.Errorf("failed to serialize executor snapshot: %w", err)
	}
	environmentSnapshotJSON, err := json.Marshal(session.EnvironmentSnapshot)
	if err != nil {
		return false, fmt.Errorf("failed to serialize environment snapshot: %w", err)
	}
	repositorySnapshotJSON, err := json.Marshal(session.RepositorySnapshot)
	if err != nil {
		return false, fmt.Errorf("failed to serialize repository snapshot: %w", err)
	}

	// metadata is NOT written here — callers wanting to change it must use
	// UpdateSessionMetadata or SetSessionMetadataKey. A full-row write here
	// would clobber metadata set via those side-channel paths since the
	// caller's in-memory copy may be stale.

	// agent_profile_id is stored as NULL when empty so the partial unique
	// index over (task_id, agent_profile_id) ignores kanban / quick-chat rows.
	var agentProfileID interface{}
	if session.AgentProfileID != "" {
		agentProfileID = session.AgentProfileID
	}
	query := `
		UPDATE task_sessions SET
			agent_profile_id = ?, execution_profile_id = ?, executor_id = ?, executor_profile_id = ?, environment_id = ?,
			repository_id = ?, base_branch = ?, base_commit_sha = ?, workspace_path = ?,
			agent_profile_snapshot = ?, executor_snapshot = ?, environment_snapshot = ?, repository_snapshot = ?,
			state = ?, error_message = ?, completed_at = ?, updated_at = ?,
			is_primary = ?, review_status = ?, is_passthrough = ?, task_environment_id = ?
		WHERE id = ?`
	args := []interface{}{agentProfileID, session.ExecutionProfileID, session.ExecutorID, session.ExecutorProfileID, session.EnvironmentID,
		session.RepositoryID, session.BaseBranch, session.BaseCommitSHA, session.WorkspacePath,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.ErrorMessage, session.CompletedAt, session.UpdatedAt,
		dialect.BoolToInt(session.IsPrimary), session.ReviewStatus,
		dialect.BoolToInt(session.IsPassthrough), session.TaskEnvironmentID,
		session.ID}
	if expected != nil {
		query += " AND state = ?"
		args = append(args, string(*expected))
	}
	result, err := exec.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// RenameTaskSession updates just the user-supplied name of an agent session.
// name is deliberately excluded from the full-row updateTaskSession write (like
// metadata) so concurrent session updates can't clobber a rename.
func (r *Repository) RenameTaskSession(ctx context.Context, id, name string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET name = ?, updated_at = ? WHERE id = ?
	`), name, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, id)
	}
	return nil
}

// UpdateTaskSessionAgentProfileSnapshot updates only the profile snapshot.
// Runtime capability events may carry a stale full session row, so keeping
// this write narrow prevents them from overwriting terminal state.
func (r *Repository) UpdateTaskSessionAgentProfileSnapshot(
	ctx context.Context,
	id string,
	snapshot map[string]interface{},
) error {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize agent profile snapshot: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET agent_profile_snapshot = ?, updated_at = ?
		WHERE id = ?
	`), string(snapshotJSON), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, id)
	}
	return nil
}

// UpdateTaskSessionState updates just the state and error message of an agent session
func (r *Repository) UpdateTaskSessionState(ctx context.Context, id string, status models.TaskSessionState, errorMessage string) error {
	now := time.Now().UTC()
	completedAt := completedAtForTaskSessionState(status, now)

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET state = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?
	`), string(status), errorMessage, completedAt, now, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, id)
	}
	return nil
}

// UpdateTaskSessionStateIfCurrent transitions a session only when its state
// still matches the caller's observation. The returned timestamp belongs to
// the committed write and remains authoritative even if a later read fails.
func (r *Repository) UpdateTaskSessionStateIfCurrent(
	ctx context.Context,
	id string,
	expected, status models.TaskSessionState,
	errorMessage string,
) (bool, time.Time, error) {
	now := time.Now().UTC()
	completedAt := completedAtForTaskSessionState(status, now)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET state = ?, error_message = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND state = ?
	`), string(status), errorMessage, completedAt, now, id, string(expected))
	if err != nil {
		return false, time.Time{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, time.Time{}, err
	}
	return rows > 0, now, nil
}

// CancelActiveTaskSession atomically transitions one active session to
// CANCELLED. A false result means the row exists in a non-active state or was
// concurrently changed before this conditional write; callers re-read to
// distinguish those cases from a missing row. The returned timestamp belongs
// to the committed cancellation, so accepting callers never need a fallible
// post-write read before scheduling teardown.
func (r *Repository) CancelActiveTaskSession(ctx context.Context, id, reason string) (bool, time.Time, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET state = ?, error_message = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
			AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
	`), string(models.TaskSessionStateCancelled), reason, now, now, id)
	if err != nil {
		return false, time.Time{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, time.Time{}, err
	}
	return rows > 0, now, nil
}

func completedAtForTaskSessionState(status models.TaskSessionState, now time.Time) *time.Time {
	if status == models.TaskSessionStateCompleted ||
		status == models.TaskSessionStateFailed ||
		status == models.TaskSessionStateCancelled {
		return &now
	}
	return nil
}

// CancelActiveTaskSessionsByTaskID transitions every active session of a task
// (CREATED/STARTING/RUNNING/WAITING_FOR_INPUT) to CANCELLED, returning the
// number of rows changed. The transition is a pure DB state change and does not
// require a live agent execution, making it the authoritative way to finalize a
// task's sessions independent of agent-process teardown.
func (r *Repository) CancelActiveTaskSessionsByTaskID(ctx context.Context, taskID, reason string) (int64, error) {
	now := time.Now().UTC()
	writeCtx := context.WithoutCancel(ctx)
	result, err := r.db.ExecContext(writeCtx, r.db.Rebind(`
		UPDATE task_sessions
		SET state = ?, error_message = ?, completed_at = ?, updated_at = ?
		WHERE task_id = ?
			AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
	`), string(models.TaskSessionStateCancelled), reason, now, now, taskID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateSessionMetadata updates only the metadata column of a session,
// avoiding a full-row overwrite that could clobber concurrent field updates.
func (r *Repository) UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error {
	metadataJSON, err := marshalSessionMetadata(metadata)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.updateSessionMetadataJSON(ctx, r.db, sessionID, metadataJSON, now)
}

func marshalSessionMetadata(metadata map[string]interface{}) (string, error) {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to serialize metadata: %w", err)
	}
	return string(metadataJSON), nil
}

func (r *Repository) updateSessionMetadataJSON(
	ctx context.Context,
	exec taskSessionExecutor,
	sessionID string,
	metadataJSON string,
	updatedAt time.Time,
) error {
	result, err := exec.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET metadata = ?, updated_at = ? WHERE id = ?
	`), metadataJSON, updatedAt, sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	return nil
}

// SetSessionMetadataKey atomically sets a single key in the session's metadata
// using SQLite's json_set. Unlike UpdateSessionMetadata (which does a full
// replacement), this preserves all other metadata keys.
func (r *Repository) SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to serialize metadata value: %w", err)
	}
	now := time.Now().UTC()
	path := "$." + key
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?, json(?)), updated_at = ? WHERE id = ?
	`), path, string(valueJSON), now, sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	return nil
}

// SetSessionMetadataKeyIfAbsent atomically writes a metadata key only when it
// does not already exist. The returned bool reports whether this call stored
// the value.
func (r *Repository) SetSessionMetadataKeyIfAbsent(
	ctx context.Context,
	sessionID string,
	key string,
	value interface{},
) (bool, error) {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to serialize metadata value: %w", err)
	}
	now := time.Now().UTC()
	driver := r.db.DriverName()
	path := key
	if !dialect.IsPostgres(driver) {
		path = "$." + key
	}
	query := setSessionMetadataKeyIfAbsentQuery(driver)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), path, string(valueJSON), now, sessionID, path)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func setSessionMetadataKeyIfAbsentQuery(driver string) string {
	if dialect.IsPostgres(driver) {
		return `
			UPDATE task_sessions
			SET metadata = jsonb_set(
				CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END,
				ARRAY[?]::text[],
				?::jsonb,
				true
			)::text,
				updated_at = ?
			WHERE id = ?
				AND jsonb_extract_path(
					CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END,
					?
				) IS NULL
		`
	}
	return `
		UPDATE task_sessions
		SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?, json(?)),
			updated_at = ?
		WHERE id = ?
			AND json_type(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?) IS NULL
	`
}

// SetSessionACPSessionID mirrors the agent's ACP session id into the session's
// "acp" metadata map as a single atomic UPDATE. json_patch merges the sub-key,
// so keys session_info already wrote (title, ...) survive without a
// read-modify-write round trip. The write only happens while the session's
// executors_running row still holds acpSessionID as its resume token — i.e.
// the CAS that stored the token hasn't been superseded by a rotated execution
// — and is skipped when the stored id is already current, so a no-op never
// touches updated_at. Returns whether a row was written.
func (r *Repository) SetSessionACPSessionID(ctx context.Context, sessionID, acpSessionID string) (bool, error) {
	patch, err := json.Marshal(map[string]interface{}{"acp": map[string]string{"session_id": acpSessionID}})
	if err != nil {
		return false, fmt.Errorf("failed to serialize metadata patch: %w", err)
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET metadata = json_patch(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, json(?)),
		    updated_at = ?
		WHERE id = ?
		  AND json_extract(metadata, '$.acp.session_id') IS NOT ?
		  AND EXISTS (
			SELECT 1 FROM executors_running er
			WHERE er.session_id = task_sessions.id AND er.resume_token = ?
		  )
	`), string(patch), now, sessionID, acpSessionID, acpSessionID)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (r *Repository) DismissLastAgentError(ctx context.Context, sessionID string, expected models.LastAgentError, dismissedAt time.Time) (bool, error) {
	next := expected
	next.DismissedAt = &dismissedAt
	valueJSON, err := json.Marshal(next)
	if err != nil {
		return false, fmt.Errorf("failed to serialize metadata value: %w", err)
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, '$.last_agent_error', json(?)),
			updated_at = ?
		WHERE id = ?
			AND json_extract(metadata, '$.last_agent_error.message') = ?
			AND (
				json_extract(metadata, '$.last_agent_error.occurred_at') = ?
				OR julianday(json_extract(metadata, '$.last_agent_error.occurred_at')) = julianday(?)
			)
	`), string(valueJSON), now, sessionID, expected.Message, expected.OccurredAt.UTC().Format(time.RFC3339Nano), expected.OccurredAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// GetLastAgentMessage returns the content of the most recent agent message in a session.
func (r *Repository) GetLastAgentMessage(ctx context.Context, sessionID string) (string, error) {
	var content string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT content FROM task_session_messages
		WHERE task_session_id = ? AND author_type = 'agent' AND type = 'message'
		ORDER BY created_at DESC LIMIT 1
	`), sessionID).Scan(&content)
	if err != nil {
		return "", err
	}
	return content, nil
}

// IncrementTaskSessionUsage adds the given deltas to the cumulative
// tokens / cost columns on task_sessions. Used by the office cost
// subscriber after a cost event lands so the per-session totals stay
// in sync without re-summing office_cost_events. The model + DTO
// don't surface these columns yet (DB-only per the office-costs
// wedge); the cost explorer follow-up will expose them.
func (r *Repository) IncrementTaskSessionUsage(
	ctx context.Context, sessionID string, tokensIn, tokensOut, costSubcents int64,
) error {
	if sessionID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		   SET tokens_in     = COALESCE(tokens_in, 0)     + ?,
		       tokens_out    = COALESCE(tokens_out, 0)    + ?,
		       cost_subcents = COALESCE(cost_subcents, 0) + ?
		 WHERE id = ?
	`), tokensIn, tokensOut, costSubcents, sessionID)
	return err
}

// UpdateTaskSessionBaseCommit updates the base_commit_sha for a session.
// This is called after agent launch to capture the HEAD commit at session start.
func (r *Repository) UpdateTaskSessionBaseCommit(ctx context.Context, id string, baseCommitSHA string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET base_commit_sha = ?, updated_at = ? WHERE id = ?
	`), baseCommitSHA, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, id)
	}
	return nil
}

// ResetTaskSessionBasesForRepository rewrites base_branch and clears
// base_commit_sha on every task_session belonging to (taskID, repositoryID).
// Used by service.UpdateRepositoryBaseBranch after the changes-panel
// "Compare against" picker changes the recorded base.
//
// Clearing base_commit_sha matters because git_handlers.computeGitCommits and
// computeCumulativeDiff read it first and only fall back to the live
// agentctl GetGitStatus().BaseCommit (which now resolves against the new
// base_branch via Phase 1) when the column is empty. Without this reset the
// task-card stat counts would refresh against the new base while the
// commits panel + cumulative diff would keep filtering against the OLD
// commit snapshot — visible to users as "Commits section disappeared
// after I changed the base".
//
// Returns the number of sessions touched so callers can log / no-op when
// the task has no sessions yet.
func (r *Repository) ResetTaskSessionBasesForRepository(ctx context.Context, taskID, repositoryID, baseBranch string) (int64, error) {
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	if repositoryID == "" {
		return 0, fmt.Errorf("repository_id is required")
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET base_branch = ?, base_commit_sha = '', updated_at = ?
		WHERE task_id = ? AND repository_id = ?
	`), baseBranch, now, taskID, repositoryID)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

// ListTaskSessions returns all agent sessions for a task
func (r *Repository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTaskSessions")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.task_id = ? ORDER BY ts.started_at DESC`,
	), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	return r.loadWorktreesBatch(ctx, sessions)
}

// ListActiveTaskSessions returns all active agent sessions across all tasks
func (r *Repository) ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListActiveTaskSessions")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx,
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY ts.started_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	return r.loadWorktreesBatch(ctx, sessions)
}

// ListActiveTaskSessionsByTaskID returns all active agent sessions for a specific task
func (r *Repository) ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.task_id = ? AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY ts.started_at DESC`,
	), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	return r.loadWorktreesBatch(ctx, sessions)
}

// loadWorktreesBatch loads worktrees for multiple sessions in a single query.
func (r *Repository) loadWorktreesBatch(ctx context.Context, sessions []*models.TaskSession) ([]*models.TaskSession, error) {
	if len(sessions) == 0 {
		return sessions, nil
	}
	sessionIDs := make([]string, len(sessions))
	for i, s := range sessions {
		sessionIDs[i] = s.ID
	}
	worktreeMap, err := r.ListWorktreesBySessionIDs(ctx, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch-load session worktrees: %w", err)
	}
	for _, session := range sessions {
		session.Worktrees = worktreeMap[session.ID]
	}
	return sessions, nil
}

// BatchGetSessionsByTaskIDs returns all task_sessions for the given task IDs,
// grouped by task ID and ordered by started_at DESC within each task. The
// returned sessions carry their associated worktrees (loaded in one extra
// query). Used by callers that need primary session, session count, and
// session info for many tasks in one round trip (e.g. the workspace
// task-list endpoint) to avoid issuing one GetSession per task.
//
// Returns an empty map for an empty input. Chunks input larger than
// sqliteMaxHostParams to stay well below SQLite's compile-time placeholder
// limit.
func (r *Repository) BatchGetSessionsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskSession, error) {
	result := make(map[string][]*models.TaskSession, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	for _, chunk := range chunkIDs(taskIDs, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		query := `SELECT ` + taskSessionSelectCols + ` ` + taskSessionFromClause +
			` WHERE ts.task_id IN (` + placeholders + `) ORDER BY ts.task_id, ts.started_at DESC`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		sessions, scanErr := r.scanTaskSessions(ctx, rows)
		_ = rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		sessions, err = r.loadWorktreesBatch(ctx, sessions)
		if err != nil {
			return nil, err
		}
		for _, s := range sessions {
			result[s.TaskID] = append(result[s.TaskID], s)
		}
	}
	return result, nil
}

func (r *Repository) HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	var exists int
	// Exclude ephemeral tasks (quick chat, config chat) - they shouldn't block profile deletion
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_sessions ts
		JOIN tasks t ON ts.task_id = t.id
		WHERE ts.agent_profile_id = ?
		  AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		  AND t.is_ephemeral = 0
		LIMIT 1
	`), agentProfileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) GetActiveTaskInfoByAgentProfile(ctx context.Context, agentProfileID string) ([]agentdto.ActiveTaskInfo, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT DISTINCT t.id, t.title, t.is_ephemeral
		FROM task_sessions ts
		JOIN tasks t ON t.id = ts.task_id
		WHERE ts.agent_profile_id = ? AND ts.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
	`), agentProfileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []agentdto.ActiveTaskInfo
	for rows.Next() {
		var info agentdto.ActiveTaskInfo
		if err := rows.Scan(&info.TaskID, &info.TaskTitle, &info.IsEphemeral); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

func (r *Repository) HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	var exists int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_sessions
		WHERE environment_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`), environmentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) HasActiveTaskSessionsByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (bool, error) {
	var exists int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_sessions
		WHERE task_environment_id = ?
			AND task_id != ?
			AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`), taskEnvironmentID, taskID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (string, error) {
	var borrowerTaskID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT task_id FROM task_sessions
		WHERE task_environment_id = ?
			AND task_id != ?
			AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		ORDER BY updated_at DESC
		LIMIT 1
	`), taskEnvironmentID, taskID).Scan(&borrowerTaskID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return borrowerTaskID, err
}

func (r *Repository) HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	var exists int
	// Only sessions of live (non-archived) tasks count; archived tasks never
	// block repository deletion. IDLE sessions are resumable and must
	// preserve their repository rows.
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1
		FROM task_sessions s
		INNER JOIN task_repositories tr ON tr.task_id = s.task_id
		INNER JOIN tasks t ON t.id = s.task_id
		WHERE s.state IN ('CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT')
			AND tr.repository_id = ?
			AND t.archived_at IS NULL
		LIMIT 1
	`), repositoryID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) CountActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (int, error) {
	var count int
	// Counts only sessions of live (non-archived) tasks, including resumable
	// IDLE sessions, matching HasActiveTaskSessionsByRepository.
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*)
		FROM task_sessions s
		INNER JOIN task_repositories tr ON tr.task_id = s.task_id
		INNER JOIN tasks t ON t.id = s.task_id
		WHERE s.state IN ('CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT')
			AND tr.repository_id = ?
			AND t.archived_at IS NULL
	`), repositoryID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// DeleteEphemeralTasksByAgentProfile deletes all ephemeral tasks (and their sessions)
// that are using the specified agent profile. This is used during profile deletion
// to clean up transient quick chat / config chat tasks.
func (r *Repository) DeleteEphemeralTasksByAgentProfile(ctx context.Context, agentProfileID string) (int64, error) {
	// Delete tasks that are ephemeral and have sessions using this profile.
	// CASCADE will handle session deletion.
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM tasks
		WHERE is_ephemeral = 1
		  AND id IN (
			SELECT DISTINCT task_id FROM task_sessions WHERE agent_profile_id = ?
		  )
	`), agentProfileID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// scanTaskSessions is a helper to scan multiple agent session rows
func (r *Repository) scanTaskSessions(ctx context.Context, rows *sql.Rows) ([]*models.TaskSession, error) {
	var result []*models.TaskSession
	for rows.Next() {
		session, err := scanTaskSessionRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, rows.Err()
}

// scanTaskSessionRow scans a single row into a TaskSession, applying all field mappings and JSON unmarshalling.
func scanTaskSessionRow(rows *sql.Rows) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime
	var isPrimary int
	var isPassthrough int
	var reviewStatus sql.NullString
	var agentProfileID sql.NullString
	var name sql.NullString

	err := rows.Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &agentProfileID,
		&session.ExecutionProfileID, &session.ExecutorID, &session.ExecutorProfileID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch, &session.BaseCommitSHA, &session.WorkspacePath,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		&isPrimary, &reviewStatus, &isPassthrough, &session.TaskEnvironmentID, &name,
	)
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
	session.IsPrimary = isPrimary == 1
	session.IsPassthrough = isPassthrough == 1
	if reviewStatus.Valid {
		session.ReviewStatus = models.ReviewStatus(reviewStatus.String)
	}
	if agentProfileID.Valid {
		session.AgentProfileID = agentProfileID.String
	}
	if name.Valid {
		session.Name = name.String
	}
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}

	if err := unmarshalSessionSnapshots(session, metadataJSON, agentProfileSnapshotJSON,
		executorSnapshotJSON, environmentSnapshotJSON, repositorySnapshotJSON); err != nil {
		return nil, err
	}

	return session, nil
}

// unmarshalSessionSnapshots deserializes all JSON snapshot fields into the session struct.
func unmarshalSessionSnapshots(
	session *models.TaskSession,
	metadataJSON, agentProfileSnapshotJSON, executorSnapshotJSON, environmentSnapshotJSON, repositorySnapshotJSON string,
) error {
	if err := unmarshalSessionJSON(metadataJSON, &session.Metadata, "agent session metadata"); err != nil {
		return err
	}
	if err := unmarshalSessionJSON(agentProfileSnapshotJSON, &session.AgentProfileSnapshot, "agent profile snapshot"); err != nil {
		return err
	}
	if err := unmarshalSessionJSON(executorSnapshotJSON, &session.ExecutorSnapshot, "executor snapshot"); err != nil {
		return err
	}
	if err := unmarshalSessionJSON(environmentSnapshotJSON, &session.EnvironmentSnapshot, "environment snapshot"); err != nil {
		return err
	}
	return unmarshalSessionJSON(repositorySnapshotJSON, &session.RepositorySnapshot, "repository snapshot")
}

// DeleteTaskSession deletes an agent session by ID
func (r *Repository) DeleteTaskSession(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_sessions WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// Task Session Worktree operations

func (r *Repository) CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error {
	if sessionWorktree.ID == "" {
		sessionWorktree.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	sessionWorktree.CreatedAt = now
	updatedAt := now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_worktrees (
			id, session_id, worktree_id, repository_id, branch_slug, position,
			worktree_path, worktree_branch, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, worktree_id) DO UPDATE SET
			repository_id = excluded.repository_id,
			branch_slug = CASE
				WHEN excluded.branch_slug != '' THEN excluded.branch_slug
				ELSE task_session_worktrees.branch_slug
			END,
			position = excluded.position,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			updated_at = excluded.updated_at
	`),
		sessionWorktree.ID,
		sessionWorktree.SessionID,
		sessionWorktree.WorktreeID,
		sessionWorktree.RepositoryID,
		sessionWorktree.BranchSlug,
		sessionWorktree.Position,
		sessionWorktree.WorktreePath,
		sessionWorktree.WorktreeBranch,
		sessionWorktree.CreatedAt,
		updatedAt,
	)
	return err
}

// UpdateTaskSessionWorktreeBranch updates the cached worktree_branch for all
// worktrees belonging to a session. Called when a branch switch or rename is
// detected in the live workspace so downstream consumers (PR watch
// reconciliation, branch listings) see the current branch rather than the
// value captured at worktree creation.
func (r *Repository) UpdateTaskSessionWorktreeBranch(ctx context.Context, sessionID, branch string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_worktrees SET worktree_branch = ?, updated_at = ? WHERE session_id = ?
	`), branch, now, sessionID)
	return err
}

// UpdateTaskSessionWorktreeBranchByRepository updates the cached worktree_branch
// for one repository row in a session. Use this for repo-scoped live git
// operations in multi-repo tasks so sibling repositories keep their branch
// snapshots.
func (r *Repository) UpdateTaskSessionWorktreeBranchByRepository(ctx context.Context, sessionID, repositoryID, branch string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_worktrees
		SET worktree_branch = ?, updated_at = ?
		WHERE session_id = ?
		  AND repository_id = ?
		  AND deleted_at IS NULL
		  AND status = 'active'
	`), branch, now, sessionID, repositoryID)
	return err
}

func (r *Repository) ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT
			tsw.id, tsw.session_id, tsw.worktree_id, tsw.repository_id,
			COALESCE(tsw.branch_slug, ''), tsw.position,
			tsw.worktree_path, tsw.worktree_branch, tsw.created_at
		FROM task_session_worktrees tsw
		WHERE tsw.session_id = ?
		  AND tsw.deleted_at IS NULL
		  AND tsw.status = 'active'
		ORDER BY tsw.position ASC, tsw.created_at ASC
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var worktrees []*models.TaskSessionWorktree
	for rows.Next() {
		var wt models.TaskSessionWorktree
		err := rows.Scan(
			&wt.ID,
			&wt.SessionID,
			&wt.WorktreeID,
			&wt.RepositoryID,
			&wt.BranchSlug,
			&wt.Position,
			&wt.WorktreePath,
			&wt.WorktreeBranch,
			&wt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, &wt)
	}
	return worktrees, rows.Err()
}

// ListWorktreesBySessionIDs returns all worktrees for the given session IDs,
// grouped by session ID. This eliminates N+1 queries when loading worktrees
// for multiple sessions. Chunks input above sqliteMaxHostParams (500) because
// callers like loadWorktreesBatch — invoked from BatchGetSessionsByTaskIDs —
// can pass `chunk_size_tasks × avg_sessions_per_task` IDs, which crosses
// SQLite's SQLITE_MAX_VARIABLE_NUMBER (999 on older builds) at modest task
// volumes (500 tasks × 2 sessions = 1000 placeholders).
func (r *Repository) ListWorktreesBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]*models.TaskSessionWorktree, error) {
	result := make(map[string][]*models.TaskSessionWorktree, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return result, nil
	}
	for _, chunk := range chunkIDs(sessionIDs, sqliteMaxHostParams) {
		if err := r.appendWorktreesForSessionChunk(ctx, chunk, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// appendWorktreesForSessionChunk runs the worktree SELECT for one chunk of
// session IDs and merges the rows into result. Extracted from
// ListWorktreesBySessionIDs so the public method stays inside the funlen cap
// after the chunking loop was added.
func (r *Repository) appendWorktreesForSessionChunk(
	ctx context.Context,
	sessionIDs []string,
	result map[string][]*models.TaskSessionWorktree,
) error {
	placeholders, args := buildInPlaceholders(sessionIDs)
	query := `SELECT tsw.id, tsw.session_id, tsw.worktree_id, tsw.repository_id, tsw.position,
		COALESCE(tsw.branch_slug, ''), tsw.worktree_path, tsw.worktree_branch, tsw.created_at
		FROM task_session_worktrees tsw
		WHERE tsw.session_id IN (` + placeholders + `)
		  AND tsw.deleted_at IS NULL
		  AND tsw.status = 'active'
		ORDER BY tsw.position ASC, tsw.created_at ASC`

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var wt models.TaskSessionWorktree
		if err := rows.Scan(&wt.ID, &wt.SessionID, &wt.WorktreeID, &wt.RepositoryID,
			&wt.Position, &wt.BranchSlug, &wt.WorktreePath, &wt.WorktreeBranch, &wt.CreatedAt); err != nil {
			return err
		}
		result[wt.SessionID] = append(result[wt.SessionID], &wt)
	}
	return rows.Err()
}

func (r *Repository) DeleteTaskSessionWorktree(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_worktrees WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task session worktree not found: %s", id)
	}
	return nil
}

func (r *Repository) DeleteTaskSessionWorktreesBySession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_worktrees WHERE session_id = ?`), sessionID)
	return err
}

// GetPrimarySessionByTaskID retrieves the primary session for a task.
// Returns ErrNoPrimarySession (wrapped) when the task has no primary session
// row; callers should use errors.Is to distinguish this from real DB errors.
func (r *Repository) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.task_id = ? AND ts.is_primary = 1 LIMIT 1`,
	), taskID)
	session, err := r.scanTaskSession(ctx, row, "task_sessions: no primary session row")
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrNoPrimarySession, taskID)
	}
	return session, err
}

// GetPrimarySessionIDsByTaskIDs returns a map of task ID to primary session ID for the given task IDs.
// Tasks without a primary session are not included in the result.
func (r *Repository) GetPrimarySessionIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error) {
	if len(taskIDs) == 0 {
		return make(map[string]string), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, task_id FROM task_sessions
		WHERE task_id IN (%s) AND is_primary = 1
	`, strings.Join(placeholders, ","))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]string)
	for rows.Next() {
		var sessionID, taskID string
		if err := rows.Scan(&sessionID, &taskID); err != nil {
			return nil, err
		}
		result[taskID] = sessionID
	}
	return result, rows.Err()
}

// GetSessionCountsByTaskIDs returns a map of task ID to session count for the given task IDs.
func (r *Repository) GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	if len(taskIDs) == 0 {
		return make(map[string]int), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT task_id, COUNT(*) as count FROM task_sessions
		WHERE task_id IN (%s)
		GROUP BY task_id
	`, strings.Join(placeholders, ","))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var taskID string
		var count int
		if err := rows.Scan(&taskID, &count); err != nil {
			return nil, err
		}
		result[taskID] = count
	}
	return result, rows.Err()
}

// GetPrimarySessionInfoByTaskIDs returns a map of task ID to primary session for the given task IDs.
// Returns review_status, executor info, agent profile snapshot, and repository snapshot.
func (r *Repository) GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	if len(taskIDs) == 0 {
		return make(map[string]*models.TaskSession), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT ts.id, ts.task_id, ts.review_status, ts.executor_id, ts.state,
		       ts.agent_profile_snapshot, ts.repository_snapshot,
		       e.type, e.name
		FROM task_sessions ts
		LEFT JOIN executors e ON e.id = ts.executor_id
		WHERE ts.task_id IN (%s) AND ts.is_primary = 1
	`, strings.Join(placeholders, ","))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*models.TaskSession)
	for rows.Next() {
		var sessionID string
		var taskID string
		var reviewStatus sql.NullString
		var executorID sql.NullString
		var sessionState sql.NullString
		var agentProfileSnapshotJSON sql.NullString
		var repositorySnapshotJSON sql.NullString
		var executorType sql.NullString
		var executorName sql.NullString
		if err := rows.Scan(&sessionID, &taskID, &reviewStatus, &executorID, &sessionState, &agentProfileSnapshotJSON, &repositorySnapshotJSON, &executorType, &executorName); err != nil {
			return nil, err
		}
		session := &models.TaskSession{
			ID:     sessionID,
			TaskID: taskID,
		}
		if sessionState.Valid {
			session.State = models.TaskSessionState(sessionState.String)
		}
		if reviewStatus.Valid {
			session.ReviewStatus = models.ReviewStatus(reviewStatus.String)
		}
		if executorID.Valid {
			session.ExecutorID = executorID.String
		}
		if executorType.Valid || executorName.Valid {
			session.ExecutorSnapshot = make(map[string]interface{}, 2)
			if executorType.Valid {
				session.ExecutorSnapshot["executor_type"] = executorType.String
			}
			if executorName.Valid {
				session.ExecutorSnapshot["executor_name"] = executorName.String
			}
		}
		if agentProfileSnapshotJSON.Valid && agentProfileSnapshotJSON.String != "" {
			if err := json.Unmarshal([]byte(agentProfileSnapshotJSON.String), &session.AgentProfileSnapshot); err != nil {
				return nil, fmt.Errorf("failed to unmarshal agent profile snapshot for task %s: %w", taskID, err)
			}
		}
		if repositorySnapshotJSON.Valid && repositorySnapshotJSON.String != "" {
			if err := json.Unmarshal([]byte(repositorySnapshotJSON.String), &session.RepositorySnapshot); err != nil {
				return nil, fmt.Errorf("failed to unmarshal repository snapshot for task %s: %w", taskID, err)
			}
		}
		result[taskID] = session
	}
	return result, rows.Err()
}

// SetSessionPrimary marks a session as primary and clears the primary flag
// on every other session for the same task, atomically. Both writes go
// through a single transaction so a concurrent caller can't observe (or
// write) a half-applied state — e.g. two sessions racing on is_primary=1,
// or a reader seeing zero primary sessions mid-swap.
//
// On SQLite the writer pool is a single connection (see
// internal/db.NewSQLiteDB), so only one transaction can hold it at a time —
// the transaction alone fully serializes concurrent callers. On Postgres,
// separate connections could otherwise run two of these transactions truly
// concurrently, each seeing zero primary sessions and both promoting; to
// close that window we take an exclusive row lock on the owning task
// (`SELECT ... FOR UPDATE`) before touching its sessions, so a second
// concurrent promotion for the same task blocks until the first commits.
func (r *Repository) SetSessionPrimary(ctx context.Context, sessionID string) error {
	now := time.Now().UTC()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// First, get the task_id for this session
	var taskID string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return err
	}

	// Serialize concurrent promotions for the same task across Postgres
	// connections. SQLite has no FOR UPDATE / row-level locking and doesn't
	// need it — the single-connection writer pool already serializes here.
	if dialect.IsPostgres(r.db.DriverName()) {
		var lockedTaskID string
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM tasks WHERE id = ? FOR UPDATE`), taskID).Scan(&lockedTaskID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	// Clear primary flag on all sessions for this task
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET is_primary = 0, updated_at = ? WHERE task_id = ?
	`), now, taskID)
	if err != nil {
		return err
	}

	// Set primary flag on the specified session
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET is_primary = 1, updated_at = ? WHERE id = ?
	`), now, sessionID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return tx.Commit()
}
