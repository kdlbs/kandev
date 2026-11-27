package coordinator

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db"
)

// stallMaxAge is the retention window past which a stall record is pruned
// regardless of its task's state (docs/specs/coordinator/system-design/
// needs-you.md#stall-records).
const stallMaxAge = 30 * 24 * time.Hour

// pruneStallsWithTasksSQL is valid on both SQLite and PostgreSQL. It deletes
// a coordinator_stalls row whose task no longer exists or is archived, in
// addition to the age cutoff.
const pruneStallsWithTasksSQL = `
	DELETE FROM coordinator_stalls
	WHERE detected_at < ?
	   OR NOT EXISTS (
			SELECT 1 FROM tasks
			WHERE tasks.id = coordinator_stalls.task_id AND tasks.archived_at IS NULL
		)`

const pruneStallsByAgeOnlySQL = `DELETE FROM coordinator_stalls WHERE detected_at < ?`

// PruneStalls deletes a stall record whose task is missing or archived, or
// whose detected_at is older than 30 days (needs-you.md#stall-records). When
// the tasks table does not exist (a store used in isolation), it prunes only
// by age. Returns the number of rows deleted.
func (s *Store) PruneStalls(ctx context.Context, now time.Time) (int64, error) {
	tasksExist, err := db.TableExistsContext(ctx, s.db, "tasks")
	if err != nil {
		return 0, fmt.Errorf("check tasks table for stall pruning: %w", err)
	}
	query := pruneStallsByAgeOnlySQL
	if tasksExist {
		query = pruneStallsWithTasksSQL
	}
	cutoff := now.Add(-stallMaxAge)
	res, err := s.db.ExecContext(ctx, s.db.Rebind(query), cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune stalls: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned stalls: %w", err)
	}
	return deleted, nil
}
