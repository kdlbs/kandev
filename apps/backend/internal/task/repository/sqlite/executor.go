package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// Executor operations

func (r *Repository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
	if executor.ID == "" {
		executor.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	executor.CreatedAt = now
	executor.UpdatedAt = now

	configJSON, err := json.Marshal(executor.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize executor config: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO executors (id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), executor.ID, executor.Name, executor.Type, executor.Status, dialect.BoolToInt(executor.IsSystem), dialect.BoolToInt(executor.Resumable), string(configJSON), executor.CreatedAt, executor.UpdatedAt, executor.DeletedAt)
	return err
}

func (r *Repository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	executor := &models.Executor{}
	var configJSON string
	var isSystem int
	var resumable int

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at
		FROM executors WHERE id = ? AND deleted_at IS NULL
	`), id).Scan(
		&executor.ID, &executor.Name, &executor.Type, &executor.Status,
		&isSystem, &resumable, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("executor not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	executor.IsSystem = isSystem == 1
	executor.Resumable = resumable == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
		}
	}
	return executor, nil
}

func (r *Repository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	executor.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(executor.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize executor config: %w", err)
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE executors SET name = ?, type = ?, status = ?, is_system = ?, resumable = ?, config = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`), executor.Name, executor.Type, executor.Status, dialect.BoolToInt(executor.IsSystem), dialect.BoolToInt(executor.Resumable), string(configJSON), executor.UpdatedAt, executor.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor not found: %s", executor.ID)
	}
	return nil
}

func (r *Repository) DeleteExecutor(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE executors SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`), now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor not found: %s", id)
	}
	return nil
}

func (r *Repository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at
		FROM executors WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Executor
	for rows.Next() {
		executor := &models.Executor{}
		var configJSON string
		var isSystem int
		var resumable int
		if err := rows.Scan(
			&executor.ID, &executor.Name, &executor.Type, &executor.Status,
			&isSystem, &resumable, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
		); err != nil {
			return nil, err
		}
		executor.IsSystem = isSystem == 1
		executor.Resumable = resumable == 1
		if configJSON != "" && configJSON != "{}" {
			if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
				return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
			}
		}
		result = append(result, executor)
	}
	return result, rows.Err()
}

func (r *Repository) UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error {
	if running == nil {
		return fmt.Errorf("executor running is nil")
	}
	if running.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if running.ID == "" {
		running.ID = running.SessionID
	}
	now := time.Now().UTC()
	if running.CreatedAt.IsZero() {
		running.CreatedAt = now
	}
	running.UpdatedAt = now

	metadataJSON := "{}"
	if running.Metadata != nil {
		b, err := json.Marshal(running.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize executor running metadata: %w", err)
		}
		metadataJSON = string(b)
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO executors_running (
			id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
			last_message_uuid, agent_execution_id, container_id, agentctl_url, agentctl_port, pid,
			worktree_id, worktree_path, worktree_branch, last_seen_at, error_message, metadata,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			id = excluded.id,
			task_id = excluded.task_id,
			executor_id = excluded.executor_id,
			runtime = excluded.runtime,
			status = excluded.status,
			resumable = excluded.resumable,
			resume_token = excluded.resume_token,
			last_message_uuid = excluded.last_message_uuid,
			agent_execution_id = excluded.agent_execution_id,
			container_id = excluded.container_id,
			agentctl_url = excluded.agentctl_url,
			agentctl_port = excluded.agentctl_port,
			pid = excluded.pid,
			worktree_id = excluded.worktree_id,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			last_seen_at = excluded.last_seen_at,
			error_message = excluded.error_message,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`),
		running.ID,
		running.SessionID,
		running.TaskID,
		running.ExecutorID,
		running.Runtime,
		running.Status,
		dialect.BoolToInt(running.Resumable),
		running.ResumeToken,
		running.LastMessageUUID,
		running.AgentExecutionID,
		running.ContainerID,
		running.AgentctlURL,
		running.AgentctlPort,
		running.PID,
		running.WorktreeID,
		running.WorktreePath,
		running.WorktreeBranch,
		running.LastSeenAt,
		running.ErrorMessage,
		metadataJSON,
		running.CreatedAt,
		running.UpdatedAt,
	)
	return err
}

func (r *Repository) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
			last_message_uuid, agent_execution_id, container_id, agentctl_url, agentctl_port,
			worktree_id, worktree_path, worktree_branch, metadata, created_at, updated_at
		FROM executors_running
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.ExecutorRunning
	for rows.Next() {
		running := &models.ExecutorRunning{}
		var metadataJSON string
		if scanErr := rows.Scan(
			&running.ID,
			&running.SessionID,
			&running.TaskID,
			&running.ExecutorID,
			&running.Runtime,
			&running.Status,
			&running.Resumable,
			&running.ResumeToken,
			&running.LastMessageUUID,
			&running.AgentExecutionID,
			&running.ContainerID,
			&running.AgentctlURL,
			&running.AgentctlPort,
			&running.WorktreeID,
			&running.WorktreePath,
			&running.WorktreeBranch,
			&metadataJSON,
			&running.CreatedAt,
			&running.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if jsonErr := json.Unmarshal([]byte(metadataJSON), &running.Metadata); jsonErr != nil {
				return nil, fmt.Errorf("failed to deserialize executor running metadata: %w", jsonErr)
			}
		}
		results = append(results, running)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	running := &models.ExecutorRunning{}
	var resumable int
	var lastSeen sql.NullTime
	var metadataJSON string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
		       last_message_uuid, agent_execution_id, container_id, agentctl_url, agentctl_port, pid,
		       worktree_id, worktree_path, worktree_branch, last_seen_at, error_message, metadata,
		       created_at, updated_at
		FROM executors_running
		WHERE session_id = ?
	`), sessionID).Scan(
		&running.ID,
		&running.SessionID,
		&running.TaskID,
		&running.ExecutorID,
		&running.Runtime,
		&running.Status,
		&resumable,
		&running.ResumeToken,
		&running.LastMessageUUID,
		&running.AgentExecutionID,
		&running.ContainerID,
		&running.AgentctlURL,
		&running.AgentctlPort,
		&running.PID,
		&running.WorktreeID,
		&running.WorktreePath,
		&running.WorktreeBranch,
		&lastSeen,
		&running.ErrorMessage,
		&metadataJSON,
		&running.CreatedAt,
		&running.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w for session: %s", models.ErrExecutorRunningNotFound, sessionID)
	}
	if err != nil {
		return nil, err
	}
	running.Resumable = resumable == 1
	if lastSeen.Valid {
		running.LastSeenAt = &lastSeen.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if jsonErr := json.Unmarshal([]byte(metadataJSON), &running.Metadata); jsonErr != nil {
			return nil, fmt.Errorf("failed to deserialize executor running metadata: %w", jsonErr)
		}
	}
	return running, nil
}

func (r *Repository) DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM executors_running WHERE session_id = ?`), sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor running not found for session: %s", sessionID)
	}
	return nil
}

func (r *Repository) HasActiveTaskSessionsByExecutor(ctx context.Context, executorID string) (bool, error) {
	var exists int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_sessions
		WHERE executor_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`), executorID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
