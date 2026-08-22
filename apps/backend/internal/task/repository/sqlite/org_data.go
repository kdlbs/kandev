package sqlite

import (
	"context"
	"fmt"
)

// Tenancy data operations: the task-repository half of the organization
// migration and of organization deletion.

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

// DeleteOrgData removes every workspace owned by an organization, cascading
// through the normal workspace-delete path so tasks, sessions, workflows and
// side tables go with them.
//
// It deliberately reuses DeleteWorkspaceCascade rather than issuing bulk
// deletes: the cascade is the one place that knows every dependent table, and
// a second copy would drift the first time a table is added.
func (r *Repository) DeleteOrgData(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT id FROM workspaces WHERE org_id = ?`), orgID)
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	for _, id := range ids {
		if _, _, err := r.DeleteWorkspaceCascade(ctx, id); err != nil {
			return fmt.Errorf("delete workspace %s: %w", id, err)
		}
	}
	return nil
}

// CountWorkspacesByOrg reports how many workspaces an organization owns. Used
// by the delete confirmation so an operator sees what they are removing.
func (r *Repository) CountWorkspacesByOrg(ctx context.Context, orgID string) (int, error) {
	var n int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM workspaces WHERE org_id = ?`), orgID).Scan(&n)
	return n, err
}
