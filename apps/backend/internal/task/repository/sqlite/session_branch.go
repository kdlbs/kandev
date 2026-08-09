package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// ListSessionsWithBranches returns sessions that have worktree branches
// on non-archived tasks. Used by the PR watch reconciler. Branches live on
// task_environment_repos; sessions reach them through
// task_sessions.task_environment_id.
func (r *Repository) ListSessionsWithBranches(ctx context.Context) ([]models.SessionBranchInfo, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT ts.id, ts.task_id, t.workspace_id, ter.worktree_branch
		FROM task_sessions ts
		INNER JOIN tasks t ON t.id = ts.task_id
		INNER JOIN task_environment_repos ter ON ter.task_environment_id = ts.task_environment_id
		WHERE t.archived_at IS NULL
		  AND ter.worktree_branch != ''
		  AND ter.deleted_at IS NULL
		  AND ter.status = 'active'
		GROUP BY ts.id, ts.task_id, t.workspace_id, ter.worktree_branch
		ORDER BY ts.started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []models.SessionBranchInfo
	for rows.Next() {
		var info models.SessionBranchInfo
		if err := rows.Scan(&info.SessionID, &info.TaskID, &info.WorkspaceID, &info.Branch); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}
