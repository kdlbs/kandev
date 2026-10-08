package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) CreateManagedAgentToolGrant(ctx context.Context, grant *models.ManagedAgentToolGrant) error {
	if grant == nil || grant.ID == "" || grant.BindingID == "" || grant.OperationID == "" ||
		grant.TokenHash == "" || grant.Scope == "" || grant.Generation < 1 || grant.ExpiresAt.IsZero() {
		return fmt.Errorf("managed agent tool grant is incomplete")
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO managed_agent_tool_grants (
			id, binding_id, operation_id, token_hash, scope_snapshot, generation, expires_at, revoked_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), grant.ID, grant.BindingID, grant.OperationID, grant.TokenHash, grant.Scope,
		grant.Generation, grant.ExpiresAt, grant.RevokedAt, grant.CreatedAt)
	if err != nil {
		return fmt.Errorf("create managed agent tool grant: %w", err)
	}
	return nil
}

func (r *Repository) GetManagedAgentToolGrantByHash(ctx context.Context, tokenHash string) (*models.ManagedAgentToolGrant, error) {
	grant := &models.ManagedAgentToolGrant{}
	var revokedAt sql.NullTime
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, binding_id, operation_id, token_hash, scope_snapshot, generation, expires_at, revoked_at, created_at
		FROM managed_agent_tool_grants WHERE token_hash = ?
	`), tokenHash).Scan(
		&grant.ID, &grant.BindingID, &grant.OperationID, &grant.TokenHash, &grant.Scope,
		&grant.Generation, &grant.ExpiresAt, &revokedAt, &grant.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentToolGrantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent tool grant: %w", err)
	}
	grant.RevokedAt = managedAgentNullableTime(revokedAt)
	return grant, nil
}

func (r *Repository) RevokeManagedAgentToolGrants(ctx context.Context, bindingID, operationID string, revokedAt time.Time) (int64, error) {
	if bindingID == "" {
		return 0, fmt.Errorf("managed agent binding ID is required to revoke tool grants")
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	query := `UPDATE managed_agent_tool_grants SET revoked_at = ? WHERE binding_id = ? AND revoked_at IS NULL`
	args := []any{revokedAt, bindingID}
	if operationID != "" {
		query += ` AND operation_id = ?`
		args = append(args, operationID)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("revoke managed agent tool grants: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count revoked managed agent tool grants: %w", err)
	}
	return count, nil
}
