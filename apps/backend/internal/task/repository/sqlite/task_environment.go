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
// If env.Repos is non-empty, the per-repo rows are inserted in the same transaction.
func (r *Repository) CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	if env.ID == "" {
		env.ID = uuid.New().String()
	}
	// Worktree-mode envs must always carry workspace_path. Without it,
	// GetOrEnsureExecutionForEnvironment returns ErrSessionWorkspaceNotReady
	// forever and the env terminal handler 503s. Reject at the boundary
	// instead of letting a corrupt row land.
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" {
		return fmt.Errorf("create task environment: worktree-mode env requires workspace_path (task=%s)", env.TaskID)
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, task_dir_name,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		env.ID, env.TaskID, env.RepositoryID, env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.AgentExecutionID, env.ControlPort, string(env.Status),
		env.WorktreeID, env.WorktreePath, env.WorktreeBranch, env.WorkspacePath,
		env.ContainerID, env.SandboxID, env.TaskDirName,
		env.CreatedAt, env.UpdatedAt,
	); err != nil {
		return err
	}

	for _, repo := range env.Repos {
		repo.TaskEnvironmentID = env.ID
		if err := r.insertTaskEnvironmentRepoTx(ctx, tx, repo); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTaskEnvironment retrieves a task environment by ID, including per-repo rows.
func (r *Repository) GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var status string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, COALESCE(task_dir_name, ''),
			created_at, updated_at
		FROM task_environments WHERE id = ?
	`), id).Scan(
		&env.ID, &env.TaskID, &env.RepositoryID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.AgentExecutionID, &env.ControlPort, &status,
		&env.WorktreeID, &env.WorktreePath, &env.WorktreeBranch, &env.WorkspacePath,
		&env.ContainerID, &env.SandboxID, &env.TaskDirName,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task environment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	env.Status = models.TaskEnvironmentStatus(status)

	repos, err := r.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	env.Repos = repos
	return env, nil
}

// GetTaskEnvironmentByTaskID retrieves the active task environment for a task,
// including per-repo rows.
func (r *Repository) GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var status string

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, COALESCE(task_dir_name, ''),
			created_at, updated_at
		FROM task_environments WHERE task_id = ? ORDER BY created_at DESC LIMIT 1
	`), taskID).Scan(
		&env.ID, &env.TaskID, &env.RepositoryID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.AgentExecutionID, &env.ControlPort, &status,
		&env.WorktreeID, &env.WorktreePath, &env.WorktreeBranch, &env.WorkspacePath,
		&env.ContainerID, &env.SandboxID, &env.TaskDirName,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No environment yet — not an error
	}
	if err != nil {
		return nil, err
	}
	env.Status = models.TaskEnvironmentStatus(status)

	repos, err := r.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	env.Repos = repos
	return env, nil
}

// UpdateTaskEnvironment updates an existing task environment.
// Per-repo rows are not touched; use the TaskEnvironmentRepo CRUD methods.
func (r *Repository) UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	// Refuse to clear workspace_path on a worktree-mode env. Same rationale
	// as CreateTaskEnvironment: empty workspace_path produces permanent 503
	// on shell terminal connect.
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" {
		return fmt.Errorf("update task environment: worktree-mode env requires workspace_path (id=%s)", env.ID)
	}
	env.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments SET
			repository_id = ?, executor_type = ?, executor_id = ?, executor_profile_id = ?,
			agent_execution_id = ?, control_port = ?, status = ?,
			worktree_id = ?, worktree_path = ?, worktree_branch = ?, workspace_path = ?,
			container_id = ?, sandbox_id = ?, task_dir_name = ?,
			updated_at = ?
		WHERE id = ?
	`),
		env.RepositoryID, env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.AgentExecutionID, env.ControlPort, string(env.Status),
		env.WorktreeID, env.WorktreePath, env.WorktreeBranch, env.WorkspacePath,
		env.ContainerID, env.SandboxID, env.TaskDirName,
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
// Per-repo rows are removed via ON DELETE CASCADE.
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

// CreateTaskEnvironmentRepo inserts a single per-repo environment row.
func (r *Repository) CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error {
	if repo.TaskEnvironmentID == "" {
		return fmt.Errorf("task_environment_id is required")
	}
	if repo.RepositoryID == "" {
		return fmt.Errorf("repository_id is required")
	}
	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repo.CreatedAt = now
	repo.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id,
			worktree_id, worktree_path, worktree_branch,
			position, error_message, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		repo.ID, repo.TaskEnvironmentID, repo.RepositoryID,
		repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.CreatedAt, repo.UpdatedAt,
	)
	return err
}

// insertTaskEnvironmentRepoTx is the in-transaction variant of CreateTaskEnvironmentRepo.
func (r *Repository) insertTaskEnvironmentRepoTx(ctx context.Context, tx *sql.Tx, repo *models.TaskEnvironmentRepo) error {
	if repo.RepositoryID == "" {
		return fmt.Errorf("repository_id is required")
	}
	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repo.CreatedAt = now
	repo.UpdatedAt = now

	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id,
			worktree_id, worktree_path, worktree_branch,
			position, error_message, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		repo.ID, repo.TaskEnvironmentID, repo.RepositoryID,
		repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.CreatedAt, repo.UpdatedAt,
	)
	return err
}

// ListTaskEnvironmentRepos returns all per-repo rows for an environment, ordered by position.
func (r *Repository) ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_environment_id, repository_id,
			worktree_id, worktree_path, worktree_branch,
			position, error_message, created_at, updated_at
		FROM task_environment_repos
		WHERE task_environment_id = ?
		ORDER BY position ASC, created_at ASC
	`), envID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskEnvironmentRepo
	for rows.Next() {
		repo := &models.TaskEnvironmentRepo{}
		if err := rows.Scan(
			&repo.ID, &repo.TaskEnvironmentID, &repo.RepositoryID,
			&repo.WorktreeID, &repo.WorktreePath, &repo.WorktreeBranch,
			&repo.Position, &repo.ErrorMessage, &repo.CreatedAt, &repo.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, repo)
	}
	return out, rows.Err()
}

// UpdateTaskEnvironmentRepo updates an existing per-repo row.
func (r *Repository) UpdateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error {
	repo.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environment_repos SET
			worktree_id = ?, worktree_path = ?, worktree_branch = ?,
			position = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`),
		repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.UpdatedAt,
		repo.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task environment repo not found: %s", repo.ID)
	}
	return nil
}

// DeleteTaskEnvironmentRepo removes a single per-repo row.
func (r *Repository) DeleteTaskEnvironmentRepo(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_environment_repos WHERE id = ?
	`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task environment repo not found: %s", id)
	}
	return nil
}

// DeleteTaskEnvironmentReposByEnv removes all per-repo rows for an environment.
func (r *Repository) DeleteTaskEnvironmentReposByEnv(ctx context.Context, envID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_environment_repos WHERE task_environment_id = ?
	`), envID)
	return err
}
