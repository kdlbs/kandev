package sqlite

import (
	"context"
	"fmt"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// The same unstarted task-bound run owns capacity waiting. An exact payload
// CAS cannot requeue a cancelled, already-launched, foreign or replaced intent.
func (r *Repository) RequeueUnstartedTaskRunForCapacity(ctx context.Context, runID, taskID string) error {
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil || runmodels.PayloadTaskID(run.Payload) != taskID || run.SessionID != "" || run.RoutingBlockedStatus != nil {
		return fmt.Errorf("office task capacity owner no longer matches")
	}
	if run.Status == runmodels.RunStatusQueued {
		return nil
	}
	if run.Status != runmodels.RunStatusClaimed {
		return fmt.Errorf("office task capacity owner is no longer claimed")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE runs SET status='queued',claimed_at=NULL WHERE id=? AND status='claimed' AND session_id='' AND payload=? AND routing_blocked_status IS NULL`), runID, run.Payload)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("office task capacity requeue lost its exact owner")
	}
	return nil
}
