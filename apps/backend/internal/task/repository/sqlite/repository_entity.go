package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// CreateRepository creates a new repository
func (r *Repository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	return r.insertRepository(ctx, r.db, repository)
}

func (r *Repository) insertRepository(ctx context.Context, exec sqlx.ExtContext, repository *models.Repository) error {
	if repository.ID == "" {
		repository.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repository.CreatedAt = now
	repository.UpdatedAt = now

	_, err := exec.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO repositories (
			id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
			provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), repository.ID, repository.WorkspaceID, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider,
		repository.ProviderRepoID, repository.ProviderHost, repository.ProviderScope, repository.ProviderOwner, repository.ProviderName, repository.RemoteURL, repository.DefaultBranch, repository.WorktreeBranchPrefix,
		repository.WorktreeBranchTemplate, dialect.BoolToInt(repository.PullBeforeWorktree), repository.SetupScript, repository.CleanupScript, repository.DevScript, repository.CopyFiles, repository.CreatedAt, repository.UpdatedAt, repository.DeletedAt)

	return err
}

// CreateRepositoryWithSecretBindings persists a repository and its complete
// binding set in one transaction.
func (r *Repository) CreateRepositoryWithSecretBindings(
	ctx context.Context, repository *models.Repository, bindings []models.RepositorySecretBinding,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.insertRepository(ctx, tx, repository); err != nil {
		return err
	}
	if err := insertRepositorySecretBindings(ctx, r.db, tx, repository.ID, bindings); err != nil {
		return err
	}
	return tx.Commit()
}

// GetRepository retrieves a repository by ID
func (r *Repository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repository := &models.Repository{}

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
		       provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		FROM repositories WHERE id = ? AND deleted_at IS NULL
	`), id).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderHost, &repository.ProviderScope, &repository.ProviderOwner, &repository.ProviderName, &repository.RemoteURL,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", repoerrors.ErrRepositoryNotFound, id)
	}
	if err == nil {
		if err := r.attachRepositorySecretBindings(ctx, repository); err != nil {
			return nil, err
		}
	}
	return repository, err
}

// UpdateRepository updates an existing repository
func (r *Repository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	return r.updateRepository(ctx, r.db, repository)
}

// UpdateRepositoryDefaultBranch updates only the default branch while the
// caller's previously read value is still current. This protects unrelated
// repository fields from stale recovery writes.
func (r *Repository) UpdateRepositoryDefaultBranch(ctx context.Context, repositoryID, expectedBranch, branch string) error {
	updatedAt := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories
		SET default_branch = ?, updated_at = ?
		WHERE id = ? AND default_branch = ? AND deleted_at IS NULL
	`), branch, updatedAt, repositoryID, expectedBranch)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("repository default branch changed or repository not found: %s", repositoryID)
	}
	return nil
}

