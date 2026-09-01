package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// GetWorkspaceGroupOwnerTaskID returns the owner task ID of the active
// (non-released) workspace group taskID belongs to, or "" if taskID is not a
// member of any active group. The owner itself has its own 'owner'-role
// member row (AddWorkspaceGroupMember is called for the owner too), so
// calling this with the owner's own taskID returns that same ID rather than
// "" — callers redirecting a write compare the result against their input
// taskID rather than against "" to detect that no-op case.
// task_workspace_groups/task_workspace_group_members are created by
// internal/office/repository/sqlite (see createWorkspaceGroupTables), not by
// this package's own schema init; both packages operate on the same physical
// SQLite database, and session.go's bindReadySharedGroupEnvironment already
// reads these tables directly for the same reason — avoiding an
// internal/office import into internal/task/repository/sqlite (and, via it,
// into internal/orchestrator).
//
// ORDER BY makes the choice deterministic when a task is (unreleased-)member
// of more than one active group at once — AddWorkspaceGroupMember does not
// constrain a task to a single group, and without an explicit order the
// LIMIT 1 pick depends on SQLite's query plan. Earliest membership wins.
func (r *Repository) GetWorkspaceGroupOwnerTaskID(ctx context.Context, taskID string) (string, error) {
	var ownerTaskID string
	err := r.ro.GetContext(ctx, &ownerTaskID, r.ro.Rebind(`
		SELECT g.owner_task_id
		FROM task_workspace_groups g
		JOIN task_workspace_group_members m ON m.workspace_group_id = g.id
		WHERE m.task_id = ? AND m.released_at IS NULL
		ORDER BY m.created_at, g.id
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

// GetWorkspaceGroupOwnerTaskIDForSession resolves the owner for the workspace
// group that owns the observing session's materialized environment. A task can
// retain active membership in more than one group, so the task-only lookup is
// not safe when a session identifies the exact shared checkout.
func (r *Repository) GetWorkspaceGroupOwnerTaskIDForSession(
	ctx context.Context, taskID, sessionID string,
) (string, error) {
	if taskID == "" || sessionID == "" {
		return "", nil
	}
	var environmentID string
	err := r.ro.GetContext(ctx, &environmentID, r.ro.Rebind(`
		SELECT COALESCE(task_environment_id, '')
		FROM task_sessions
		WHERE id = ? AND task_id = ?
		LIMIT 1
	`), sessionID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// Sessions created before a shared environment is materialized do not yet
	// carry an exact group identity. Preserve the existing task-only behavior
	// until that binding is available; once an environment is present, the
	// exact join below prevents a different active membership from winning.
	if environmentID == "" {
		return r.GetWorkspaceGroupOwnerTaskID(ctx, taskID)
	}
	var ownerTaskID string
	err = r.ro.GetContext(ctx, &ownerTaskID, r.ro.Rebind(`
		SELECT g.owner_task_id
		FROM task_sessions s
		JOIN task_workspace_groups g
		  ON g.materialized_environment_id = s.task_environment_id
		JOIN task_workspace_group_members m
		  ON m.workspace_group_id = g.id
		 AND m.task_id = s.task_id
		 AND m.released_at IS NULL
		WHERE s.id = ?
		  AND s.task_id = ?
		  AND s.task_environment_id <> ''
		ORDER BY g.id
		LIMIT 1
	`), sessionID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ownerTaskID, nil
}
