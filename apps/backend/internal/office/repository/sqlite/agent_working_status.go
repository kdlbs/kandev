// Office agent "working" status transitions.
//
// MarkAgentWorking and ClearAgentWorking are the only production writers of
// AgentStatusWorking.
//
// Both are compare-and-swap by design. An unconditional UPDATE would let a
// late-arriving reset clobber a status set by a concurrent, higher-priority
// writer — most dangerously autoPauseAgent (office/service/failure.go),
// which pauses an agent after consecutive failures on the very same code
// path that clears "working". A CAS makes each write a no-op unless the
// agent is still in the exact state the caller observed, so the reset can
// be called redundantly from any terminal path without resurrecting a
// paused, stopped, or pending-approval agent back to idle.

package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// MarkAgentWorking transitions an agent from "idle" to "working" and records
// runID as the run that owns the transition. Returns true when this call
// performed the transition.
//
// Scoped to status = 'idle' so it cannot overwrite a status a concurrent
// writer set between the scheduler's isAgentActive check and the launch
// (a user pausing the agent, a budget pause, an approval gate). A false
// return therefore means "the agent was not idle" and is not an error: the
// run still launches, exactly as it did before this status existed.
func (r *Repository) MarkAgentWorking(ctx context.Context, id, runID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET status = 'working', working_run_id = ?, updated_at = ?
		WHERE id = ? AND status = 'idle' AND `+agentInstanceFilter+`
	`), runID, time.Now().UTC(), id)
	if err != nil {
		return false, err
	}
	return rowsChanged(res)
}

// ClearAgentWorking transitions an agent from "working" back to "idle", but
// only when runID matches the run recorded by MarkAgentWorking. Returns true
// when this call performed the transition.
//
// The runID match is what keeps a stale or duplicate terminal event for a
// finished run from clobbering a successor run's live "working" status: once
// a new run has re-marked the agent working, its runID no longer matches the
// old event's, so the old event's clear becomes a no-op instead of an
// incorrect reset. Combined with the status = 'working' scope, this is also
// what makes the call safe to repeat from every terminal path (success,
// failure, cancellation, and the never-launched branches) without ordering
// constraints between them.
func (r *Repository) ClearAgentWorking(ctx context.Context, id, runID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET status = 'idle', working_run_id = '', updated_at = ?
		WHERE id = ? AND status = 'working' AND working_run_id = ? AND `+agentInstanceFilter+`
	`), time.Now().UTC(), id, runID)
	if err != nil {
		return false, err
	}
	return rowsChanged(res)
}

func rowsChanged(res sql.Result) (bool, error) {
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