func (r *Repository) updateRepository(ctx context.Context, exec sqlx.ExtContext, repository *models.Repository) error {
	repository.UpdatedAt = time.Now().UTC()

	result, err := exec.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories SET
			name = ?, source_type = ?, local_path = ?, provider = ?, provider_repo_id = ?, provider_host = ?, provider_scope = ?, provider_owner = ?,
			provider_name = ?, remote_url = ?, default_branch = ?, worktree_branch_prefix = ?, worktree_branch_template = ?, pull_before_worktree = ?, setup_script = ?, cleanup_script = ?, dev_script = ?, copy_files = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`), repository.Name, repository.SourceType, repository.LocalPath, repository.Provider, repository.ProviderRepoID,
		repository.ProviderHost, repository.ProviderScope, repository.ProviderOwner, repository.ProviderName, repository.RemoteURL, repository.DefaultBranch, repository.WorktreeBranchPrefix, repository.WorktreeBranchTemplate, dialect.BoolToInt(repository.PullBeforeWorktree),
		repository.SetupScript, repository.CleanupScript, repository.DevScript, repository.CopyFiles, repository.UpdatedAt, repository.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", repository.ID)
	}
	return nil
}

// UpdateRepositoryWithSecretBindings replaces bindings atomically with the
// repository mutation.
func (r *Repository) UpdateRepositoryWithSecretBindings(
	ctx context.Context, repository *models.Repository, bindings []models.RepositorySecretBinding,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.updateRepository(ctx, tx, repository); err != nil {
		return err
	}
	if err := insertRepositorySecretBindings(ctx, r.db, tx, repository.ID, bindings); err != nil {
		return err
	}
	return tx.Commit()
}

// pruneRepositoryDependents removes the rows that must not outlive a deleted
// repository. Repository deletion is *soft* (deleted_at), so declared foreign
// key cascades never fire for it and these rows have to go explicitly. All three
// delete paths call this, so a fourth dependent table cannot be wired into one
// path and forgotten in the others.
func (r *Repository) pruneRepositoryDependents(ctx context.Context, tx *sqlx.Tx, id string) error {
	statements := []string{
		`DELETE FROM repository_secret_bindings WHERE repository_id = ?`,
		`DELETE FROM repository_branch_policies WHERE repository_id = ?`,
		// A membership row pointing at a deleted repository would offer the user
		// a repository they can no longer select. Repository sets keep existing
		// with their remaining members.
		`DELETE FROM repository_set_items WHERE repository_id = ?`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(statement), id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRepository soft-deletes a repository by ID
func (r *Repository) DeleteRepository(ctx context.Context, id string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`), now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", id)
	}
	if err := r.pruneRepositoryDependents(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRepositoryIfUnreferenced soft-deletes only an unadopted repository.
// Keeping the reference check inside UPDATE closes the cleanup/adoption race.
func (r *Repository) DeleteRepositoryIfUnreferenced(ctx context.Context, id string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories
		SET deleted_at = ?, updated_at = ?
		WHERE id = ?
			AND deleted_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM task_repositories WHERE repository_id = repositories.id
			)
	`), now, now, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows > 0 {
		if err := r.pruneRepositoryDependents(ctx, tx, id); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
	}
	return rows > 0, nil
}

// DeleteRepositoryIfNoActiveTaskSessions soft-deletes a repository only when
// no live task linked to it has a session that blocks deletion. Keeping the
// predicate in the UPDATE prevents a session from becoming active between a
// separate check and the delete.
func (r *Repository) DeleteRepositoryIfNoActiveTaskSessions(ctx context.Context, id string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories
		SET deleted_at = ?, updated_at = ?
		WHERE id = ?
			AND deleted_at IS NULL
			AND NOT EXISTS (
				SELECT 1
				FROM task_sessions s
				INNER JOIN task_repositories tr ON tr.task_id = s.task_id
				INNER JOIN tasks t ON t.id = s.task_id
				WHERE tr.repository_id = repositories.id
					AND t.archived_at IS NULL
					AND s.state IN ('CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT')
			)
	`), now, now, id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows > 0 {
		if err := r.pruneRepositoryDependents(ctx, tx, id); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
	}
	return rows > 0, nil
}

