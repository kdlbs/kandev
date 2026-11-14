package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// ListStaleClaimedRuns returns the claims that are older than cutoff. The
// caller checks execution state without holding a database transaction.
func (r *Repository) ListStaleClaimedRuns(ctx context.Context, cutoff time.Time) ([]*runmodels.Run, error) {
	var runs []*runmodels.Run
	query := fmt.Sprintf(`
		SELECT * FROM runs
		WHERE status = 'claimed' AND %s
		ORDER BY claimed_at ASC, id ASC
	`, staleClaimTimestampPredicate(r.ro.DriverName()))
	if err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(query), cutoff); err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []*runmodels.Run{}
	}
	return runs, nil
}

// RecoverStaleClaimedRun requeues one unchanged stale claim. Matching the
// observed claim, session, and route attempt prevents a probe from overwriting
// completion, cancellation, a successor claim, or newer routing ownership.
func (r *Repository) RecoverStaleClaimedRun(
	ctx context.Context, observed *runmodels.Run, cutoff time.Time,
) (bool, error) {
	if observed == nil || observed.ClaimedAt == nil {
		return false, nil
	}
	claimTimestampMatch := "claimed_at = ?"
	if r.db.DriverName() == dialect.SQLite3 {
		claimTimestampMatch = "julianday(claimed_at) = julianday(?)"
	}
	cutoffMatch := staleClaimTimestampPredicate(r.db.DriverName())
	query := fmt.Sprintf(`
		UPDATE runs
		SET status = 'queued', claimed_at = NULL,
		    priority_class = CASE WHEN priority_class = ? THEN priority_class ELSE ? END
		WHERE id = ?
		  AND status = 'claimed'
		  AND %s
		  AND %s
		  AND COALESCE(session_id, '') = ?
		  AND current_route_attempt_seq = ?
	`, claimTimestampMatch, cutoffMatch)
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), runmodels.PriorityClassHuman, runmodels.PriorityClassRecovery,
		observed.ID, observed.ClaimedAt, cutoff, observed.SessionID, observed.CurrentRouteAttemptSeq)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func staleClaimTimestampPredicate(driver string) string {
	if driver == dialect.SQLite3 {
		// SQLite rows include both Go timestamps and legacy datetime() text.
		return "julianday(claimed_at) < julianday(?)"
	}
	return "claimed_at < ?"
}
