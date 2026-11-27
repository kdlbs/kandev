package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const branchPolicyAdmissionLockNamespace = "repository-branch-policy-admission:"

func (r *Repository) lockRepositoryBranchPolicyAdmission(ctx context.Context, tx *sqlx.Tx, repositoryID string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, branchPolicyAdmissionLockNamespace+repositoryID)
		return err
	}
	// A write obtains SQLite admission even when the policy set is empty.
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE repository_branch_policies SET id = id WHERE repository_id = ?`), repositoryID)
	return err
}

func (r *Repository) insertRepositoryBranchPolicy(ctx context.Context, tx *sqlx.Tx, policy *models.RepositoryBranchPolicy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	policy.CreatedAt = time.Now().UTC()
	policy.UpdatedAt = policy.CreatedAt
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), policy.ID, policy.RepositoryID, policy.Name, policy.Description, policy.BaseBranch,
		policy.BranchTemplate, policy.PullRequestTarget, policy.CreatedAt, policy.UpdatedAt)
	if err != nil {
		if isBranchPolicyInsertNameConflict(err) {
			return repoerrors.ErrRepositoryBranchPolicyNameConflict
		}
		return err
	}
	stored, err := r.scanRepositoryBranchPolicy(tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT `+repositoryBranchPolicyColumns+` FROM repository_branch_policies WHERE id = ? AND repository_id = ?`), policy.ID, policy.RepositoryID))
	if err != nil {
		return err
	}
	*policy = *stored
	return nil
}

func isBranchPolicyInsertNameConflict(err error) bool {
	if !isBranchPolicyNameConflict(err) {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return true
	}
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}
