package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const repositoryBranchPolicyColumns = `id, repository_id, name, description, base_branch, branch_template, pull_request_target, created_at, updated_at`

func (r *Repository) CreateRepositoryBranchPolicy(ctx context.Context, policy *models.RepositoryBranchPolicy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	policy.CreatedAt = now
	policy.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), policy.ID, policy.RepositoryID, policy.Name, policy.Description, policy.BaseBranch,
		policy.BranchTemplate, policy.PullRequestTarget, policy.CreatedAt, policy.UpdatedAt)
	return err
}

func (r *Repository) scanRepositoryBranchPolicy(row *sql.Row) (*models.RepositoryBranchPolicy, error) {
	policy := &models.RepositoryBranchPolicy{}
	err := row.Scan(&policy.ID, &policy.RepositoryID, &policy.Name, &policy.Description,
		&policy.BaseBranch, &policy.BranchTemplate, &policy.PullRequestTarget,
		&policy.CreatedAt, &policy.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrRepositoryBranchPolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return policy, nil
}

func (r *Repository) GetRepositoryBranchPolicy(ctx context.Context, id string) (*models.RepositoryBranchPolicy, error) {
	return r.scanRepositoryBranchPolicy(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryBranchPolicyColumns+` FROM repository_branch_policies WHERE id = ?`), id))
}

func (r *Repository) GetRepositoryBranchPolicyByName(ctx context.Context, repositoryID, name string) (*models.RepositoryBranchPolicy, error) {
	policy, err := r.scanRepositoryBranchPolicy(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+repositoryBranchPolicyColumns+` FROM repository_branch_policies WHERE repository_id = ? AND LOWER(name) = ?`),
		repositoryID, strings.ToLower(strings.TrimSpace(name))))
	if errors.Is(err, repoerrors.ErrRepositoryBranchPolicyNotFound) {
		return nil, nil
	}
	return policy, err
}

func (r *Repository) ListRepositoryBranchPolicies(ctx context.Context, repositoryID string) ([]*models.RepositoryBranchPolicy, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+repositoryBranchPolicyColumns+` FROM repository_branch_policies
		WHERE repository_id = ? ORDER BY LOWER(name), id
	`), repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	policies := make([]*models.RepositoryBranchPolicy, 0)
	for rows.Next() {
		policy := &models.RepositoryBranchPolicy{}
		if err := rows.Scan(&policy.ID, &policy.RepositoryID, &policy.Name, &policy.Description,
			&policy.BaseBranch, &policy.BranchTemplate, &policy.PullRequestTarget,
			&policy.CreatedAt, &policy.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (r *Repository) ListRepositoryBranchPoliciesByWorkspace(ctx context.Context, workspaceID string) ([]*models.RepositoryBranchPolicy, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+repositoryBranchPolicyColumnsWithAlias("p")+` FROM repository_branch_policies p
		JOIN repositories r ON r.id = p.repository_id
		WHERE r.workspace_id = ? ORDER BY p.repository_id, LOWER(p.name), p.id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	policies := make([]*models.RepositoryBranchPolicy, 0)
	for rows.Next() {
		policy := &models.RepositoryBranchPolicy{}
		if err := rows.Scan(&policy.ID, &policy.RepositoryID, &policy.Name, &policy.Description,
			&policy.BaseBranch, &policy.BranchTemplate, &policy.PullRequestTarget,
			&policy.CreatedAt, &policy.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func repositoryBranchPolicyColumnsWithAlias(alias string) string {
	return alias + ".id, " + alias + ".repository_id, " + alias + ".name, " + alias + ".description, " +
		alias + ".base_branch, " + alias + ".branch_template, " + alias + ".pull_request_target, " +
		alias + ".created_at, " + alias + ".updated_at"
}

func (r *Repository) UpdateRepositoryBranchPolicy(ctx context.Context, policy *models.RepositoryBranchPolicy) error {
	policy.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE repository_branch_policies
		SET name = ?, description = ?, base_branch = ?, branch_template = ?, pull_request_target = ?, updated_at = ?
		WHERE id = ?
	`), policy.Name, policy.Description, policy.BaseBranch, policy.BranchTemplate,
		policy.PullRequestTarget, policy.UpdatedAt, policy.ID)
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

func (r *Repository) DeleteRepositoryBranchPolicy(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM repository_branch_policies WHERE id = ?`), id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (r *Repository) CreateRepositoryBranchPoliciesIfEmpty(ctx context.Context, repositoryID string, policies []*models.RepositoryBranchPolicy) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM repository_branch_policies WHERE repository_id = ?`), repositoryID); err != nil {
		return err
	}
	if count > 0 {
		return repoerrors.ErrRepositoryBranchPoliciesExist
	}
	for _, policy := range policies {
		if policy.ID == "" {
			policy.ID = uuid.New().String()
		}
		now := time.Now().UTC()
		policy.RepositoryID = repositoryID
		policy.CreatedAt = now
		policy.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO repository_branch_policies (`+repositoryBranchPolicyColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), policy.ID, policy.RepositoryID, policy.Name, policy.Description, policy.BaseBranch,
			policy.BranchTemplate, policy.PullRequestTarget, policy.CreatedAt, policy.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
