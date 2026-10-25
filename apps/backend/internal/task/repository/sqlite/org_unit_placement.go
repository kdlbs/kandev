package sqlite

import "context"

// CountWorkspacesInUnit reports how many workspaces a unit holds, which is what
// stops internal/orgunit from deleting a unit out from under them.
func (r *Repository) CountWorkspacesInUnit(ctx context.Context, unitID string) (int, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM workspaces WHERE unit_id = ?`), unitID).Scan(&count)
	return count, err
}

// UnplacedWorkspace is a workspace that predates the unit tree.
type UnplacedWorkspace struct {
	ID      string
	OrgID   string
	OwnerID string
}

// UnplacedWorkspaces returns every workspace with no unit, which is the input
// to the one-shot placement backfill.
func (r *Repository) UnplacedWorkspaces(ctx context.Context) ([]UnplacedWorkspace, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id, COALESCE(org_id, ''), COALESCE(owner_id, '')
		  FROM workspaces
		 WHERE COALESCE(unit_id, '') = ''`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []UnplacedWorkspace
	for rows.Next() {
		var w UnplacedWorkspace
		if err := rows.Scan(&w.ID, &w.OrgID, &w.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// PlaceWorkspace records a workspace's unit.
func (r *Repository) PlaceWorkspace(ctx context.Context, workspaceID, unitID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE workspaces SET unit_id = ? WHERE id = ?`), unitID, workspaceID)
	return err
}
