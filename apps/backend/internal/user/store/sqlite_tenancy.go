package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/user/models"
)

// Tenancy operations: assigning accounts to an organization, the instance
// operator tier, and removing an organization's accounts.
//
// Split from sqlite.go because that file is already at the 800-line limit.

// SetUserOrg assigns a user to an organization. A user belongs to exactly one
// org, and this is the only way a user's org changes: there is no transfer
// flow, because moving a person between tenants would leave their workspaces
// behind in the old one.
func (r *sqliteRepository) SetUserOrg(ctx context.Context, id, orgID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users SET org_id = ?, updated_at = ? WHERE id = ?
	`), orgID, time.Now().UTC(), id)
	return err
}

// AssignUsersWithoutOrg puts every account that has no organization into the
// given one. It is the user half of the tenancy migration and is idempotent.
func (r *sqliteRepository) AssignUsersWithoutOrg(ctx context.Context, orgID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users SET org_id = ? WHERE org_id = '' OR org_id IS NULL
	`), orgID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListUsersByOrg returns the accounts belonging to one organization.
func (r *sqliteRepository) ListUsersByOrg(ctx context.Context, orgID string) ([]*models.User, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+userColumns+` FROM users WHERE org_id = ? ORDER BY created_at ASC, id ASC
	`), orgID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	users := make([]*models.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// SetOperator grants or revokes the instance operator tier.
func (r *sqliteRepository) SetOperator(ctx context.Context, id string, operator bool) error {
	flag := 0
	if operator {
		flag = 1
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users SET is_operator = ?, updated_at = ? WHERE id = ?
	`), flag, time.Now().UTC(), id)
	return err
}

// CountOperators reports how many accounts hold the operator tier. Used to
// refuse removing the last one.
func (r *sqliteRepository) CountOperators(ctx context.Context) (int, error) {
	var n int
	err := r.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_operator = 1`).Scan(&n)
	return n, err
}

// FirstAdminID returns the oldest active admin account, or "" when there is
// none. Used to grant the operator tier during the tenancy migration.
func (r *sqliteRepository) FirstAdminID(ctx context.Context) (string, error) {
	var id string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id FROM users WHERE role = 'admin' AND status = 'active'
		ORDER BY created_at ASC, id ASC LIMIT 1
	`)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// DeleteUsersByOrg removes every account belonging to an organization. Called
// only from organization deletion, after that org's data is gone.
func (r *sqliteRepository) DeleteUsersByOrg(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM users WHERE org_id = ?`), orgID)
	return err
}
