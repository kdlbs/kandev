package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// cancellableRunStatuses is the guard on every terminal-cancel write: a run
// may only be moved to cancelled while it is still queued or claimed.
//
// Callers reach the cancel path by reading a run (or a set of run IDs) and
// then writing, so without the guard a run that the scheduler claimed and
// completed between that SELECT and this UPDATE would be stamped cancelled
// and have its real finished_at overwritten.
const cancellableRunStatuses = `status IN ('queued', 'claimed')`

// CancelRunsWhere is the single writer of the terminal cancel state on the
// runs table. CancelRun, BulkCancelRuns and the office repository's
// CancelRunsForTasks are all thin selectors over it, so the guard above
// cannot be bypassed by picking a different entry point.
//
// selector is an extra WHERE fragment ANDed onto the guard; it must be a
// SQL literal owned by the calling repository method, using ? placeholders
// (rebound here) for every value. Never interpolate caller-supplied data
// into it. selectorArgs bind those placeholders, in order.
//
// Returns the number of rows actually cancelled: 0 means every run the
// selector matched had already reached a terminal state.
func (r *Repository) CancelRunsWhere(
	ctx context.Context,
	cancelReason string,
	selector string,
	selectorArgs ...interface{},
) (int64, error) {
	args := make([]interface{}, 0, len(selectorArgs)+2)
	args = append(args, cancelReason, time.Now().UTC())
	args = append(args, selectorArgs...)

	query := fmt.Sprintf(`
		UPDATE runs
		SET status = 'cancelled', cancel_reason = ?, finished_at = ?
		WHERE %s
		  AND (%s)
	`, cancellableRunStatuses, selector)

	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CancelRun marks a run as cancelled with an optional cancel reason.
// A run that already reached a terminal state is left untouched.
func (r *Repository) CancelRun(ctx context.Context, id, cancelReason string) error {
	_, err := r.CancelRunsWhere(ctx, cancelReason, `id = ?`, id)
	return err
}

// BulkCancelRuns cancels multiple runs by ID with the given reason.
// Runs in the set that already reached a terminal state are left untouched.
func (r *Repository) BulkCancelRuns(ctx context.Context, ids []string, cancelReason string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	selector := fmt.Sprintf(`id IN (%s)`, strings.Join(placeholders, ","))
	_, err := r.CancelRunsWhere(ctx, cancelReason, selector, args...)
	return err
}
