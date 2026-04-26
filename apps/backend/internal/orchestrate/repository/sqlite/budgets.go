package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
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
		INSERT INTO orchestrate_budget_policies (
			id, workspace_id, scope_type, scope_id, limit_cents, period,
			alert_threshold_pct, action_on_exceed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), policy.ID, policy.WorkspaceID, policy.ScopeType, policy.ScopeID,
		policy.LimitCents, policy.Period, policy.AlertThresholdPct,
		policy.ActionOnExceed, policy.CreatedAt, policy.UpdatedAt)
	return err
}

// GetBudgetPolicy returns a budget policy by ID.
func (r *Repository) GetBudgetPolicy(ctx context.Context, id string) (*models.BudgetPolicy, error) {
	var policy models.BudgetPolicy
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_budget_policies WHERE id = ?`), id).StructScan(&policy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("budget policy not found: %s", id)
	}
	return &policy, err
}

// ListBudgetPolicies returns all budget policies for a workspace.
func (r *Repository) ListBudgetPolicies(ctx context.Context, workspaceID string) ([]*models.BudgetPolicy, error) {
	var policies []*models.BudgetPolicy
	err := r.ro.SelectContext(ctx, &policies, r.ro.Rebind(
		`SELECT * FROM orchestrate_budget_policies WHERE workspace_id = ? ORDER BY created_at`), workspaceID)
	if err != nil {
		return nil, err
	}
	if policies == nil {
		policies = []*models.BudgetPolicy{}
	}
	return policies, nil
}

// UpdateBudgetPolicy updates an existing budget policy.
func (r *Repository) UpdateBudgetPolicy(ctx context.Context, policy *models.BudgetPolicy) error {
	policy.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_budget_policies SET
			scope_type = ?, scope_id = ?, limit_cents = ?, period = ?,
			alert_threshold_pct = ?, action_on_exceed = ?, updated_at = ?
		WHERE id = ?
	`), policy.ScopeType, policy.ScopeID, policy.LimitCents, policy.Period,
		policy.AlertThresholdPct, policy.ActionOnExceed, policy.UpdatedAt, policy.ID)
	return err
}

// DeleteBudgetPolicy deletes a budget policy by ID.
func (r *Repository) DeleteBudgetPolicy(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_budget_policies WHERE id = ?`), id)
	return err
}
