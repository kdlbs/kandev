package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// ListSessionsWithBranches returns sessions that have worktree branches
// on non-archived tasks. Used by the PR watch reconciler.
func (r *Repository) ListSessionsWithBranches(ctx context.Context) ([]models.SessionBranchInfo, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT ts.id, ts.task_id, tsw.worktree_branch
		FROM task_sessions ts
		INNER JOIN tasks t ON t.id = ts.task_id
		INNER JOIN task_session_worktrees tsw ON tsw.session_id = ts.id
		WHERE t.archived_at IS NULL
		  AND tsw.worktree_branch != ''
		  AND tsw.deleted_at IS NULL
		  AND tsw.status = 'active'
		GROUP BY ts.id
		ORDER BY ts.started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []models.SessionBranchInfo
	for rows.Next() {
		var info models.SessionBranchInfo
		if err := rows.Scan(&info.SessionID, &info.TaskID, &info.Branch); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}
