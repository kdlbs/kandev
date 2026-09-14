package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// GetWorkspaceBudgetDefault returns the effective built-in default ceiling
// for workspaceID, in subcents. It never returns an error for "no row
// written yet" -- the caller substitutes the shipped constant in that case
// (AC-OFFICE-BUDGET-003.9), so an absent row is reported as (0, false, nil),
// not an error.
func (r *Repository) GetWorkspaceBudgetDefault(ctx context.Context, workspaceID string) (int64, bool, error) {
	var limitSubcents int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT limit_subcents FROM office_budget_default_settings WHERE workspace_id = ?`,
	), workspaceID).Scan(&limitSubcents)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return limitSubcents, true, nil
}

// SetWorkspaceBudgetDefault writes workspaceID's built-in default ceiling.
// The single-statement upsert is what makes AC-OFFICE-BUDGET-003.10's
// last-write-wins hold without a read-modify-write race: two concurrent
// writers each execute one INSERT ... ON CONFLICT statement, and SQLite/
// Postgres both serialize those against the same primary key.
func (r *Repository) SetWorkspaceBudgetDefault(ctx context.Context, workspaceID string, limitSubcents int64) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_budget_default_settings (workspace_id, limit_subcents, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			limit_subcents = excluded.limit_subcents,
			updated_at = excluded.updated_at
	`), workspaceID, limitSubcents, time.Now().UTC())
	return err
}
