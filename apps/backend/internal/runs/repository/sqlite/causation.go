package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// ResolveAgentProfileWorkspaceID returns the workspace_id for the given
// agent profile, so causation resolution can stamp a new run's workspace
// (AC-OFFICE-RUN-CAUSATION-001.20) before the row exists. Returns
// sql.ErrNoRows when the profile is unknown.
func (r *Repository) ResolveAgentProfileWorkspaceID(ctx context.Context, agentProfileID string) (string, error) {
	var workspaceID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id FROM agent_profiles WHERE id = ?
	`), agentProfileID).Scan(&workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceID, nil
}

// CountSelfTriggeredRuns counts runs queued for agentProfileID with the
// given reason, whose persisted actor kind is `agent` and actor id equals
// agentProfileID, requested strictly after since
// (AC-OFFICE-LAUNCH-SAFETY-004.7, the strict window boundary of
// AC-OFFICE-LAUNCH-SAFETY-005.8). Counts every such row regardless of its
// current status, per AC-OFFICE-LAUNCH-SAFETY-004.5: a refused wake was
// never inserted so it was never eligible to be counted, and a queued wake
// that later finished, failed, or was cancelled still counts.
func (r *Repository) CountSelfTriggeredRuns(
	ctx context.Context, agentProfileID, reason string, since time.Time,
) (int, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE agent_profile_id = ? AND reason = ?
		  AND actor_kind = ? AND actor_id = ?
		  AND requested_at > ?
	`), agentProfileID, reason, string(models.ActorKindAgent), agentProfileID, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
