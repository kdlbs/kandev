package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// LoadPRWatchTaskActivity returns the bounded, task-local activity projection
// used by GitHub searching-watch admission. Provider and watch bookkeeping is
// deliberately absent from this query: those writers can update generic
// UpdatedAt columns without representing work on the branch.
func (r *Repository) LoadPRWatchTaskActivity(
	ctx context.Context, taskIDs []string,
) (map[string]models.PRWatchTaskActivity, error) {
	result := make(map[string]models.PRWatchTaskActivity, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	for _, chunk := range chunkIDs(taskIDs, sqliteMaxHostParams/7) {
		placeholders, ids := buildInPlaceholders(chunk)
		if err := r.loadPRWatchTaskActivityChunk(ctx, placeholders, ids, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) loadPRWatchTaskActivityChunk(
	ctx context.Context,
	placeholders string,
	ids []interface{},
	result map[string]models.PRWatchTaskActivity,
) error {
	query := r.prWatchActivityQuery(placeholders)
	args := make([]interface{}, 0, len(ids)*7+1)
	for range 7 {
		args = append(args, ids...)
	}
	args = append(args, TriggeredByLiveMonitor)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("load PR watch task activity: %w", err)
	}
	if err := scanPRWatchActivityRows(rows, result); err != nil {
		return err
	}

	runningRows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT task_id
		FROM task_sessions
		WHERE task_id IN (`+placeholders+`)
		  AND state IN ('STARTING', 'RUNNING')
		GROUP BY task_id`), ids...)
	if err != nil {
		return fmt.Errorf("load running PR watch tasks: %w", err)
	}
	defer func() { _ = runningRows.Close() }()
	for runningRows.Next() {
		var taskID string
		if err := runningRows.Scan(&taskID); err != nil {
			return fmt.Errorf("scan running PR watch task: %w", err)
		}
		activity := result[taskID]
		activity.Running = true
		result[taskID] = activity
	}
	if err := runningRows.Err(); err != nil {
		return fmt.Errorf("read running PR watch tasks: %w", err)
	}
	return nil
}

func (r *Repository) prWatchActivityQuery(placeholders string) string {
	lifecycleOnlyPredicate := turnLifecycleOnlyPredicate(r.ro.DriverName(), "turn")
	return `
			WITH activity AS (
				SELECT id AS task_id, created_at AS activity_at
				FROM tasks
				WHERE id IN (` + placeholders + `)
				UNION ALL
				SELECT task_id, created_at AS activity_at
				FROM task_session_messages
				WHERE task_id IN (` + placeholders + `)
				  AND author_type = 'user'
				UNION ALL
				SELECT task_id, queued_at AS activity_at
				FROM queued_messages
				WHERE task_id IN (` + placeholders + `)
				  AND queued_by NOT IN ('agent', 'workflow', 'server')
				UNION ALL
				SELECT task_id, started_at AS activity_at
				FROM task_session_turns turn
				WHERE task_id IN (` + placeholders + `)
				  AND started_at IS NOT NULL
				  AND NOT (` + lifecycleOnlyPredicate + `)
				UNION ALL
				SELECT task_id, completed_at AS activity_at
				FROM task_session_turns turn
				WHERE task_id IN (` + placeholders + `)
				  AND completed_at IS NOT NULL
				  AND NOT (` + lifecycleOnlyPredicate + `)
				UNION ALL
				SELECT sess.task_id, c.committed_at AS activity_at
				FROM task_session_commits c
				INNER JOIN task_sessions sess ON sess.id = c.session_id
				WHERE sess.task_id IN (` + placeholders + `)
				UNION ALL
				SELECT sess.task_id, snap.created_at AS activity_at
				FROM task_session_git_snapshots snap
				INNER JOIN task_sessions sess ON sess.id = snap.session_id
				WHERE sess.task_id IN (` + placeholders + `)
				  AND snap.triggered_by <> ?
				  AND (snap.head_commit <> '' OR snap.remote_branch <> '')
			)
			SELECT task_id, MAX(activity_at) AS activity_at
			FROM activity
			GROUP BY task_id`

}

func scanPRWatchActivityRows(
	rows *sql.Rows,
	result map[string]models.PRWatchTaskActivity,
) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			taskID        string
			rawActivityAt interface{}
		)
		if err := rows.Scan(&taskID, &rawActivityAt); err != nil {
			return fmt.Errorf("scan PR watch task activity: %w", err)
		}
		activityAt, err := parseTaskActivityTime(rawActivityAt)
		if err != nil {
			return fmt.Errorf("parse PR watch task activity %q: %w", taskID, err)
		}
		result[taskID] = models.PRWatchTaskActivity{LastActivityAt: activityAt}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read PR watch task activity: %w", err)
	}
	return nil
}
