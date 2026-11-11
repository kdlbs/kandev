package sqlite

import (
	"context"
	"fmt"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// LoadTaskStatusSummaryQuotaWaits projects only live task-session routes whose
// logical profile and generation still agree with the current session row.
// Native utility/office route owners and historical generations cannot supply a
// task wait. Primary sessions win; other sessions follow a stable newest order.
func (r *Repository) LoadTaskStatusSummaryQuotaWaits(
	ctx context.Context,
	taskIDs []string,
) (map[string]*statussummary.QuotaWaitSummary, error) {
	waits := make(map[string]*statussummary.QuotaWaitSummary, len(taskIDs))
	for _, ids := range chunkIDs(taskIDs, sqliteMaxHostParams) {
		if err := r.loadTaskStatusSummaryQuotaWaitChunk(ctx, ids, waits); err != nil {
			return nil, err
		}
	}
	return waits, nil
}

func (r *Repository) loadTaskStatusSummaryQuotaWaitChunk(
	ctx context.Context,
	taskIDs []string,
	waits map[string]*statussummary.QuotaWaitSummary,
) error {
	placeholders, args := buildInPlaceholders(taskIDs)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
        SELECT s.task_id, s.id, d.state, d.policy_state_json
        FROM task_sessions s
        JOIN dynamic_route_states d ON d.session_id = s.id
            AND d.logical_profile_id = s.agent_profile_id
            AND d.route_generation = s.route_generation
        WHERE s.task_id IN (`+placeholders+`)
            AND s.state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
            AND d.state IN ('retry_wait', 'waiting_for_reset')
        ORDER BY s.task_id, s.is_primary DESC, s.started_at DESC, s.id
    `), args...)
	if err != nil {
		return fmt.Errorf("load task quota waits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var taskID string
		var route dynamicruntime.RouteState
		if err := rows.Scan(&taskID, &route.SessionID, &route.Status, &route.PolicyStateJSON); err != nil {
			return fmt.Errorf("scan task quota wait: %w", err)
		}
		if waits[taskID] == nil {
			if wait := statussummary.QuotaWaitSummaryFromRouteState(&route); wait != nil {
				waits[taskID] = wait
			}
		}
	}
	return rows.Err()
}
