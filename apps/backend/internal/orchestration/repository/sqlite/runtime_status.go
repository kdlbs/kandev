package sqlite

import "context"

// SetRuntimeWorking updates only execution state, preserving concurrent profile edits and pauses.
func (r *Repository) SetRuntimeWorking(ctx context.Context, id string, working bool) error {
	from, to := "working", runtimeIdle
	if working {
		from, to = runtimeIdle, "working"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE agent_profiles SET status=? WHERE id=? AND status=? AND deleted_at IS NULL AND EXISTS(SELECT 1 FROM workspace_orchestrators WHERE agent_id=agent_profiles.id)`), to, id, from)
	return err
}
