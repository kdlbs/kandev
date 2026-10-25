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
)

func repositoryUpdateQuery(row *models.Repository, intent models.RepositoryCheckoutIntent) (string, []any) {
	assignments := []string{
		"name = ?", "source_type = ?", "local_path = ?", "provider = ?", "provider_repo_id = ?",
		"provider_host = ?", "provider_scope = ?", "provider_owner = ?", "provider_name = ?", "remote_url = ?",
		"worktree_branch_prefix = ?", "worktree_branch_template = ?", "setup_script = ?", "cleanup_script = ?",
		"dev_script = ?", "copy_files = ?", "updated_at = ?",
	}
	args := []any{row.Name, row.SourceType, row.LocalPath, row.Provider, row.ProviderRepoID,
		row.ProviderHost, row.ProviderScope, row.ProviderOwner, row.ProviderName, row.RemoteURL,
		row.WorktreeBranchPrefix, row.WorktreeBranchTemplate, row.SetupScript, row.CleanupScript,
		row.DevScript, row.CopyFiles, row.UpdatedAt}
	if intent.DefaultBranch != nil {
		assignments = append(assignments, "default_branch = ?")
		args = append(args, *intent.DefaultBranch)
	}
	if intent.PullBeforeWorktree != nil {
		assignments = append(assignments, "pull_before_worktree = ?")
		args = append(args, dialect.BoolToInt(*intent.PullBeforeWorktree))
	}
	args = append(args, row.ID)
	return "UPDATE repositories SET " + strings.Join(assignments, ", ") + " WHERE id = ? AND deleted_at IS NULL", args
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
	proposed := *row
	proposed.UpdatedAt = time.Now().UTC()
	query, args := repositoryUpdateQuery(&proposed, intent)
	var branch string
	var pull bool
	var updated time.Time
	err = tx.QueryRowContext(ctx, r.db.Rebind(query+" RETURNING default_branch, pull_before_worktree, updated_at"), args...).Scan(&branch, &pull, &updated)
	if errors.Is(err, sql.ErrNoRows) {
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
	row.DefaultBranch = branch
	row.PullBeforeWorktree = pull
	row.UpdatedAt = updated
	return nil
}
