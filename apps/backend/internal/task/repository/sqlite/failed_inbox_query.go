package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ListFailedInboxTasks returns the bounded, workspace-scoped page of failed
// tasks for the Inbox Failed tab (docs/specs/ui/system-design/inbox-failed-bucket-01.md,
// "Ordering, paging, and the failure instant" and "Session resolution"). A
// row exists when tasks.state is the terminal failed state and
// tasks.archived_at is null (AC .6, .7); the correlated subquery selects the
// same session AC .30/.30a pick per task, and every ordering key is explicit
// rather than left to either engine's default (AC .10, .10a).
func (r *Repository) ListFailedInboxTasks(ctx context.Context, opts models.ListFailedInboxOptions) (*models.FailedInboxPage, error) {
	if opts.Limit < 1 {
		return nil, fmt.Errorf("ListFailedInboxTasks: limit must be >= 1, got %d", opts.Limit)
	}

	drv := r.ro.DriverName()
	query := failedInboxQuery(drv)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), string(v1.TaskStateFailed), opts.WorkspaceID, opts.Limit+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	taskRows, err := scanFailedInboxRows(rows)
	if err != nil {
		return nil, err
	}

	page := &models.FailedInboxPage{Rows: taskRows}
	if len(taskRows) > opts.Limit {
		page.Rows = taskRows[:opts.Limit]
		page.HasMore = true
	}
	return page, nil
}

// failedInboxSessionOrder is the session-selection order AC .30/.30a specify:
// the primary session first, then most-recently-started, tied broken by the
// lowest session id compared as a byte-ordered string.
func failedInboxSessionOrder(drv string) string {
	return fmt.Sprintf("s.is_primary DESC, s.started_at DESC, %s ASC", dialect.ByteOrderedText(drv, "s.id"))
}

// failedInboxRowOrder is the row order AC .10/.10a specify: unresolvable
// failure instants first, then failure instant descending, then task id
// ascending as a byte-ordered string -- explicit rather than left to either
// engine's default null placement (design#Ordering-paging-and-the-failure-instant).
func failedInboxRowOrder(drv string) string {
	return fmt.Sprintf(
		"(CASE WHEN cs.completed_at IS NULL THEN 0 ELSE 1 END) ASC, cs.completed_at DESC, %s ASC",
		dialect.ByteOrderedText(drv, "t.id"),
	)
}

func failedInboxQuery(drv string) string {
	return fmt.Sprintf(`
		SELECT t.id, t.title, t.workspace_id, t.origin, cs.completed_at, cs.error_message
		FROM tasks t
		LEFT JOIN task_sessions cs ON cs.id = (
			SELECT s.id FROM task_sessions s
			WHERE s.task_id = t.id
			ORDER BY %s
			LIMIT 1
		)
		WHERE t.state = ? AND t.archived_at IS NULL AND t.workspace_id = ?
		ORDER BY %s
		LIMIT ?
	`, failedInboxSessionOrder(drv), failedInboxRowOrder(drv))
}

func scanFailedInboxRows(rows *sql.Rows) ([]models.FailedInboxTaskRow, error) {
	var out []models.FailedInboxTaskRow
	for rows.Next() {
		var row models.FailedInboxTaskRow
		var origin sql.NullString
		var completedAt sql.NullTime
		var errorMessage sql.NullString
		if err := rows.Scan(&row.TaskID, &row.Title, &row.WorkspaceID, &origin, &completedAt, &errorMessage); err != nil {
			return nil, err
		}
		row.Origin = origin.String
		if completedAt.Valid {
			completedAtCopy := completedAt.Time
			row.FailureInstant = &completedAtCopy
		}
		row.Reason = errorMessage.String
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
