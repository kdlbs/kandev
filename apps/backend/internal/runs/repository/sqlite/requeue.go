package sqlite

import "context"

// RequeueClaimedRun CAS-transitions a claimed run back to queued
// (clearing claimed_at) without touching retry_count or scheduled_retry_at
// — unlike ScheduleRetry, this is not a failure/backoff path. It exists so
// a gate discovered only after claim (for example a workspace pause, read
// right before launch) can leave the run in its most retryable state
// instead of finishing it, so the same run resumes cleanly once the gate
// lifts. Reports whether this call actually made the transition; false
// (with a nil error) means the run was no longer claimed by the time this
// ran (already progressed or requeued elsewhere) and is not an error.
func (r *Repository) RequeueClaimedRun(ctx context.Context, runID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', claimed_at = NULL
		WHERE id = ? AND status = 'claimed'
	`), runID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
