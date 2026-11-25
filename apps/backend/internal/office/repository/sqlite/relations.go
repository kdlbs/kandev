package sqlite

import (
	"context"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
)

func relationPair(taskA, taskB string) (string, string) {
	if taskA < taskB {
		return taskA, taskB
	}
	return taskB, taskA
}

// AddRelation inserts one symmetric task relationship. Duplicate pairs are no-ops.
func (r *Repository) AddRelation(ctx context.Context, taskA, taskB string) error {
	low, high := relationPair(taskA, taskB)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_relations (task_id_low, task_id_high, created_at)
		VALUES (?, ?, ?) ON CONFLICT(task_id_low, task_id_high) DO NOTHING
	`), low, high, time.Now().UTC())
	return err
}

// ListRelationsForTask returns the task IDs related to taskID in stable order.
func (r *Repository) ListRelationsForTask(ctx context.Context, taskID string) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`
		SELECT CASE WHEN task_id_low = ? THEN task_id_high ELSE task_id_low END
		FROM task_relations WHERE task_id_low = ? OR task_id_high = ?
		ORDER BY created_at, task_id_low, task_id_high
	`), taskID, taskID, taskID)
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

// ListRelationsForTasks returns symmetric adjacency for a batch of task IDs.
func (r *Repository) ListRelationsForTasks(ctx context.Context, taskIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	type row struct {
		Low  string `db:"task_id_low"`
		High string `db:"task_id_high"`
	}
	// A relation pair whose two ends land in different 500-id chunks matches
	// both chunks' queries, so dedupe by pair before building adjacency —
	// otherwise that pair is appended to each side's list twice.
	seen := make(map[row]struct{})
	for _, chunk := range chunkTaskIDs(taskIDs) {
		query, args, err := sqlx.In(`SELECT task_id_low, task_id_high FROM task_relations
			WHERE task_id_low IN (?) OR task_id_high IN (?)
			ORDER BY created_at, task_id_low, task_id_high`, chunk, chunk)
		if err != nil {
			return nil, err
		}
		var rows []row
		if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
			return nil, err
		}
		for _, rel := range rows {
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			result[rel.Low] = append(result[rel.Low], rel.High)
			result[rel.High] = append(result[rel.High], rel.Low)
		}
	}
	for id := range result {
		sort.Strings(result[id])
	}
	return result, nil
}

// RemoveRelationsForTask deletes all relationships touching taskID.
func (r *Repository) RemoveRelationsForTask(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_relations WHERE task_id_low = ? OR task_id_high = ?`), taskID, taskID)
	return err
}
