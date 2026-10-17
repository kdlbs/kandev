package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// ListUnstartedClaimedTasklessRuns preserves an already-claimed common grant
// through the gap before the immutable Office attempt is registered. An active
// attempt is counted by ListAdmittedRunExecutions instead, never twice.
func (r *Repository) ListUnstartedClaimedTasklessRuns(ctx context.Context) ([]runmodels.Run, error) {
	var rows []runmodels.Run
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT w.* FROM runs w JOIN agent_profiles a ON a.id=w.agent_profile_id AND (w.workspace_id='' OR a.workspace_id=w.workspace_id)
	WHERE w.status='claimed' AND w.routing_blocked_status IS NULL AND COALESCE(a.status,'idle') NOT IN ('paused','stopped','pending_approval')
	AND NOT EXISTS(SELECT 1 FROM office_run_sessions s WHERE s.run_id=w.id AND s.state IN (?,?))
	ORDER BY w.requested_at,w.id`), models.RunSessionStatePreparing, models.RunSessionStateRunning)
	if err != nil {
		return nil, err
	}
	result := make([]runmodels.Run, 0, len(rows))
	for _, row := range rows {
		if runmodels.PayloadTaskID(row.Payload) == "" {
			result = append(result, row)
		}
	}
	return result, nil
}
