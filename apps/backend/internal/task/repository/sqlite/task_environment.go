package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateTaskEnvironment creates a new task environment record.
// If env.Repos is non-empty, the per-repo rows are inserted in the same transaction.
func (r *Repository) CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	if env.ID == "" {
		env.ID = uuid.New().String()
	}
	// A creating worktree environment has no path until its elected owner
	// materializes it. Ready/stopped environments still require one.
	// Without it,
	// GetOrEnsureExecutionForEnvironment returns ErrSessionWorkspaceNotReady
	// forever and the env terminal handler 503s. Reject at the boundary
	// instead of letting a corrupt row land.
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" && env.Status != models.TaskEnvironmentStatusCreating {
		return fmt.Errorf("create task environment: worktree-mode env requires workspace_path (task=%s)", env.TaskID)
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize environment creation against the task cleanup barrier so a
	// worktree admitted after inventory capture cannot be missed.
	if err := r.taskCleanupBarrierLocked(ctx, tx, env.TaskID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, executor_type, executor_id, executor_profile_id,
			control_port, status, materialization_session_id,
			workspace_path,
			container_id, container_bootstrap_nonce_secret_id, container_control_auth_token_secret_id, sandbox_id, task_dir_name,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		env.ID, env.TaskID, env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.ControlPort, string(env.Status), env.MaterializationSessionID,
		env.WorkspacePath,
		env.ContainerID, env.ContainerBootstrapNonceSecretID, env.ContainerControlAuthTokenSecretID, env.SandboxID, env.TaskDirName,
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
		SELECT id, task_id, executor_type, executor_id, executor_profile_id,
			control_port, status, materialization_session_id,
			workspace_path,
			container_id, COALESCE(container_bootstrap_nonce_secret_id, ''), COALESCE(container_control_auth_token_secret_id, ''), sandbox_id, COALESCE(task_dir_name, ''),
			created_at, updated_at
		FROM task_environments WHERE id = ?
	`), id).Scan(
		&env.ID, &env.TaskID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.ControlPort, &status, &env.MaterializationSessionID,
		&env.WorkspacePath,
		&env.ContainerID, &env.ContainerBootstrapNonceSecretID, &env.ContainerControlAuthTokenSecretID, &env.SandboxID, &env.TaskDirName,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, id)
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
		SELECT id, task_id, executor_type, executor_id, executor_profile_id,
			control_port, status, materialization_session_id,
			workspace_path,
			container_id, COALESCE(container_bootstrap_nonce_secret_id, ''), COALESCE(container_control_auth_token_secret_id, ''), sandbox_id, COALESCE(task_dir_name, ''),
			created_at, updated_at
		FROM task_environments WHERE task_id = ? ORDER BY created_at DESC LIMIT 1
	`), taskID).Scan(
		&env.ID, &env.TaskID, &env.ExecutorType, &env.ExecutorID, &env.ExecutorProfileID,
		&env.ControlPort, &status, &env.MaterializationSessionID,
		&env.WorkspacePath,
		&env.ContainerID, &env.ContainerBootstrapNonceSecretID, &env.ContainerControlAuthTokenSecretID, &env.SandboxID, &env.TaskDirName,
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
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" && env.Status != models.TaskEnvironmentStatusCreating {
		return fmt.Errorf("update task environment: worktree-mode env requires workspace_path (id=%s)", env.ID)
	}
	env.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments SET
			executor_type = ?, executor_id = ?, executor_profile_id = ?,
			control_port = ?, status = ?, materialization_session_id = ?,
			workspace_path = ?,
			container_id = ?, container_bootstrap_nonce_secret_id = ?, container_control_auth_token_secret_id = ?, sandbox_id = ?, task_dir_name = ?,
			updated_at = ?
		WHERE id = ?
	`),
		env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.ControlPort, string(env.Status), env.MaterializationSessionID,
		env.WorkspacePath,
		env.ContainerID, env.ContainerBootstrapNonceSecretID, env.ContainerControlAuthTokenSecretID, env.SandboxID, env.TaskDirName,
		env.UpdatedAt,
		env.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, env.ID)
	}
	return nil
}

// FinalizeTaskEnvironmentMaterialization writes the complete canonical
// repository inventory and transitions the elected creating environment to
// ready in one transaction. The materializer ID prevents a sibling session
// from publishing another session's workspace.
func (r *Repository) FinalizeTaskEnvironmentMaterialization(
	ctx context.Context,
	env *models.TaskEnvironment,
	repos []*models.TaskEnvironmentRepo,
	materializationSessionID string,
) error {
	if env == nil || env.ID == "" || materializationSessionID == "" {
		return fmt.Errorf("finalize task environment: environment and materializer are required")
	}
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" {
		return fmt.Errorf("finalize task environment: worktree-mode env requires workspace_path (id=%s)", env.ID)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var taskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_environments WHERE id = ?`), env.ID).Scan(&taskID); err != nil {
		return fmt.Errorf("finalize task environment: resolve owner: %w", err)
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_environment_repos WHERE task_environment_id = ?`), env.ID); err != nil {
		return err
	}
	for _, repo := range repos {
		repo.TaskEnvironmentID = env.ID
		if err := r.insertTaskEnvironmentRepoTx(ctx, tx, repo); err != nil {
			return err
		}
	}
	env.UpdatedAt = time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments SET
			executor_type = ?, executor_id = ?, executor_profile_id = ?,
			control_port = ?, status = ?, materialization_session_id = '',
			workspace_path = ?, container_id = ?,
			container_bootstrap_nonce_secret_id = ?, container_control_auth_token_secret_id = ?,
			sandbox_id = ?, task_dir_name = ?, updated_at = ?
		WHERE id = ? AND status = ? AND materialization_session_id = ?
	`),
		env.ExecutorType, env.ExecutorID, env.ExecutorProfileID,
		env.ControlPort, string(models.TaskEnvironmentStatusReady),
		env.WorkspacePath, env.ContainerID,
		env.ContainerBootstrapNonceSecretID, env.ContainerControlAuthTokenSecretID,
		env.SandboxID, env.TaskDirName, env.UpdatedAt,
		env.ID, string(models.TaskEnvironmentStatusCreating), materializationSessionID,
	)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("finalize task environment: materialization claim was not current")
	}
	return tx.Commit()
}

