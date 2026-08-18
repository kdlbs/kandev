package sqlite

import (
	"context"
	"database/sql"
)

// HasPriorSessionForAgent returns true when a task_session row exists for the
// given (task, agent_instance) pair AND is not in CREATED state. CREATED means
// the row was just provisioned and the agent has never received any prompt yet,
// so the next launch is still a "fresh" one. Any other state (STARTING /
// RUNNING / IDLE / WAITING_FOR_INPUT / COMPLETED / FAILED / CANCELLED) means
// the agent CLI has already been launched at least once with the role prompt
// and the next launch can resume the ACP session — so the office prompt
// builder should skip the AGENTS.md preamble.
func (r *Repository) HasPriorSessionForAgent(ctx context.Context, taskID, agentInstanceID string) (bool, error) {
	// Taskless runs (PR 2 of office-heartbeat-rework) ALWAYS start a
	// fresh session — the continuation summary bridges context, not
	// the conversation log. Returning false here guarantees that
	// invariant even if a future caller forgets to check taskID up
	// front.
	if taskID == "" {
		return false, nil
	}
	if agentInstanceID == "" {
		return false, nil
	}
	var state string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT state FROM task_sessions
		 WHERE task_id = ? AND agent_profile_id = ?
		 ORDER BY started_at DESC LIMIT 1`,
	), taskID, agentInstanceID).Scan(&state)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state != "CREATED", nil
}

// GetSessionAgentProfileID returns the agent_profile_id recorded on the
// task_sessions row for the given task. This is the agent that actually ran
// the session, as opposed to RunnerProjection's workflow-configured runner.
// Returns "" if the session, task pair, or agent_profile_id is not found.
func (r *Repository) GetSessionAgentProfileID(
	ctx context.Context, taskID, sessionID string,
) (string, error) {
	var agentProfileID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(agent_profile_id, '') FROM task_sessions
		 WHERE task_id = ? AND id = ?`,
	), taskID, sessionID).Scan(&agentProfileID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return agentProfileID, nil
}
