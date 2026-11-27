package sqlite

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const branchPolicyPatchLockNamespace = "repository-branch-policy:"
const branchPolicyNameIndex = "uniq_repository_branch_policies_repository_lower_name"

// PatchRepositoryBranchPolicy validates and writes the current locked policy.
// normalize is a pure, policy-specific function owned by the calling service.
func (r *Repository) PatchRepositoryBranchPolicy(
	ctx context.Context, id, repositoryID string, patch *models.RepositoryBranchPolicyPatch,
	normalize func(*models.RepositoryBranchPolicy) (*models.RepositoryBranchPolicy, error),
) (*models.RepositoryBranchPolicy, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	policy, err := r.lockBranchPolicyPatch(ctx, tx, id, repositoryID)
	if err != nil {
		return nil, err
	}
	applyBranchPolicyPatch(policy, patch)
	policy, err = normalize(policy)
	if err != nil {
		return nil, err
	}
	var exists bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (
 SELECT 1 FROM repository_branch_policies WHERE repository_id = ? AND LOWER(name) = ? AND id <> ?
 )`), repositoryID, strings.ToLower(policy.Name), id).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, repoerrors.ErrRepositoryBranchPolicyNameConflict
	}
	policy.UpdatedAt = time.Now().UTC()
	if err := writeBranchPolicyPatch(ctx, tx, policy); err != nil {
		return nil, err
	}
	policy, err = r.scanRepositoryBranchPolicy(tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT `+repositoryBranchPolicyColumns+` FROM repository_branch_policies WHERE id = ? AND repository_id = ?`), id, repositoryID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return policy, nil
}

func (r *Repository) lockBranchPolicyPatch(ctx context.Context, tx *sqlx.Tx, id, repositoryID string) (*models.RepositoryBranchPolicy, error) {
	query := `SELECT ` + repositoryBranchPolicyColumns + ` FROM repository_branch_policies WHERE id = ? AND repository_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, branchPolicyPatchLockNamespace+id); err != nil {
			return nil, err
		}
		query += ` FOR UPDATE`
	} else if err := lockSQLiteBranchPolicyPatch(ctx, tx, id, repositoryID); err != nil {
		return nil, err
	}
	return r.scanRepositoryBranchPolicy(tx.QueryRowContext(ctx, tx.Rebind(query), id, repositoryID))
}

func lockSQLiteBranchPolicyPatch(ctx context.Context, tx *sqlx.Tx, id, repositoryID string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE repository_branch_policies SET id = id WHERE id = ? AND repository_id = ?`), id, repositoryID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	return nil
}

func applyBranchPolicyPatch(policy *models.RepositoryBranchPolicy, patch *models.RepositoryBranchPolicyPatch) {
	if patch.Name != nil {
		policy.Name = *patch.Name
	}
	if patch.Description != nil {
		policy.Description = *patch.Description
	}
	if patch.BaseBranch != nil {
		policy.BaseBranch = *patch.BaseBranch
	}
	if patch.BranchTemplate != nil {
		policy.BranchTemplate = *patch.BranchTemplate
	}
	if patch.PullRequestTarget != nil {
		policy.PullRequestTarget = *patch.PullRequestTarget
	}
}

func writeBranchPolicyPatch(ctx context.Context, tx *sqlx.Tx, policy *models.RepositoryBranchPolicy) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE repository_branch_policies
 SET name = ?, description = ?, base_branch = ?, branch_template = ?, pull_request_target = ?, updated_at = ?
 WHERE id = ? AND repository_id = ?`), policy.Name, policy.Description, policy.BaseBranch, policy.BranchTemplate,
		policy.PullRequestTarget, policy.UpdatedAt, policy.ID, policy.RepositoryID)
	if err != nil {
		if isBranchPolicyNameConflict(err) {
			return repoerrors.ErrRepositoryBranchPolicyNameConflict
		}
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	return nil
}

func isBranchPolicyNameConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == branchPolicyNameIndex
	}
	return strings.Contains(err.Error(), branchPolicyNameIndex)
}
