package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
)

// CreateBudgetPolicy creates a new budget policy.
func (r *Repository) CreateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_budget_policies (
			id, workspace_id, scope_type, scope_id, limit_subcents, period,
			alert_threshold_pct, action_on_exceed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), policy.ID, policy.WorkspaceID, policy.ScopeType, policy.ScopeID,
		policy.LimitSubcents, policy.Period, policy.AlertThresholdPct,
		policy.ActionOnExceed, policy.CreatedAt, policy.UpdatedAt)
	return err
}

// GetBudgetPolicy returns a budget policy by ID.
func (r *Repository) GetBudgetPolicy(ctx context.Context, id string) (*models.BudgetPolicy, error) {
	var policy models.BudgetPolicy
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_budget_policies WHERE id = ?`), id).StructScan(&policy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("budget policy not found: %s", id)
	}
	return &policy, err
}

// ListBudgetPolicies returns all budget policies for a workspace, ordered by
// created_at with id as a tiebreak so evaluation order is total and
// reproducible.
func (r *Repository) ListBudgetPolicies(ctx context.Context, workspaceID string) ([]*models.BudgetPolicy, error) {
	var policies []*models.BudgetPolicy
	err := r.ro.SelectContext(ctx, &policies, r.ro.Rebind(
		`SELECT * FROM office_budget_policies WHERE workspace_id = ? ORDER BY created_at ASC, id ASC`), workspaceID)
	if err != nil {
		return nil, err
	}
	if policies == nil {
		policies = []*models.BudgetPolicy{}
	}
	return policies, nil
}

// UpdateBudgetPolicy updates an existing budget policy and discards that
// policy's claims in the same transaction, so either both apply or neither
// does.
func (r *Repository) UpdateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	policy.UpdatedAt = time.Now().UTC()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.updateBudgetPolicyTx(ctx, tx, policy); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w; rollback: %v", err, rollbackErr)
		}
		return err
	}
	return tx.Commit()
}

func (r *Repository) updateBudgetPolicyTx(ctx context.Context, tx *sqlx.Tx, policy *models.BudgetPolicy) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM office_budget_claims WHERE policy_id = ?`), policy.ID); err != nil {
		return fmt.Errorf("discard claims: %w", err)
	}
	if r.failBudgetPolicyUpdateErr != nil {
		return r.failBudgetPolicyUpdateErr
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE office_budget_policies SET
			scope_type = ?, scope_id = ?, limit_subcents = ?, period = ?,
			alert_threshold_pct = ?, action_on_exceed = ?, updated_at = ?
		WHERE id = ?
	`), policy.ScopeType, policy.ScopeID, policy.LimitSubcents, policy.Period,
		policy.AlertThresholdPct, policy.ActionOnExceed, policy.UpdatedAt, policy.ID)
	if err != nil {
		return fmt.Errorf("update policy: %w", err)
	}
	return nil
}

// DeleteBudgetPolicy deletes a budget policy by ID. Its claims are removed by
// the office_budget_claims foreign key's ON DELETE CASCADE, not here.
func (r *Repository) DeleteBudgetPolicy(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_budget_policies WHERE id = ?`), id)
	return err
}

// Claim atomically records that (policyID, periodKey, level) may emit its
// budget notification. claimed=true means this call won and the caller
// should emit; claimed=false with a nil error means an earlier evaluation
// already holds the claim, or the referenced policy no longer exists — a
// foreign-key violation is deliberately reported as an ordinary miss, not a
// store failure.
func (r *Repository) Claim(ctx context.Context, policyID, periodKey, level string) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_budget_claims (policy_id, period_key, level, claimed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(policy_id, period_key, level) DO NOTHING
	`), policyID, periodKey, level, time.Now().UTC())
	if err != nil {
		if db.IsForeignKeyViolation(err) {
			return false, nil
		}
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}
