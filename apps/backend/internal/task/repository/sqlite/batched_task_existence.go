package sqlite

import (
	"context"
	"fmt"
)

// batchedTaskIDExistence reports, for each of taskIDs, whether at least one
// row in the named table carries that task_id — the shared shape behind
// every "does this task have an X" batched projection read (task
// environments, running executors), collapsed into one place because the
// two call sites were otherwise byte-for-byte identical. table is always a
// caller-supplied constant, never external input. Chunked via chunkIDs so a
// board or boot load with many tasks stays under SQLite's/PostgreSQL's
// per-statement bind-parameter limit instead of erroring the whole read.
func (r *Repository) batchedTaskIDExistence(ctx context.Context, table string, taskIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	for _, chunk := range chunkIDs(taskIDs, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(fmt.Sprintf(
			`SELECT DISTINCT task_id FROM %s WHERE task_id IN (%s)`, table, placeholders,
		)), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var taskID string
			if err := rows.Scan(&taskID); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result[taskID] = true
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
