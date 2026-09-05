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
	"time"
)

// MarkAgentWorking transitions an agent from "idle" to "working". Returns
// true when this call performed the transition.
//
// Scoped to status = 'idle' so it cannot overwrite a status a concurrent
// writer set between the scheduler's isAgentActive check and the launch
// (a user pausing the agent, a budget pause, an approval gate). A false
// return therefore means "the agent was not idle" and is not an error: the
// run still launches, exactly as it did before this status existed.
func (r *Repository) MarkAgentWorking(ctx context.Context, id string) (bool, error) {
	return r.swapAgentStatus(ctx, id, "idle", "working")
}

// ClearAgentWorking transitions an agent from "working" back to "idle".
// Returns true when this call performed the transition.
//
// Scoped to status = 'working' so it is a no-op for an agent that has
// already moved on. This is what makes it safe to call from every terminal
// path (success, failure, cancellation, and the never-launched branches)
// without ordering constraints between them.
func (r *Repository) ClearAgentWorking(ctx context.Context, id string) (bool, error) {
	return r.swapAgentStatus(ctx, id, "working", "idle")
}

// swapAgentStatus performs a conditional status update, matching
// UpdateAgentStatusFields' office-row scoping. pause_reason is deliberately
// left untouched: neither transition here is a pause, and clearing the
// column would erase a reason another writer owns.
func (r *Repository) swapAgentStatus(ctx context.Context, id, from, to string) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ? AND `+agentInstanceFilter+`
	`), to, time.Now().UTC(), id, from)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
