package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// worktree file materialization modes. Kept as local constants so the
// persistence layer doesn't import the worktree package (which would create an
// import cycle via worktree's test dependencies).
const (
	worktreeFileModeCopy    = "copy"
	worktreeFileModeSymlink = "symlink"
)

// normalizeWorktreeFileMode defaults empty/unknown values to copy.
func normalizeWorktreeFileMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), worktreeFileModeSymlink) {
		return worktreeFileModeSymlink
	}
	return worktreeFileModeCopy
}

// marshalWorktreeFiles serializes a repository's worktree file list to the JSON
// text stored in the repositories.worktree_files column. A nil/empty list is
// persisted as "[]".
func marshalWorktreeFiles(files []models.WorktreeFile) string {
	if len(files) == 0 {
		return "[]"
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// unmarshalWorktreeFiles parses the JSON text from worktree_files back into a
// slice, normalizing each file's mode and dropping blank paths. Blank or
// malformed values (including pre-feature rows and the old string-array format)
// yield an empty slice.
func unmarshalWorktreeFiles(raw string) []models.WorktreeFile {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var files []models.WorktreeFile
	if err := json.Unmarshal([]byte(trimmed), &files); err != nil {
		return nil
	}
	out := make([]models.WorktreeFile, 0, len(files))
	for _, f := range files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		out = append(out, models.WorktreeFile{
			Path: f.Path,
			Mode: normalizeWorktreeFileMode(f.Mode),
		})
	}
	return out
}

// CreateRepository creates a new repository
func (r *Repository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	if repository.ID == "" {
		repository.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repository.CreatedAt = now
	repository.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO repositories (
			id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
			provider_name, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script,
			copy_files, worktree_files, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), repository.ID, repository.WorkspaceID, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider,
		repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch, repository.WorktreeBranchPrefix,
		repository.WorktreeBranchTemplate, dialect.BoolToInt(repository.PullBeforeWorktree), repository.SetupScript, repository.CleanupScript, repository.DevScript,
		repository.CopyFiles, marshalWorktreeFiles(repository.WorktreeFiles),
		repository.CreatedAt, repository.UpdatedAt, repository.DeletedAt)

	return err
}

// repositoryColumns is the shared SELECT column list for repository rows, in the
// order expected by scanRepository.
const repositoryColumns = `id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
	provider_name, default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script,
	copy_files, worktree_files, created_at, updated_at, deleted_at`

// rowScanner abstracts *sql.Row and *sql.Rows so a single scan routine serves
// both single-row and iterating queries.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanRepository decodes one repository row, converting the stored JSON file
// list and normalizing the materialization mode.
func scanRepository(s rowScanner) (*models.Repository, error) {
	repository := &models.Repository{}
	var worktreeFiles string
	err := s.Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
		&repository.DefaultBranch, &repository.WorktreeBranchPrefix, &repository.WorktreeBranchTemplate, &repository.PullBeforeWorktree, &repository.SetupScript,
		&repository.CleanupScript, &repository.DevScript, &repository.CopyFiles, &worktreeFiles,
		&repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	repository.WorktreeFiles = unmarshalWorktreeFiles(worktreeFiles)
	return repository, nil
}

// GetRepository retrieves a repository by ID
func (r *Repository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryColumns+` FROM repositories WHERE id = ? AND deleted_at IS NULL`), id)
	repository, err := scanRepository(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repository, err
}

// UpdateRepository updates an existing repository
func (r *Repository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	repository.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories SET
			name = ?, source_type = ?, local_path = ?, provider = ?, provider_repo_id = ?, provider_owner = ?,
			provider_name = ?, default_branch = ?, worktree_branch_prefix = ?, worktree_branch_template = ?, pull_before_worktree = ?, setup_script = ?, cleanup_script = ?, dev_script = ?,
			copy_files = ?, worktree_files = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`), repository.Name, repository.SourceType, repository.LocalPath, repository.Provider, repository.ProviderRepoID,
		repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch, repository.WorktreeBranchPrefix, repository.WorktreeBranchTemplate, dialect.BoolToInt(repository.PullBeforeWorktree),
		repository.SetupScript, repository.CleanupScript, repository.DevScript,
		repository.CopyFiles, marshalWorktreeFiles(repository.WorktreeFiles),
		repository.UpdatedAt, repository.ID)
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
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE repositories SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`), now, now, id)
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
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryColumns+` FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at DESC`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Repository
	for rows.Next() {
		repository, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, rows.Err()
}

// GetRepositoryByProviderInfo finds a repository by workspace, provider, owner, and name.
// Returns nil, nil if not found.
func (r *Repository) GetRepositoryByProviderInfo(ctx context.Context, workspaceID, provider, owner, name string) (*models.Repository, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryColumns+` FROM repositories
		WHERE workspace_id = ? AND provider = ? AND provider_owner = ? AND provider_name = ? AND deleted_at IS NULL`),
		workspaceID, provider, owner, name)
	repository, err := scanRepository(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return repository, err
}

// GetRepositoryByLocalPath finds a non-deleted repository by its local_path.
// Returns nil, nil if not found. Used to resolve repository config during
// worktree creation, where only the repository path (not its ID) is available.
func (r *Repository) GetRepositoryByLocalPath(ctx context.Context, localPath string) (*models.Repository, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryColumns+` FROM repositories WHERE local_path = ? AND deleted_at IS NULL`),
		localPath)
	repository, err := scanRepository(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return repository, err
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
