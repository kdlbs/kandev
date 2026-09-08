package sqlite

import (
	"context"
	"fmt"
	"strings"
)

// batchedTaskIDExistence reports, for each of taskIDs, whether at least one
// row in the named table carries that task_id — the shared shape behind
// every "does this task have an X" batched projection read (task
// environments, running executors), collapsed into one place because the
// two call sites were otherwise byte-for-byte identical.
func (r *Repository) batchedTaskIDExistence(ctx context.Context, table string, taskIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i], args[i] = "?", id
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(fmt.Sprintf(
		`SELECT DISTINCT task_id FROM %s WHERE task_id IN (%s)`, table, strings.Join(placeholders, ","),
	)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		result[taskID] = true
	}
	return result, rows.Err()
}
