package sqlite

import (
	"context"
	"database/sql"
	"time"
)

// RuntimeState holds the DB-persisted runtime state for an agent instance.
type RuntimeState struct {
	AgentID              string     `db:"agent_id"`
	Status               string     `db:"status"`
	PauseReason          string     `db:"pause_reason"`
	LastWakeupFinishedAt *time.Time `db:"last_wakeup_finished_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

// GetAgentRuntime returns the runtime state for a single agent.
// Returns nil, nil if the agent has no runtime row.
func (r *Repository) GetAgentRuntime(ctx context.Context, agentID string) (*RuntimeState, error) {
	var state RuntimeState
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_agent_runtime WHERE agent_id = ?`), agentID).StructScan(&state)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// UpsertAgentRuntime creates or updates the runtime state for an agent.
func (r *Repository) UpsertAgentRuntime(ctx context.Context, agentID, status, pauseReason string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_agent_runtime (agent_id, status, pause_reason, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			status = excluded.status,
			pause_reason = excluded.pause_reason,
			updated_at = excluded.updated_at
	`), agentID, status, pauseReason, now)
	return err
}

// DeleteAgentRuntime removes the runtime row for an agent.
func (r *Repository) DeleteAgentRuntime(ctx context.Context, agentID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_agent_runtime WHERE agent_id = ?`), agentID)
	return err
}

// ListAgentRuntimes returns all runtime rows keyed by agent ID.
func (r *Repository) ListAgentRuntimes(ctx context.Context) (map[string]*RuntimeState, error) {
	var rows []*RuntimeState
	err := r.ro.SelectContext(ctx, &rows,
		`SELECT * FROM orchestrate_agent_runtime`)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*RuntimeState, len(rows))
	for _, row := range rows {
		result[row.AgentID] = row
	}
	return result, nil
}

// UpdateRuntimeLastWakeupFinished records the time an agent's wakeup finished.
func (r *Repository) UpdateRuntimeLastWakeupFinished(ctx context.Context, agentID string, finishedAt time.Time) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_agent_runtime
		SET last_wakeup_finished_at = ?, updated_at = ?
		WHERE agent_id = ?
	`), finishedAt, now, agentID)
	return err
}

// ListDistinctTriggerRoutineIDs returns the set of routine IDs that have triggers in the DB.
func (r *Repository) ListDistinctTriggerRoutineIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids,
		`SELECT DISTINCT routine_id FROM orchestrate_routine_triggers`)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// DeleteTriggersByRoutineID removes all triggers for a routine.
func (r *Repository) DeleteTriggersByRoutineID(ctx context.Context, routineID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_routine_triggers WHERE routine_id = ?`), routineID)
	return err
}

// DeleteRunsByRoutineID removes all runs for a routine.
func (r *Repository) DeleteRunsByRoutineID(ctx context.Context, routineID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_routine_runs WHERE routine_id = ?`), routineID)
	return err
}

// DeleteBudgetPoliciesForRemovedScopes removes budget policies whose scope_id
// is no longer in the provided set of valid IDs.
func (r *Repository) DeleteBudgetPoliciesForRemovedScopes(
	ctx context.Context, scopeType string, validIDs []string,
) (int64, error) {
	if len(validIDs) == 0 {
		res, err := r.db.ExecContext(ctx, r.db.Rebind(
			`DELETE FROM orchestrate_budget_policies WHERE scope_type = ?`), scopeType)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	// Build placeholders for the IN clause.
	query := `DELETE FROM orchestrate_budget_policies WHERE scope_type = ? AND scope_id NOT IN (`
	args := []interface{}{scopeType}
	for i, id := range validIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, id)
	}
	query += ")"
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteChannelsForRemovedAgents removes channels whose agent_instance_id
// is no longer in the provided set of valid IDs.
func (r *Repository) DeleteChannelsForRemovedAgents(
	ctx context.Context, validAgentIDs []string,
) (int64, error) {
	if len(validAgentIDs) == 0 {
		res, err := r.db.ExecContext(ctx,
			`DELETE FROM orchestrate_channels`)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	query := `DELETE FROM orchestrate_channels WHERE agent_instance_id NOT IN (`
	args := make([]interface{}, 0, len(validAgentIDs))
	for i, id := range validAgentIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, id)
	}
	query += ")"
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
