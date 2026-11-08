package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func repositoryUpdateQuery(row *models.Repository, intent models.RepositoryCheckoutIntent) (string, []any) {
	assignments := []string{
		"name = ?", "source_type = ?", "local_path = ?", "provider = ?", "provider_repo_id = ?",
		"provider_host = ?", "provider_scope = ?", "provider_owner = ?", "provider_name = ?",
		"worktree_branch_prefix = ?", "worktree_branch_template = ?", "setup_script = ?", "cleanup_script = ?",
		"dev_script = ?", "copy_files = ?", "updated_at = ?",
	}
	args := []any{row.Name, row.SourceType, row.LocalPath, row.Provider, row.ProviderRepoID,
		row.ProviderHost, row.ProviderScope, row.ProviderOwner, row.ProviderName,
		row.WorktreeBranchPrefix, row.WorktreeBranchTemplate, row.SetupScript, row.CleanupScript,
		row.DevScript, row.CopyFiles, row.UpdatedAt}
	if intent.RemoteURL != nil {
		assignments = append(assignments, "remote_url = ?")
		args = append(args, *intent.RemoteURL)
	}
	if intent.DefaultBranch != nil {
		assignments = append(assignments, "default_branch = ?")
		args = append(args, *intent.DefaultBranch)
	}
	if intent.PullBeforeWorktree != nil {
		assignments = append(assignments, "pull_before_worktree = ?")
		args = append(args, dialect.BoolToInt(*intent.PullBeforeWorktree))
	}
	query := "UPDATE repositories SET " + strings.Join(assignments, ", ") + " WHERE id = ? AND deleted_at IS NULL"
	args = append(args, row.ID)
	if fence := intent.ObservedIdentity; fence != nil {
		query += " AND workspace_id = ? AND source_type = ? AND local_path = ? AND provider = ? AND provider_host = ?" +
			" AND provider_scope = ? AND provider_repo_id = ? AND provider_owner = ? AND provider_name = ?"
		args = append(args, fence.WorkspaceID, fence.SourceType, fence.LocalPath, fence.Provider, fence.ProviderHost,
			fence.ProviderScope, fence.ProviderRepoID, fence.ProviderOwner, fence.ProviderName)
	}
	return query, args
}

func nextRepositoryUpdatedAt(previous time.Time) time.Time {
	now := time.Now().UTC()
	minimum := previous.Add(time.Microsecond)
	if now.After(minimum) {
		return now
	}
	return minimum
}

// UpdateRepositoryWithCheckoutIntent preserves omitted checkout columns and
// captures both choices from the successful mutation.
func (r *Repository) UpdateRepositoryWithCheckoutIntent(ctx context.Context, row *models.Repository, intent models.RepositoryCheckoutIntent) error {
	return r.writeRepositoryCheckout(ctx, row, intent, nil)
}

// UpdateRepositoryWithSecretBindingsAndCheckoutIntent commits settings and
// the supplied binding set together.
func (r *Repository) UpdateRepositoryWithSecretBindingsAndCheckoutIntent(ctx context.Context, row *models.Repository, bindings []models.RepositorySecretBinding, intent models.RepositoryCheckoutIntent) error {
	return r.writeRepositoryCheckout(ctx, row, intent, &bindings)
}

func (r *Repository) writeRepositoryCheckout(ctx context.Context, row *models.Repository, intent models.RepositoryCheckoutIntent, bindings *[]models.RepositorySecretBinding) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stored, found, err := r.lockRepositoryUpdatedAt(ctx, tx, row.ID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("repository not found: %s", row.ID)
	}
	proposed := *row
	proposed.UpdatedAt = nextRepositoryUpdatedAt(stored)
	query, args := repositoryUpdateQuery(&proposed, intent)
	var branch, remoteURL string
	var pull bool
	var updated time.Time
	err = tx.QueryRowContext(ctx, r.db.Rebind(query+" RETURNING remote_url, default_branch, pull_before_worktree, updated_at"), args...).Scan(&remoteURL, &branch, &pull, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		if intent.ObservedIdentity != nil {
			return repoerrors.ErrTaskVersionConflict
		}
		return fmt.Errorf("repository not found: %s", row.ID)
	}
	if err != nil {
		return err
	}
	if bindings != nil {
		if err := insertRepositorySecretBindings(ctx, r.db, tx, row.ID, *bindings); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	row.RemoteURL = remoteURL
	row.DefaultBranch = branch
	row.PullBeforeWorktree = pull
	row.UpdatedAt = updated
	return nil
}
