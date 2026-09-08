package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrOwnershipChanged reports that the workspace owner changed between
// authorization and the write, so the transfer was not applied.
var ErrOwnershipChanged = errors.New("workspace ownership changed; retry the transfer")

const workspaceMemberColumns = `workspace_id, user_id, role, added_by, created_at`

// ListWorkspaceMembers returns every membership row for a workspace, owner
// first and then by creation order, so the UI has a stable list.
func (r *Repository) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*models.WorkspaceMember, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+workspaceMemberColumns+`
		FROM workspace_members
		WHERE workspace_id = ?
		ORDER BY CASE role WHEN 'owner' THEN 0 ELSE 1 END, created_at, user_id
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	members := make([]*models.WorkspaceMember, 0)
	for rows.Next() {
		member := &models.WorkspaceMember{}
		if err := rows.Scan(&member.WorkspaceID, &member.UserID, &member.Role, &member.AddedBy, &member.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// GetWorkspaceMember returns one membership row, or nil when the user holds
// none. A missing row is not an error: it is the common case.
func (r *Repository) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*models.WorkspaceMember, error) {
	member := &models.WorkspaceMember{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+workspaceMemberColumns+`
		FROM workspace_members WHERE workspace_id = ? AND user_id = ?
	`), workspaceID, userID).Scan(
		&member.WorkspaceID, &member.UserID, &member.Role, &member.AddedBy, &member.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return member, nil
}

// ListWorkspaceIDsForMember returns the workspaces a user holds a row on. It
// backs the per-request member set: one query per request instead of one per
// workspace on a board render.
func (r *Repository) ListWorkspaceIDsForMember(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, role FROM workspace_members WHERE user_id = ?
	`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	roles := make(map[string]string)
	for rows.Next() {
		var workspaceID, role string
		if err := rows.Scan(&workspaceID, &role); err != nil {
			return nil, err
		}
		roles[workspaceID] = role
	}
	return roles, rows.Err()
}

// UpsertWorkspaceMember adds a member or changes an existing member's role.
func (r *Repository) UpsertWorkspaceMember(ctx context.Context, member *models.WorkspaceMember) error {
	if member.CreatedAt.IsZero() {
		member.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspace_members (`+workspaceMemberColumns+`)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`), member.WorkspaceID, member.UserID, member.Role, member.AddedBy, member.CreatedAt)
	return err
}

// DeleteWorkspaceMember removes one membership row. Removing a row that does
// not exist is not an error.
func (r *Repository) DeleteWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?
	`), workspaceID, userID)
	return err
}

// DeleteWorkspaceMembersByWorkspace clears a workspace's membership. The
// foreign key cascades on workspace delete, but workspace-scoped side tables
// are also cleared explicitly from the deleted handler so the behavior does
// not depend on the dialect honoring the cascade.
func (r *Repository) DeleteWorkspaceMembersByWorkspace(ctx context.Context, workspaceID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workspace_members WHERE workspace_id = ?
	`), workspaceID)
	return err
}

// CountWorkspaceMembers returns membership counts for the given workspaces in
// one query, keyed by workspace ID. Used to populate list DTOs without an
// N+1 on the workspace list.
func (r *Repository) CountWorkspaceMembers(ctx context.Context) (map[string]int, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT workspace_id, COUNT(*) FROM workspace_members GROUP BY workspace_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var workspaceID string
		var count int
		if err := rows.Scan(&workspaceID, &count); err != nil {
			return nil, err
		}
		counts[workspaceID] = count
	}
	return counts, rows.Err()
}

// TransferWorkspaceOwnership moves the accountable owner in one transaction:
// workspaces.owner_id and the mirroring owner row must never disagree.
func (r *Repository) TransferWorkspaceOwnership(ctx context.Context, workspaceID, fromUserID, toUserID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	// Conditional on the owner we were authorized against. Two transfers
	// authorized while A owned the workspace can arrive sequentially; without
	// this the second would move ownership off B while demoting the stale A,
	// leaving B holding an owner row that owner_id no longer names.
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE workspaces SET owner_id = ?, updated_at = ? WHERE id = ? AND owner_id = ?
	`), toUserID, now, workspaceID, fromUserID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOwnershipChanged
	}
	// Both rows are upserted rather than updated. An UPDATE silently affects
	// nothing when the row is absent, which would leave owner_id naming a user
	// with no owner row -- exactly the disagreement this table exists to
	// prevent. Absent rows are reachable through pre-auth data and direct
	// repository writes, so the write has to be total.
	if err := upsertMemberTx(ctx, tx, r.db.Rebind, workspaceID, toUserID, "owner", now); err != nil {
		return err
	}
	if fromUserID != "" && fromUserID != toUserID {
		if err := upsertMemberTx(ctx, tx, r.db.Rebind, workspaceID, fromUserID, "collaborator", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertMemberTx(
	ctx context.Context,
	tx *sqlx.Tx,
	rebind func(string) string,
	workspaceID, userID, role string,
	now time.Time,
) error {
	_, err := tx.ExecContext(ctx, rebind(`
		INSERT INTO workspace_members (workspace_id, user_id, role, added_by, created_at)
		VALUES (?, ?, ?, '', ?)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`), workspaceID, userID, role, now)
	return err
}

// DeleteWorkspaceMembersByUser removes every membership an account holds.
//
// This is the cross-store cleanup that stands in for a foreign key: the users
// table is owned by internal/user/store and initializes independently of this
// repository, so a real FK here would break schema init depending on which
// store runs first.
func (r *Repository) DeleteWorkspaceMembersByUser(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workspace_members WHERE user_id = ?
	`), userID)
	return err
}
