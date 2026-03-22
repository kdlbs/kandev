package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateTaskEnvironment creates a new task environment record.
func (r *Repository) CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	if env.ID == "" {
		env.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		env.ID, env.TaskID, env.RepositoryID, env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.AgentExecutionID, env.ControlPort, string(env.Status),
		env.WorktreeID, env.WorktreePath, env.WorktreeBranch, env.WorkspacePath,
		env.ContainerID, env.SandboxID,
		env.CreatedAt, env.UpdatedAt,
	)
	return err
}

// GetTaskEnvironment retrieves a task environment by ID.
func (r *Repository) GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var status string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id,
			created_at, updated_at
		FROM task_environments WHERE id = ?
	`), id).Scan(
		&env.ID, &env.TaskID, &env.RepositoryID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.AgentExecutionID, &env.ControlPort, &status,
		&env.WorktreeID, &env.WorktreePath, &env.WorktreeBranch, &env.WorkspacePath,
		&env.ContainerID, &env.SandboxID,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task environment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	env.Status = models.TaskEnvironmentStatus(status)
	return env, nil
}

// GetTaskEnvironmentByTaskID retrieves the active task environment for a task.
func (r *Repository) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var status string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id,
			created_at, updated_at
		FROM task_environments WHERE task_id = ? ORDER BY created_at DESC LIMIT 1
	`), taskID).Scan(
		&env.ID, &env.TaskID, &env.RepositoryID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.AgentExecutionID, &env.ControlPort, &status,
		&env.WorktreeID, &env.WorktreePath, &env.WorktreeBranch, &env.WorkspacePath,
		&env.ContainerID, &env.SandboxID,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No environment yet — not an error
	}
	if err != nil {
		return nil, err
	}
	env.Status = models.TaskEnvironmentStatus(status)
	return env, nil
}

// UpdateTaskEnvironment updates an existing task environment.
func (r *Repository) UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	env.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments SET
			repository_id = ?, executor_type = ?, executor_id = ?, executor_profile_id = ?,
			agent_execution_id = ?, control_port = ?, status = ?,
			worktree_id = ?, worktree_path = ?, worktree_branch = ?, workspace_path = ?,
			container_id = ?, sandbox_id = ?,
			updated_at = ?
		WHERE id = ?
	`),
		env.RepositoryID, env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.AgentExecutionID, env.ControlPort, string(env.Status),
		env.WorktreeID, env.WorktreePath, env.WorktreeBranch, env.WorkspacePath,
		env.ContainerID, env.SandboxID,
		env.UpdatedAt,
		env.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task environment not found: %s", env.ID)
	}
	return nil
}

// DeleteTaskEnvironment deletes a task environment by ID.
func (r *Repository) DeleteTaskEnvironment(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_environments WHERE id = ?
	`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task environment not found: %s", id)
	}
	return nil
}

// DeleteTaskEnvironmentsByTask deletes all task environments for a given task.
func (r *Repository) DeleteTaskEnvironmentsByTask(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_environments WHERE task_id = ?
	`), taskID)
	return err
}