// ListRepositories returns all repositories for a workspace
func (r *Repository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
		       provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Repository
	for rows.Next() {
		repository := &models.Repository{}
		err := rows.Scan(
			&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
			&repository.Provider, &repository.ProviderRepoID, &repository.ProviderHost, &repository.ProviderScope, &repository.ProviderOwner, &repository.ProviderName, &repository.RemoteURL,
			&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachRepositorySecretBindingsBatch(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetRepositoryByProviderIdentity finds a repository by its durable provider
// identity. Scope + RepositoryID are authoritative when both are present;
// legacy origin/owner/name matching is used only when both are absent. This
// deliberately refuses to adopt an old unscoped row for a new scoped provider.
// Returns nil, nil if not found. When duplicate rows share the same provider
// identity (e.g. left behind by a resolver race that predates
// Service.repoResolveMu), orders by created_at then id so the row returned
// here is always the same earliest-created row that
// dedupeRepositoriesByIdentity keeps as the canonical winner in
// ListRepositories — otherwise a caller could resolve to, and write
// backfilled fields onto, a duplicate that ListRepositories hides.
func (r *Repository) GetRepositoryByProviderIdentity(
	ctx context.Context, identity models.ProviderRepositoryIdentity,
) (*models.Repository, error) {
	repository := &models.Repository{}
	where := `workspace_id = ? AND provider = ? AND provider_host = ?
			AND provider_owner = ? AND provider_name = ?`
	args := []any{identity.WorkspaceID, identity.Provider, identity.Host, identity.Owner, identity.Name}
	if identity.Scope != "" {
		if identity.RepositoryID == "" {
			return nil, nil
		}
		where = `workspace_id = ? AND provider = ? AND provider_scope = ? AND provider_repo_id = ?`
		args = []any{identity.WorkspaceID, identity.Provider, identity.Scope, identity.RepositoryID}
	}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
		       provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		FROM repositories
		WHERE `+where+` AND deleted_at IS NULL
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`), args...).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderHost, &repository.ProviderScope, &repository.ProviderOwner, &repository.ProviderName, &repository.RemoteURL,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil {
		if err := r.attachRepositorySecretBindings(ctx, repository); err != nil {
			return nil, err
		}
	}
	return repository, err
}

// GetRepositoryByProviderInfo keeps the concrete SQLite helper available for
// legacy callers and tests. Service code uses GetRepositoryByProviderIdentity.
func (r *Repository) GetRepositoryByProviderInfo(
	ctx context.Context, workspaceID, provider, host, owner, name string,
) (*models.Repository, error) {
	return r.GetRepositoryByProviderIdentity(ctx, models.ProviderRepositoryIdentity{
		WorkspaceID: workspaceID, Provider: provider, Host: host, Owner: owner, Name: name,
	})
}

// GetRepositoryByLocalPath finds a live repository by workspace and canonical
// local_path. Returns nil, nil if not found. Mirrors GetRepositoryByProviderInfo,
// including the created_at/id tiebreak, so local-path resolution can do the
// same lookup-then-create as provider-URL resolution instead of trusting a
// possibly-stale in-memory snapshot.
func (r *Repository) GetRepositoryByLocalPath(ctx context.Context, workspaceID, localPath string) (*models.Repository, error) {
	repository := &models.Repository{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_host, provider_scope, provider_owner,
		       provider_name, remote_url, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files, created_at, updated_at, deleted_at
		FROM repositories
		WHERE workspace_id = ? AND local_path = ? AND local_path != '' AND deleted_at IS NULL
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`), workspaceID, localPath).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderHost, &repository.ProviderScope, &repository.ProviderOwner, &repository.ProviderName, &repository.RemoteURL,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript, &repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil {
		if err := r.attachRepositorySecretBindings(ctx, repository); err != nil {
			return nil, err
		}
	}
	return repository, err
}

func (r *Repository) ListRepositorySecretBindings(ctx context.Context, repositoryID string) ([]*models.RepositorySecretBinding, error) {
	var rows []repositorySecretBindingRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT repository_id, key, secret_id, created_at, updated_at
		FROM repository_secret_bindings WHERE repository_id = ? ORDER BY key`), repositoryID); err != nil {
		return nil, err
	}
	return repositorySecretBindingsFromRows(rows), nil
}

func (r *Repository) ListRepositorySecretBindingsByRepositoryIDs(
	ctx context.Context, repositoryIDs []string,
) (map[string][]*models.RepositorySecretBinding, error) {
	result := make(map[string][]*models.RepositorySecretBinding, len(repositoryIDs))
	if len(repositoryIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT repository_id, key, secret_id, created_at, updated_at
		FROM repository_secret_bindings WHERE repository_id IN (?) ORDER BY repository_id, key`, repositoryIDs)
	if err != nil {
		return nil, err
	}
	var rows []repositorySecretBindingRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, binding := range repositorySecretBindingsFromRows(rows) {
		result[binding.RepositoryID] = append(result[binding.RepositoryID], binding)
	}
	return result, nil
}

func (r *Repository) ReplaceRepositorySecretBindings(
	ctx context.Context, repositoryID string, bindings []models.RepositorySecretBinding,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertRepositorySecretBindings(ctx, r.db, tx, repositoryID, bindings); err != nil {
		return err
	}
	return tx.Commit()
}

type repositorySecretBindingRow struct {
	RepositoryID string    `db:"repository_id"`
	Key          string    `db:"key"`
	SecretID     string    `db:"secret_id"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func insertRepositorySecretBindings(
	ctx context.Context, db *sqlx.DB, exec *sqlx.Tx, repositoryID string, bindings []models.RepositorySecretBinding,
) error {
	if _, err := exec.ExecContext(ctx, db.Rebind(`DELETE FROM repository_secret_bindings WHERE repository_id = ?`), repositoryID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, binding := range bindings {
		if _, err := exec.ExecContext(ctx, db.Rebind(`
			INSERT INTO repository_secret_bindings (repository_id, key, secret_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`), repositoryID, binding.Key, binding.SecretID, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) attachRepositorySecretBindings(ctx context.Context, repository *models.Repository) error {
	bindings, err := r.ListRepositorySecretBindings(ctx, repository.ID)
	if err != nil {
		return err
	}
	repository.SecretBindings = bindingsToValues(bindings)
	return nil
}

func (r *Repository) attachRepositorySecretBindingsBatch(ctx context.Context, repositories []*models.Repository) error {
	ids := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		ids = append(ids, repository.ID)
	}
	bindings, err := r.ListRepositorySecretBindingsByRepositoryIDs(ctx, ids)
	if err != nil {
		return err
	}
	for _, repository := range repositories {
		repository.SecretBindings = bindingsToValues(bindings[repository.ID])
	}
	return nil
}

func repositorySecretBindingsFromRows(rows []repositorySecretBindingRow) []*models.RepositorySecretBinding {
	result := make([]*models.RepositorySecretBinding, 0, len(rows))
	for _, row := range rows {
		result = append(result, &models.RepositorySecretBinding{
			RepositoryID: row.RepositoryID,
			Key:          row.Key,
			SecretID:     row.SecretID,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}
	return result
}

func bindingsToValues(bindings []*models.RepositorySecretBinding) []models.RepositorySecretBinding {
	result := make([]models.RepositorySecretBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding != nil {
			result = append(result, *binding)
		}
	}
	return result
}

// CreateRepositoryScript creates a new repository script
func (r *Repository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	if script.ID == "" {
		script.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	script.CreatedAt = now
	script.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO repository_scripts (id, repository_id, name, command, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), script.ID, script.RepositoryID, script.Name, script.Command, script.Position, script.CreatedAt, script.UpdatedAt)

	return err
}

// GetRepositoryScript retrieves a repository script by ID
func (r *Repository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	script := &models.RepositoryScript{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE id = ?
	`), id).Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository script not found: %s", id)
	}
	return script, err
}

