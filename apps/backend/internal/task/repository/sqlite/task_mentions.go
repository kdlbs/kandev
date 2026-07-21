package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// SearchMentionTasks searches active, durable tasks by title for mention
// autocomplete. Results are lightweight projections, not hydrated tasks.
func (r *Repository) SearchMentionTasks(
	ctx context.Context,
	workspaceID, query, excludeTaskID string,
	limit int,
) ([]*models.Task, error) {
	query = strings.TrimSpace(query)
	if workspaceID == "" || query == "" || limit <= 0 {
		return []*models.Task{}, nil
	}

	escapedQuery := escapeLikePattern(query)
	containsPattern := "%" + escapedQuery + "%"
	prefixPattern := escapedQuery + "%"
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT t.id, t.workspace_id, COALESCE(t.identifier, ''), t.title
		FROM tasks t
		WHERE t.workspace_id = ?
		  AND t.archived_at IS NULL
		  AND t.is_ephemeral = 0
		  AND LOWER(t.title) LIKE LOWER(?) ESCAPE '\'
		  AND (? = '' OR t.id <> ?)
		ORDER BY
		  CASE WHEN LOWER(t.title) LIKE LOWER(?) ESCAPE '\' THEN 0 ELSE 1 END,
		  LOWER(t.title) ASC,
		  t.id ASC
		LIMIT ?
	`), workspaceID, containsPattern, excludeTaskID, excludeTaskID, prefixPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search mention tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]*models.Task, 0)
	for rows.Next() {
		task := &models.Task{}
		if err := rows.Scan(&task.ID, &task.WorkspaceID, &task.Identifier, &task.Title); err != nil {
			return nil, fmt.Errorf("search mention tasks: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search mention tasks: %w", err)
	}
	return tasks, nil
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