func (r *Repository) TransferTaskEnvironmentToTask(ctx context.Context, envID, taskID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments SET
			task_id = ?,
			updated_at = ?
		WHERE id = ?
	`), taskID, time.Now().UTC(), envID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, envID)
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
		return fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, id)
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

// CreateTaskEnvironmentRepo inserts a single per-repo environment row. The
// owning task is resolved through the environment and serialized against the
// task cleanup barrier before the insert commits.
func (r *Repository) CreateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error {
	if repo.TaskEnvironmentID == "" {
		return fmt.Errorf("task_environment_id is required")
	}
	if repo.RepositoryID == "" {
		return fmt.Errorf("repository_id is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var taskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id FROM task_environments WHERE id = ?`), repo.TaskEnvironmentID).Scan(&taskID); err != nil {
		return fmt.Errorf("resolve environment owner %s: %w", repo.TaskEnvironmentID, err)
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return err
	}

	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repo.CreatedAt = now
	repo.UpdatedAt = now
	if repo.Status == "" {
		repo.Status = worktreeRepoStatusActive
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch,
			position, error_message, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		repo.ID, repo.TaskEnvironmentID, repo.RepositoryID, repo.BranchSlug,
		repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.Status, repo.CreatedAt, repo.UpdatedAt,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// insertTaskEnvironmentRepoTx is the in-transaction variant of CreateTaskEnvironmentRepo.
func (r *Repository) insertTaskEnvironmentRepoTx(ctx context.Context, tx *sqlx.Tx, repo *models.TaskEnvironmentRepo) error {
	if repo.RepositoryID == "" {
		return fmt.Errorf("repository_id is required")
	}
	if repo.ID == "" {
		repo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repo.CreatedAt = now
	repo.UpdatedAt = now
	if repo.Status == "" {
		repo.Status = worktreeRepoStatusActive
	}

	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch,
			position, error_message, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		repo.ID, repo.TaskEnvironmentID, repo.RepositoryID, repo.BranchSlug,
		repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.Status, repo.CreatedAt, repo.UpdatedAt,
	)
	return err
}

// ListTaskEnvironmentRepos returns all per-repo rows for an environment, ordered by position.
func (r *Repository) ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_environment_id, repository_id,
			COALESCE(branch_slug, ''),
			worktree_id, worktree_path, worktree_branch,
			position, error_message, COALESCE(status, ''),
			created_at, updated_at, merged_at, deleted_at
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
		var mergedAt, deletedAt sql.NullTime
		if err := rows.Scan(
			&repo.ID, &repo.TaskEnvironmentID, &repo.RepositoryID,
			&repo.BranchSlug,
			&repo.WorktreeID, &repo.WorktreePath, &repo.WorktreeBranch,
			&repo.Position, &repo.ErrorMessage, &repo.Status,
			&repo.CreatedAt, &repo.UpdatedAt, &mergedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if mergedAt.Valid {
			t := mergedAt.Time
			repo.MergedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			repo.DeletedAt = &t
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
			branch_slug = ?,
			worktree_id = ?, worktree_path = ?, worktree_branch = ?,
			position = ?, error_message = ?, status = ?,
			merged_at = ?, deleted_at = ?, updated_at = ?
		WHERE id = ?
	`),
		repo.BranchSlug, repo.WorktreeID, repo.WorktreePath, repo.WorktreeBranch,
		repo.Position, repo.ErrorMessage, repo.Status,
		repo.MergedAt, repo.DeletedAt, repo.UpdatedAt,
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
