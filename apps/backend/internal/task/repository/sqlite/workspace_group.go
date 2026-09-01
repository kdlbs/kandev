package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// GetWorkspaceGroupOwnerTaskID returns the owner task ID of the active
// (non-released) workspace group taskID belongs to, or "" if taskID is not a
// member of any active group (including the common case where it is itself
// the owner). task_workspace_groups/task_workspace_group_members are created
// by internal/office/repository/sqlite (see createWorkspaceGroupTables), not
// by this package's own schema init; both packages operate on the same
// physical SQLite database, and session.go's bindReadySharedGroupEnvironment
// already reads these tables directly for the same reason — avoiding an
// internal/office import into internal/task/repository/sqlite (and, via it,
// into internal/orchestrator).
func (r *Repository) GetWorkspaceGroupOwnerTaskID(ctx context.Context, taskID string) (string, error) {
	var ownerTaskID string
	err := r.ro.GetContext(ctx, &ownerTaskID, r.ro.Rebind(`
		SELECT g.owner_task_id
		FROM task_workspace_groups g
		JOIN task_workspace_group_members m ON m.workspace_group_id = g.id
		WHERE m.task_id = ? AND m.released_at IS NULL
		LIMIT 1
	`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ownerTaskID, nil
}
