package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// ListTaskIDsWithLaunchQueueSummary includes stale positive observations after
// their deferred launch has cleared. Successfully cleared projections and
// deleted tasks leave the set, so recovery needs no separate retry ownership.
func (r *Repository) ListTaskIDsWithLaunchQueueSummary(ctx context.Context) ([]string, error) {
	predicate := "json_type(s.summary, '$.launch_queue') = 'object'"
	if dialect.IsPostgres(r.ro.DriverName()) {
		predicate = "jsonb_typeof(s.summary::jsonb -> 'launch_queue') = 'object'"
	}
	query := `SELECT s.task_id FROM task_status_summaries s
		JOIN tasks t ON t.id = s.task_id
		WHERE ` + predicate + ` ORDER BY s.task_id`
	var ids []string
	if err := r.ro.SelectContext(ctx, &ids, query); err != nil {
		return nil, fmt.Errorf("list launch queue summary recovery candidates: %w", err)
	}
	return ids, nil
}
