// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// upsertSubagentContextSQL is a single atomic INSERT ... ON CONFLICT DO UPDATE.
// No read-then-write anywhere (AC-14): every merge, terminality, and
// write-once rule lives in the conflict clause.
//
//   - turn_id: the *original* turn wins (AC-2a); a later turn never
//     re-attributes the fan-out.
//   - every other nullable column: fill-forward — an absent/empty reported
//     value never blanks a value already learned (AC-4, AC-5).
//   - tool_status: frozen once settled_at is set (AC-12); until then it fills
//     forward like any other column.
//   - is_async: sticky — a later frame reporting false never resets it.
//   - settled_at: write-once (AC-11).
//   - source, observed_at, id: absent from the SET list entirely — write-once,
//     so a backfilled row is never relabelled live (AC-22) and the first
//     observation wins (AC-1a).
const upsertSubagentContextSQL = `
	INSERT INTO task_session_subagents (
		id, task_session_id, task_id, turn_id, tool_call_id, parent_tool_call_id,
		subagent_type, description, agent_id, child_session_id, model,
		agent_status, tool_status, is_async, total_tokens, tool_use_count,
		duration_ms, source, observed_at, settled_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (task_session_id, tool_call_id) DO UPDATE SET
		turn_id = COALESCE(task_session_subagents.turn_id, excluded.turn_id),
		parent_tool_call_id = COALESCE(excluded.parent_tool_call_id, task_session_subagents.parent_tool_call_id),
		subagent_type = COALESCE(excluded.subagent_type, task_session_subagents.subagent_type),
		description = COALESCE(excluded.description, task_session_subagents.description),
		agent_id = COALESCE(excluded.agent_id, task_session_subagents.agent_id),
		child_session_id = COALESCE(excluded.child_session_id, task_session_subagents.child_session_id),
		model = COALESCE(excluded.model, task_session_subagents.model),
		agent_status = COALESCE(excluded.agent_status, task_session_subagents.agent_status),
		tool_status = CASE
			WHEN task_session_subagents.settled_at IS NOT NULL THEN task_session_subagents.tool_status
			ELSE COALESCE(excluded.tool_status, task_session_subagents.tool_status)
		END,
		is_async = CASE WHEN excluded.is_async = 1 THEN 1 ELSE task_session_subagents.is_async END,
		total_tokens = COALESCE(excluded.total_tokens, task_session_subagents.total_tokens),
		tool_use_count = COALESCE(excluded.tool_use_count, task_session_subagents.tool_use_count),
		duration_ms = COALESCE(excluded.duration_ms, task_session_subagents.duration_ms),
		settled_at = COALESCE(task_session_subagents.settled_at, excluded.settled_at),
		updated_at = excluded.updated_at
`

// subagentContextSourceLive is the default models.SubagentContext.Source for
// a row observed on the live orchestrator frame path, as opposed to "backfill".
const subagentContextSourceLive = "live"

// UpsertSubagentContext inserts or merges one subagent invocation row, keyed
// on (task_session_id, tool_call_id). Safe under concurrent calls for the
// same key (AC-14) and for different keys in the same turn (AC-15): the
// merge is expressed entirely in the statement's conflict clause, never in
// Go-level read-then-write.
func (r *Repository) UpsertSubagentContext(ctx context.Context, sc *models.SubagentContext) error {
	if sc.ID == "" {
		sc.ID = uuid.New().String()
	}
	if sc.Source == "" {
		sc.Source = subagentContextSourceLive
	}
	now := time.Now().UTC()
	if sc.ObservedAt.IsZero() {
		sc.ObservedAt = now
	}
	if sc.UpdatedAt.IsZero() {
		sc.UpdatedAt = now
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(upsertSubagentContextSQL),
		sc.ID, sc.TaskSessionID, sc.TaskID, sc.TurnID, sc.ToolCallID, sc.ParentToolCallID,
		sc.SubagentType, sc.Description, sc.AgentID, sc.ChildSessionID, sc.Model,
		sc.AgentStatus, sc.ToolStatus, dialect.BoolToInt(sc.IsAsync), sc.TotalTokens, sc.ToolUseCount,
		sc.DurationMs, sc.Source, sc.ObservedAt, sc.SettledAt, sc.UpdatedAt,
	)
	return err
}

const selectSubagentContextColumns = `
	id, task_session_id, task_id, turn_id, tool_call_id, parent_tool_call_id,
	subagent_type, description, agent_id, child_session_id, model,
	agent_status, tool_status, is_async, total_tokens, tool_use_count,
	duration_ms, source, observed_at, settled_at, updated_at
`

// ListSubagentContextsBySession returns every subagent context for a session,
// ordered observed_at ASC, tool_call_id ASC. tool_call_id is the named
// tiebreak (AC-13): unique within a session by construction, stable across
// replays, and total — the generated id is deliberately not used.
func (r *Repository) ListSubagentContextsBySession(ctx context.Context, sessionID string) ([]*models.SubagentContext, error) {
	return r.listSubagentContexts(ctx, "task_session_id", sessionID)
}

// ListSubagentContextsByTurn returns every subagent context recorded under a
// turn, in the same observed_at ASC, tool_call_id ASC order. A subagent whose
// tool_call_id was later re-attributed away from this turn (AC-2a preserves
// the original turn) never appears here for the newer turn.
func (r *Repository) ListSubagentContextsByTurn(ctx context.Context, turnID string) ([]*models.SubagentContext, error) {
	return r.listSubagentContexts(ctx, "turn_id", turnID)
}

func (r *Repository) listSubagentContexts(ctx context.Context, column, value string) ([]*models.SubagentContext, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+selectSubagentContextColumns+`
		FROM task_session_subagents
		WHERE `+column+` = ?
		ORDER BY observed_at ASC, tool_call_id ASC
	`), value)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.SubagentContext
	for rows.Next() {
		sc := &models.SubagentContext{}
		var isAsync int
		if err := rows.Scan(
			&sc.ID, &sc.TaskSessionID, &sc.TaskID, &sc.TurnID, &sc.ToolCallID, &sc.ParentToolCallID,
			&sc.SubagentType, &sc.Description, &sc.AgentID, &sc.ChildSessionID, &sc.Model,
			&sc.AgentStatus, &sc.ToolStatus, &isAsync, &sc.TotalTokens, &sc.ToolUseCount,
			&sc.DurationMs, &sc.Source, &sc.ObservedAt, &sc.SettledAt, &sc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sc.IsAsync = isAsync == 1
		result = append(result, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
