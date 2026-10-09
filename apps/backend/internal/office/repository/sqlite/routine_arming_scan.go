package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// ListWorkspaceIDsOrdered returns every workspace ID ordered by created_at
// ascending, ties broken by id ascending, for the Office routine arming
// startup scan (AC-OFFICE-ROUTINE-ARMING-003.2). The workspaces table is
// owned by the task package's schema; this package already reads it
// directly elsewhere (e.g. HasOfficeAdoption).
func (r *Repository) ListWorkspaceIDsOrdered(ctx context.Context) ([]string, error) {
	var ids []string
	if err := r.ro.SelectContext(ctx, &ids,
		`SELECT id FROM workspaces ORDER BY created_at, id`); err != nil {
		return nil, err
	}
	return ids, nil
}

// ListRoutinesOrdered returns a workspace's routines ordered by created_at
// ascending, ties broken by id ascending, for the Office routine arming
// startup scan (AC-OFFICE-ROUTINE-ARMING-003.2). ListRoutines orders by name
// instead, for the UI, so this is a separate method rather than a shared one.
func (r *Repository) ListRoutinesOrdered(ctx context.Context, workspaceID string) ([]*models.Routine, error) {
	var routines []*models.Routine
	if err := r.ro.SelectContext(ctx, &routines, r.ro.Rebind(
		`SELECT * FROM office_routines WHERE workspace_id = ? ORDER BY created_at, id`), workspaceID); err != nil {
		return nil, err
	}
	if routines == nil {
		routines = []*models.Routine{}
	}
	return routines, nil
}