// UpdateRepositoryScript updates an existing repository script
func (r *Repository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	script.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE repository_scripts SET name = ?, command = ?, position = ?, updated_at = ? WHERE id = ?
	`), script.Name, script.Command, script.Position, script.UpdatedAt, script.ID)
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
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM repository_scripts WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository script not found: %s", id)
	}
	return nil
}

// ListScriptsByRepositoryIDs returns all scripts for the given repository IDs,
// grouped by repository ID. This eliminates N+1 queries when loading scripts for multiple repos.
func (r *Repository) ListScriptsByRepositoryIDs(ctx context.Context, repoIDs []string) (map[string][]*models.RepositoryScript, error) {
	result := make(map[string][]*models.RepositoryScript, len(repoIDs))
	if len(repoIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(repoIDs))
	args := make([]interface{}, len(repoIDs))
	for i, id := range repoIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts
		WHERE repository_id IN (%s)
		ORDER BY position
	`, strings.Join(placeholders, ","))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		script := &models.RepositoryScript{}
		err := rows.Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result[script.RepositoryID] = append(result[script.RepositoryID], script)
	}
	return result, rows.Err()
}

// ListRepositoryScripts returns all scripts for a repository
func (r *Repository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE repository_id = ? ORDER BY position
	`), repositoryID)
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
