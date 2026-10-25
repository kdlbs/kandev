package sqlite

import (
	"context"
)

// Tenancy data operations: the task-repository half of organization migration
// and lifecycle counts.

// AssignWorkspacesWithoutOrg puts every workspace that has no organization
// into the given one. Idempotent: a second run moves nothing.
func (r *Repository) AssignWorkspacesWithoutOrg(ctx context.Context, orgID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workspaces SET org_id = ? WHERE org_id = '' OR org_id IS NULL
	`), orgID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DropCrossOrgWorkspaceMembers removes membership rows whose user and
// workspace belong to different organizations.
//
// Such a row cannot grant anything (the resolver checks the tenant boundary
// before membership), but leaving it would show a colleague from another
// organization in a member list, which reads as a leak even though it is not
// one. Rows whose user or workspace has no org yet are left alone: they are
// mid-migration, not cross-tenant.
func (r *Repository) DropCrossOrgWorkspaceMembers(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM workspace_members
		WHERE EXISTS (
			SELECT 1 FROM workspaces w, users u
			WHERE w.id = workspace_members.workspace_id
			  AND u.id = workspace_members.user_id
			  AND w.org_id <> ''
			  AND u.org_id <> ''
			  AND w.org_id <> u.org_id
		)
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CountWorkspacesByOrg reports how many workspaces an organization owns. Used
// by the delete confirmation so an operator sees what they are removing.
func (r *Repository) CountWorkspacesByOrg(ctx context.Context, orgID string) (int, error) {
	var n int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM workspaces WHERE org_id = ?`), orgID).Scan(&n)
	return n, err
}
