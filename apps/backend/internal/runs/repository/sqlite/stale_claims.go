package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

// StaleClaimedTaskRun identifies a task-bound run that has stayed claimed
// past the stale-claim cutoff.
type StaleClaimedTaskRun struct {
	ID             string `db:"id"`
	AgentProfileID string `db:"agent_profile_id"`
	TaskID         string `db:"task_id"`
	SessionID      string `db:"session_id"`
}

// ListStaleClaimedTaskRuns returns the task-bound runs that RecoverStale
// would requeue at the same cutoff.
func (r *Repository) ListStaleClaimedTaskRuns(
	ctx context.Context, claimedOlderThan time.Time,
) ([]StaleClaimedTaskRun, error) {
	taskIDExpr := dialect.JSONExtract(r.db.DriverName(), "payload", "task_id")
	var runs []StaleClaimedTaskRun
	err := r.db.SelectContext(ctx, &runs, r.db.Rebind(fmt.Sprintf(`
		SELECT id, agent_profile_id,
		       COALESCE(%s, '') AS task_id,
		       COALESCE(session_id, '') AS session_id
		FROM runs
		WHERE status = 'claimed' AND claimed_at < ?
		  AND COALESCE(%s, '') <> ''
		ORDER BY claimed_at ASC, id ASC
	`, taskIDExpr, taskIDExpr)), claimedOlderThan)
	if err != nil {
		return nil, err
	}
	return runs, nil
}

// RenewStaleClaim moves a stale claim's claimed_at to now while the run is
// still claimed and still older than the cutoff. It reports whether the
// row was renewed; false means a concurrent writer already settled,
// requeued, or renewed the run.
func (r *Repository) RenewStaleClaim(
	ctx context.Context, runID string, claimedOlderThan, now time.Time,
) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET claimed_at = ?
		WHERE id = ? AND status = 'claimed' AND claimed_at < ?
	`), now, runID, claimedOlderThan)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
