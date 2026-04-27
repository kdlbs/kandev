package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// GetTaskCounts returns aggregated task counts by status for a project.
func (r *Repository) GetTaskCounts(ctx context.Context, projectID string) (*models.TaskCounts, error) {
	var counts models.TaskCounts
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN state IN ('IN_PROGRESS', 'SCHEDULING') THEN 1 ELSE 0 END), 0) AS in_progress,
			COALESCE(SUM(CASE WHEN state = 'COMPLETED' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN state = 'BLOCKED' THEN 1 ELSE 0 END), 0) AS blocked
		FROM tasks WHERE project_id = ?
	`), projectID).StructScan(&counts)
	if err != nil {
		return nil, fmt.Errorf("get task counts for project %s: %w", projectID, err)
	}
	return &counts, nil
}
