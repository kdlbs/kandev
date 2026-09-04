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

	// A ready environment with zero repo rows is indistinguishable from "the
	// canonical inventory was never captured" and permanently bricks reuse
	// (validateReuseEnvironmentInventory refuses every later launch). Reject
	// at the boundary instead of trusting the caller: only a task with no
	// repositories configured at all may legitimately have none.
	if env.Status == models.TaskEnvironmentStatusReady && len(env.Repos) == 0 {
		hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, env.TaskID)
		if err != nil {
			return err
		}
		if hasRepos {
			return fmt.Errorf("create task environment: ready status requires repository inventory for repo-backed task (task=%s)", env.TaskID)
		}
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
	// Creating and failed worktree environments can lack a path because
	// materialization can fail before one exists. Reusable states require it.
	if env.ExecutorType == string(models.ExecutorTypeWorktree) && env.WorkspacePath == "" &&
		env.Status != models.TaskEnvironmentStatusCreating && env.Status != models.TaskEnvironmentStatusFailed {
		return fmt.Errorf("update task environment: worktree-mode env requires workspace_path (id=%s)", env.ID)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if env.Status == models.TaskEnvironmentStatusReady {
		if err := r.validateReadyTaskEnvironment(ctx, tx, env.ID); err != nil {
			return err
		}
	}
	env.UpdatedAt = time.Now().UTC()

	result, err := tx.ExecContext(ctx, r.db.Rebind(`
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
	return tx.Commit()
}

func (r *Repository) validateReadyTaskEnvironment(ctx context.Context, tx *sqlx.Tx, environmentID string) error {
	taskID, currentStatus, err := r.taskEnvironmentStateTx(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return err
	}
	// Workspace-source materialization updates an already-ready environment
	// after it adds a folder or repository. That update does not publish a new
	// ready state, so it must remain compatible with legacy environments whose
	// canonical inventory is still empty. Launch persistence separately rejects
	// that state before it calls this repository method.
	if currentStatus == models.TaskEnvironmentStatusReady {
		return nil
	}
	hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, taskID)
	if err != nil || !hasRepos {
		return err
	}
	var inventoryCount int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(1) FROM task_environment_repos WHERE task_environment_id = ?`), environmentID).Scan(&inventoryCount); err != nil {
		return err
	}
	if inventoryCount == 0 {
		return fmt.Errorf("update task environment: ready status requires repository inventory for repo-backed task (id=%s)", environmentID)
	}
	return nil
}

func (r *Repository) taskEnvironmentStateTx(ctx context.Context, tx *sqlx.Tx, environmentID string) (string, models.TaskEnvironmentStatus, error) {
	var taskID string
	var status string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, status FROM task_environments WHERE id = ?`), environmentID).Scan(&taskID, &status)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, environmentID)
	}
	return taskID, models.TaskEnvironmentStatus(status), err
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
	// See the matching guard in CreateTaskEnvironment: a materializer that
	// publishes ready with an empty inventory (e.g. because its prepare step
	// failed and produced no repos) must not be allowed to land, or the
	// environment becomes permanently unreusable.
	if len(repos) == 0 {
		hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if hasRepos {
			return fmt.Errorf("finalize task environment: ready status requires repository inventory for repo-backed task (id=%s)", env.ID)
		}
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

// PersistTaskEnvironmentTransition atomically updates an existing environment
// and reconciles its repository inventory. A transition must not leave the old
// executor attached to a partially replaced inventory when any write fails.
// When replacePhysical is true, active rows absent from repos become deleted
// tombstones so old worktree paths cannot re-enter a later launch.
func (r *Repository) PersistTaskEnvironmentTransition(
	ctx context.Context,
	env *models.TaskEnvironment,
	repos []*models.TaskEnvironmentRepo,
	replacePhysical bool,
) error {
	if env == nil || env.ID == "" {
		return fmt.Errorf("persist task environment transition: environment is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	taskID, _, err := r.taskEnvironmentStateTx(ctx, tx, env.ID)
	if err != nil {
		return err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return err
	}
	existing, err := listTaskEnvironmentReposTx(ctx, r, tx, env.ID)
	if err != nil {
		return err
	}
	if err := r.validateTaskEnvironmentTransitionWorkspace(ctx, tx, taskID, env, existing); err != nil {
		return err
	}
	if err := r.reconcileTaskEnvironmentReposTx(ctx, tx, env.ID, existing, repos, replacePhysical); err != nil {
		return err
	}
	if err := r.validateTaskEnvironmentTransitionReady(ctx, tx, env.ID, env.Status); err != nil {
		return err
	}
	if err := r.updateTaskEnvironmentTransitionTx(ctx, tx, env); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) validateTaskEnvironmentTransitionWorkspace(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	env *models.TaskEnvironment,
	existing []*models.TaskEnvironmentRepo,
) error {
	if env.ExecutorType != string(models.ExecutorTypeWorktree) || env.WorkspacePath != "" {
		return nil
	}
	hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if hasRepos || len(existing) > 0 {
		return fmt.Errorf("persist task environment transition: worktree-mode env requires workspace_path (id=%s)", env.ID)
	}
	return nil
}

func (r *Repository) validateTaskEnvironmentTransitionReady(
	ctx context.Context,
	tx *sqlx.Tx,
	environmentID string,
	status models.TaskEnvironmentStatus,
) error {
	if status != models.TaskEnvironmentStatusReady {
		return nil
	}
	return r.validateReadyTaskEnvironment(ctx, tx, environmentID)
}

func (r *Repository) updateTaskEnvironmentTransitionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	env *models.TaskEnvironment,
) error {
	env.UpdatedAt = time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
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
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("%w: %s", ErrTaskEnvironmentNotFound, env.ID)
	}
	return nil
}

func listTaskEnvironmentReposTx(ctx context.Context, r *Repository, tx *sqlx.Tx, envID string) ([]*models.TaskEnvironmentRepo, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT `+envRepoSelectCols+`
		FROM task_environment_repos ter
		WHERE ter.task_environment_id = ?
		ORDER BY ter.position ASC, ter.created_at ASC
	`), envID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []*models.TaskEnvironmentRepo
	for rows.Next() {
		row, err := scanEnvRepoRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *Repository) reconcileTaskEnvironmentReposTx(
	ctx context.Context,
	tx *sqlx.Tx,
	envID string,
	existing, repos []*models.TaskEnvironmentRepo,
	replacePhysical bool,
) error {
	byKey, legacyFlatByRepo := indexTaskEnvironmentReposTx(existing)
	matched := make(map[string]struct{}, len(repos))
	for position, incoming := range repos {
		row := findTaskEnvironmentRepoTransition(byKey, legacyFlatByRepo, incoming)
		if row == nil {
			if incoming == nil || incoming.RepositoryID == "" {
				continue
			}
			incoming.TaskEnvironmentID = envID
			incoming.Position = position
			if err := r.insertTaskEnvironmentRepoTx(ctx, tx, incoming); err != nil {
				return err
			}
			continue
		}
		matched[row.ID] = struct{}{}
		if err := r.updateTaskEnvironmentRepoTransitionTx(ctx, tx, row, incoming, position, replacePhysical); err != nil {
			return err
		}
	}
	if !replacePhysical {
		return nil
	}
	return r.tombstoneOmittedTaskEnvironmentReposTx(ctx, tx, existing, matched)
}

