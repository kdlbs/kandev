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

// CancelledRun identifies one row CancelRunsWhere actually cancelled,
// carrying the fields a caller needs to classify the transition's
// terminal shape (office_loop_terminal_total) without a second read.
type CancelledRun struct {
	ID             string
	AgentProfileID string
	SessionID      string
	RequestedAt    time.Time
}

// CancelRunsWhere is the single writer of the terminal cancel state on the
// runs table. CancelRun, BulkCancelRuns and the office repository's
// CancelRunsForTasks and CancelDisplacedParticipantRun are all thin
// selectors over it, so the guard above cannot be bypassed by picking a
// different entry point.
//
// selector is an extra WHERE fragment ANDed onto the guard; it must be a
// SQL literal owned by the calling repository method, using ? placeholders
// (rebound here) for every value. Never interpolate caller-supplied data
// into it. selectorArgs bind those placeholders, in order.
//
// Returns the rows actually cancelled: an empty slice means every run the
// selector matched had already reached a terminal state. The guard and the
// write are the same UPDATE statement (RETURNING reads the classification
// columns off the rows it just changed), so there is no separate read a
// row's status could change behind: a run that reaches a terminal state on
// another connection is either fully visible to this statement or not, and
// either way it cannot be cancelled by it.
func (r *Repository) CancelRunsWhere(
	ctx context.Context,
	cancelReason string,
	selector string,
	selectorArgs ...interface{},
) ([]CancelledRun, error) {
	query := fmt.Sprintf(`
		UPDATE runs
		SET status = 'cancelled', cancel_reason = ?, finished_at = ?
		WHERE %s
		  AND (%s)
		RETURNING id, agent_profile_id, session_id, requested_at
	`, cancellableRunStatuses, selector)
	args := append([]interface{}{cancelReason, time.Now().UTC()}, selectorArgs...)
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var cancelled []CancelledRun
	for rows.Next() {
		var row CancelledRun
		if err := rows.Scan(&row.ID, &row.AgentProfileID, &row.SessionID, &row.RequestedAt); err != nil {
			return nil, err
		}
		cancelled = append(cancelled, row)
	}
	return cancelled, rows.Err()
}

// CancelRun marks a run as cancelled with an optional cancel reason.
// A run that already reached a terminal state is left untouched. Returns
// whether this call actually cancelled the row, so a caller counting the
// transition (office_loop_terminal_total) can tell a persisted change
// from a no-op on a run that was already terminal.
func (r *Repository) CancelRun(ctx context.Context, id, cancelReason string) (bool, error) {
	rows, err := r.CancelRunsWhere(ctx, cancelReason, `id = ?`, id)
	return len(rows) > 0, err
}

// BulkCancelRuns cancels multiple runs by ID with the given reason,
// returning the rows actually cancelled (see CancelRunsWhere). Runs in
// the set that already reached a terminal state are left untouched.
func (r *Repository) BulkCancelRuns(ctx context.Context, ids []string, cancelReason string) ([]CancelledRun, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	selector := fmt.Sprintf(`id IN (%s)`, strings.Join(placeholders, ","))
	return r.CancelRunsWhere(ctx, cancelReason, selector, args...)
}
