// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, turn.ID, turn.TaskSessionID, turn.TaskID, turn.StartedAt, turn.CompletedAt, metadataJSON, turn.CreatedAt, turn.UpdatedAt)

	return err
}

// GetTurn retrieves a turn by ID
func (r *Repository) GetTurn(ctx context.Context, id string) (*models.Turn, error) {
	turn := &models.Turn{}
	var metadataJSON string
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns WHERE id = ?
	`, id).Scan(&turn.ID, &turn.TaskSessionID, &turn.TaskID, &turn.StartedAt, &completedAt, &metadataJSON, &turn.CreatedAt, &turn.UpdatedAt)
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

// GetActiveTurnBySessionID gets the currently active (non-completed) turn for a session
func (r *Repository) GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn := &models.Turn{}
	var metadataJSON string
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns
		WHERE task_session_id = ? AND completed_at IS NULL
		ORDER BY started_at DESC LIMIT 1
	`, sessionID).Scan(&turn.ID, &turn.TaskSessionID, &turn.TaskID, &turn.StartedAt, &completedAt, &metadataJSON, &turn.CreatedAt, &turn.UpdatedAt)
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

	_, err := r.db.ExecContext(ctx, `
		UPDATE task_session_turns
		SET completed_at = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, turn.CompletedAt, metadataJSON, turn.UpdatedAt, turn.ID)

	return err
}

// CompleteTurn marks a turn as completed with the current time
func (r *Repository) CompleteTurn(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE task_session_turns
		SET completed_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, id)
	return err
}

// ListTurnsBySession returns all turns for a session ordered by start time
func (r *Repository) ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns WHERE task_session_id = ? ORDER BY started_at ASC
	`, sessionID)
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

// Task Session operations

// CreateTaskSession creates a new agent session
func (r *Repository) CreateTaskSession(ctx context.Context, session *models.TaskSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	session.StartedAt = now
	session.UpdatedAt = now
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

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO task_sessions (
			id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
			repository_id, base_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.TaskID, session.AgentExecutionID, session.ContainerID, session.AgentProfileID,
		session.ExecutorID, session.EnvironmentID, session.RepositoryID, session.BaseBranch,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.ErrorMessage, string(metadataJSON),
		session.StartedAt, session.CompletedAt, session.UpdatedAt)

	return err
}

// GetTaskSession retrieves an agent session by ID
func (r *Repository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE id = ?
	`, id).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// GetTaskSessionByTaskID retrieves the most recent agent session for a task
func (r *Repository) GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// GetActiveTaskSessionByTaskID retrieves the active (running/waiting) agent session for a task
func (r *Repository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions
		WHERE task_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active agent session for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// UpdateTaskSession updates an existing agent session
func (r *Repository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	session.UpdatedAt = time.Now().UTC()

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

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_sessions SET
			agent_execution_id = ?, container_id = ?, agent_profile_id = ?, executor_id = ?, environment_id = ?,
			repository_id = ?, base_branch = ?,
			agent_profile_snapshot = ?, executor_snapshot = ?, environment_snapshot = ?, repository_snapshot = ?,
			state = ?, error_message = ?, metadata = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, session.AgentExecutionID, session.ContainerID, session.AgentProfileID, session.ExecutorID, session.EnvironmentID,
		session.RepositoryID, session.BaseBranch,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.ErrorMessage, string(metadataJSON), session.CompletedAt,
		session.UpdatedAt, session.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", session.ID)
	}
	return nil
}

// UpdateTaskSessionState updates just the state and error message of an agent session
func (r *Repository) UpdateTaskSessionState(ctx context.Context, id string, status models.TaskSessionState, errorMessage string) error {
	now := time.Now().UTC()

	var completedAt *time.Time
	if status == models.TaskSessionStateCompleted || status == models.TaskSessionStateFailed || status == models.TaskSessionStateCancelled {
		completedAt = &now
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_sessions SET state = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?
	`, string(status), errorMessage, completedAt, now, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// ListTaskSessions returns all agent sessions for a task
func (r *Repository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

// ListActiveTaskSessions returns all active agent sessions across all tasks
func (r *Repository) ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

// ListActiveTaskSessionsByTaskID returns all active agent sessions for a specific task
func (r *Repository) ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

func (r *Repository) HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM task_sessions
		WHERE agent_profile_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, agentProfileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM task_sessions
		WHERE environment_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, environmentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM task_sessions s
		INNER JOIN task_repositories tr ON tr.task_id = s.task_id
		WHERE s.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
			AND tr.repository_id = ?
		LIMIT 1
	`, repositoryID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// scanTaskSessions is a helper to scan multiple agent session rows
func (r *Repository) scanTaskSessions(ctx context.Context, rows *sql.Rows) ([]*models.TaskSession, error) {
	var result []*models.TaskSession
	for rows.Next() {
		session := &models.TaskSession{}
		var state string
		var metadataJSON string
		var agentProfileSnapshotJSON string
		var executorSnapshotJSON string
		var environmentSnapshotJSON string
		var repositorySnapshotJSON string
		var completedAt sql.NullTime

		err := rows.Scan(
			&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
			&session.ExecutorID, &session.EnvironmentID,
			&session.RepositoryID, &session.BaseBranch,
			&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
			&state, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.State = models.TaskSessionState(state)
		if completedAt.Valid {
			session.CompletedAt = &completedAt.Time
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
			}
		}
		if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
			}
		}
		if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
			}
		}
		if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
			}
		}
		if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
			}
		}

		result = append(result, session)
	}
	return result, rows.Err()
}

// DeleteTaskSession deletes an agent session by ID
func (r *Repository) DeleteTaskSession(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, id)
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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_session_worktrees (
			id, session_id, worktree_id, repository_id, position,
			worktree_path, worktree_branch, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, worktree_id) DO UPDATE SET
			repository_id = excluded.repository_id,
			position = excluded.position,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			updated_at = excluded.updated_at
	`,
		sessionWorktree.ID,
		sessionWorktree.SessionID,
		sessionWorktree.WorktreeID,
		sessionWorktree.RepositoryID,
		sessionWorktree.Position,
		sessionWorktree.WorktreePath,
		sessionWorktree.WorktreeBranch,
		sessionWorktree.CreatedAt,
		updatedAt,
	)
	return err
}

func (r *Repository) ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			tsw.id, tsw.session_id, tsw.worktree_id, tsw.repository_id, tsw.position,
			tsw.worktree_path, tsw.worktree_branch, tsw.created_at
		FROM task_session_worktrees tsw
		WHERE tsw.session_id = ?
		ORDER BY tsw.position ASC, tsw.created_at ASC
	`, sessionID)
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

func (r *Repository) DeleteTaskSessionWorktree(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_session_worktrees WHERE id = ?`, id)
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
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_session_worktrees WHERE session_id = ?`, sessionID)
	return err
}