func indexTaskEnvironmentReposTx(rows []*models.TaskEnvironmentRepo) (map[string]*models.TaskEnvironmentRepo, map[string]*models.TaskEnvironmentRepo) {
	byKey := make(map[string]*models.TaskEnvironmentRepo, len(rows))
	legacyFlatByRepo := make(map[string]*models.TaskEnvironmentRepo)
	for _, row := range rows {
		if row == nil {
			continue
		}
		byKey[row.RepositoryID+"\x00"+row.BranchSlug] = row
		if row.RepositoryID != "" && row.BranchSlug == "" {
			legacyFlatByRepo[row.RepositoryID] = row
		}
	}
	return byKey, legacyFlatByRepo
}

func findTaskEnvironmentRepoTransition(
	byKey map[string]*models.TaskEnvironmentRepo,
	legacyFlatByRepo map[string]*models.TaskEnvironmentRepo,
	incoming *models.TaskEnvironmentRepo,
) *models.TaskEnvironmentRepo {
	if incoming == nil || incoming.RepositoryID == "" {
		return nil
	}
	key := incoming.RepositoryID + "\x00" + incoming.BranchSlug
	row := byKey[key]
	if row == nil && incoming.BranchSlug != "" {
		row = legacyFlatByRepo[incoming.RepositoryID]
		if row != nil {
			delete(legacyFlatByRepo, incoming.RepositoryID)
			byKey[key] = row
		}
	}
	return row
}

func (r *Repository) updateTaskEnvironmentRepoTransitionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	row, incoming *models.TaskEnvironmentRepo,
	position int,
	replacePhysical bool,
) error {
	row.BranchSlug = incoming.BranchSlug
	if replacePhysical || incoming.WorktreeID != "" {
		row.WorktreeID = incoming.WorktreeID
		row.WorktreePath = incoming.WorktreePath
		row.WorktreeBranch = incoming.WorktreeBranch
	}
	row.Position = position
	row.ErrorMessage = incoming.ErrorMessage
	// A later successful transition can recreate a slot that an earlier
	// replace-mode transition tombstoned. Rebind the existing row to the
	// active inventory instead of leaving the matching key permanently
	// excluded from reuse.
	if replacePhysical {
		row.Status = worktreeRepoStatusActive
		row.DeletedAt = nil
	}
	row.UpdatedAt = time.Now().UTC()
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environment_repos SET
			branch_slug = ?, worktree_id = ?, worktree_path = ?, worktree_branch = ?,
			position = ?, error_message = ?, status = ?, deleted_at = ?, updated_at = ?
		WHERE id = ?
	`), row.BranchSlug, row.WorktreeID, row.WorktreePath, row.WorktreeBranch,
		row.Position, row.ErrorMessage, row.Status, row.DeletedAt, row.UpdatedAt, row.ID)
	return err
}

func (r *Repository) tombstoneOmittedTaskEnvironmentReposTx(
	ctx context.Context,
	tx *sqlx.Tx,
	existing []*models.TaskEnvironmentRepo,
	matched map[string]struct{},
) error {
	deletedAt := time.Now().UTC()
	for _, row := range existing {
		if row == nil || row.DeletedAt != nil || row.Status == worktreeRepoStatusDeleted {
			continue
		}
		if _, ok := matched[row.ID]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_environment_repos
			SET status = ?, deleted_at = ?, updated_at = ?
			WHERE id = ?
		`), worktreeRepoStatusDeleted, deletedAt, deletedAt, row.ID); err != nil {
			return err
		}
	}
	return nil
}

// taskHasRepositoriesTx reports whether a task has any task_repositories
// rows, i.e. whether it is repo-backed and therefore expected to carry a
// non-empty canonical inventory whenever its environment is ready.
func (r *Repository) taskHasRepositoriesTx(ctx context.Context, tx *sqlx.Tx, taskID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(1) FROM task_repositories WHERE task_id = ?`), taskID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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
