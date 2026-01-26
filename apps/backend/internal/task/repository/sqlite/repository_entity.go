package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateRepository creates a new repository
func (r *Repository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	if repository.ID == "" {
		repository.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repository.CreatedAt = now
	repository.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO repositories (
			id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
			provider_name, default_branch, worktree_branch_prefix, pull_before_worktree, setup_script, cleanup_script, dev_script, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, repository.ID, repository.WorkspaceID, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider,
		repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch, repository.WorktreeBranchPrefix,
		boolToInt(repository.PullBeforeWorktree), repository.SetupScript, repository.CleanupScript, repository.DevScript, repository.CreatedAt, repository.UpdatedAt, repository.DeletedAt)

	return err
}

// GetRepository retrieves a repository by ID
func (r *Repository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repository := &models.Repository{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, worktree_branch_prefix, pull_before_worktree, setup_script, cleanup_script, dev_script, created_at, updated_at, deleted_at
		FROM repositories WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repository, err
}

// UpdateRepository updates an existing repository
func (r *Repository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	repository.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE repositories SET
			name = ?, source_type = ?, local_path = ?, provider = ?, provider_repo_id = ?, provider_owner = ?,
			provider_name = ?, default_branch = ?, worktree_branch_prefix = ?, pull_before_worktree = ?, setup_script = ?, cleanup_script = ?, dev_script = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider, repository.ProviderRepoID,
		repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch, repository.WorktreeBranchPrefix, boolToInt(repository.PullBeforeWorktree),
		repository.SetupScript, repository.CleanupScript, repository.DevScript, repository.UpdatedAt, repository.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", repository.ID)
	}
	return nil
}

// DeleteRepository soft-deletes a repository by ID
func (r *Repository) DeleteRepository(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE repositories SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", id)
	}
	return nil
}

// ListRepositories returns all repositories for a workspace
func (r *Repository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, worktree_branch_prefix, pull_before_worktree, setup_script, cleanup_script, dev_script, created_at, updated_at, deleted_at
		FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Repository
	for rows.Next() {
		repository := &models.Repository{}
		err := rows.Scan(
			&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
			&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
			&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, rows.Err()
}

// CreateRepositoryScript creates a new repository script
func (r *Repository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	if script.ID == "" {
		script.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	script.CreatedAt = now
	script.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO repository_scripts (id, repository_id, name, command, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, script.ID, script.RepositoryID, script.Name, script.Command, script.Position, script.CreatedAt, script.UpdatedAt)

	return err
}

// GetRepositoryScript retrieves a repository script by ID
func (r *Repository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	script := &models.RepositoryScript{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE id = ?
	`, id).Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository script not found: %s", id)
	}
	return script, err
}

// UpdateRepositoryScript updates an existing repository script
func (r *Repository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	script.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE repository_scripts SET name = ?, command = ?, position = ?, updated_at = ? WHERE id = ?
	`, script.Name, script.Command, script.Position, script.UpdatedAt, script.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository script not found: %s", script.ID)
	}
	return nil
}

// DeleteRepositoryScript deletes a repository script by ID
func (r *Repository) DeleteRepositoryScript(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM repository_scripts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository script not found: %s", id)
	}
	return nil
}

// ListRepositoryScripts returns all scripts for a repository
func (r *Repository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE repository_id = ? ORDER BY position
	`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.RepositoryScript
	for rows.Next() {
		script := &models.RepositoryScript{}
		err := rows.Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, script)
	}
	return result, rows.Err()
}
