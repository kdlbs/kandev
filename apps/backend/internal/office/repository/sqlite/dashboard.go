package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// RunActivityRow is a raw row from the run-activity aggregate query.
type RunActivityRow struct {
	Date      string `db:"date"`
	Succeeded int    `db:"succeeded"`
	Failed    int    `db:"failed"`
	Other     int    `db:"other"`
}

// QueryRunActivity returns per-day run outcome counts for the last n days,
// joining task_sessions through tasks to scope by workspace.
func (r *Repository) QueryRunActivity(ctx context.Context, workspaceID string, days int) ([]RunActivityRow, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	var rows []RunActivityRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT DATE(ts.started_at) as date,
		       SUM(CASE WHEN ts.state = 'COMPLETED' THEN 1 ELSE 0 END) as succeeded,
		       SUM(CASE WHEN ts.state = 'FAILED' THEN 1 ELSE 0 END) as failed,
		       COUNT(*) - SUM(CASE WHEN ts.state IN ('COMPLETED','FAILED') THEN 1 ELSE 0 END) as other
		FROM task_sessions ts
		WHERE ts.started_at >= ?
		  AND ts.task_id IN (
		      SELECT id FROM tasks WHERE workspace_id = ? AND is_ephemeral = 0
		  )
		GROUP BY DATE(ts.started_at)
		ORDER BY date
	`), cutoff, workspaceID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// TaskBreakdownRow is a raw row from the task-breakdown aggregate query.
type TaskBreakdownRow struct {
	State string `db:"state"`
	Count int    `db:"count"`
}

// QueryTaskBreakdown returns task counts grouped by state for a workspace.
func (r *Repository) QueryTaskBreakdown(ctx context.Context, workspaceID string) ([]TaskBreakdownRow, error) {
	var rows []TaskBreakdownRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT COALESCE(state,'') as state, COUNT(*) as count
		FROM tasks
		WHERE workspace_id = ?
		  AND is_ephemeral = 0
		  AND archived_at IS NULL
		GROUP BY state
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// RecentTaskRow is a raw row from the recent-tasks query.
type RecentTaskRow struct {
	ID                     string `db:"id"`
	Identifier             string `db:"identifier"`
	Title                  string `db:"title"`
	State                  string `db:"state"`
	AssigneeAgentProfileID string `db:"assignee"`
	UpdatedAt              string `db:"updated_at"`
}

// QueryRecentTasks returns the n most recently updated non-archived tasks for a workspace.
func (r *Repository) QueryRecentTasks(ctx context.Context, workspaceID string, limit int) ([]RecentTaskRow, error) {
	var rows []RecentTaskRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT id,
		       COALESCE(identifier,'') as identifier,
		       title,
		       COALESCE(state,'') as state,
		       `+RunnerProjection("tasks")+` as assignee,
		       updated_at
		FROM tasks
		WHERE workspace_id = ?
		  AND is_ephemeral = 0
		  AND archived_at IS NULL
		ORDER BY updated_at DESC
		LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// LiveSessionRow is a raw row from the recent-sessions query for live runs.
type LiveSessionRow struct {
	ID          string  `db:"id"`
	TaskID      string  `db:"task_id"`
	AgentExecID string  `db:"agent_execution_id"`
	State       string  `db:"state"`
	StartedAt   string  `db:"started_at"`
	CompletedAt *string `db:"completed_at"`
}

// QueryRecentSessions returns the n most recent task sessions for a workspace,
// joining through tasks for workspace scoping.
func (r *Repository) QueryRecentSessions(ctx context.Context, workspaceID string, limit int) ([]LiveSessionRow, error) {
	var rows []LiveSessionRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT ts.id,
		       ts.task_id,
		       COALESCE(ts.agent_execution_id,'') as agent_execution_id,
		       COALESCE(ts.state,'') as state,
		       ts.started_at,
		       ts.completed_at
		FROM task_sessions ts
		WHERE ts.task_id IN (
		      SELECT id FROM tasks WHERE workspace_id = ? AND is_ephemeral = 0
		)
		ORDER BY ts.started_at DESC
		LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// TaskTitleRow holds the minimal task fields needed for live-runs enrichment.
type TaskTitleRow struct {
	ID         string `db:"id"`
	Title      string `db:"title"`
	Identifier string `db:"identifier"`
}

// GetTasksByIDs returns minimal task info for a set of task IDs.
// IDs not found in the database are silently omitted from the result.
func (r *Repository) GetTasksByIDs(ctx context.Context, ids []string) ([]TaskTitleRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT id,
	                 COALESCE(title,'') as title,
	                 COALESCE(identifier,'') as identifier
	          FROM tasks
	          WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	var rows []TaskTitleRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// AgentSessionRow is a raw row from the per-agent recent-sessions query.
// Mirrors LiveSessionRow but adds the agent_profile_id column so callers
// can group rows by agent without re-querying.
type AgentSessionRow struct {
	ID             string  `db:"id"`
	TaskID         string  `db:"task_id"`
	AgentProfileID string  `db:"agent_profile_id"`
	State          string  `db:"state"`
	StartedAt      string  `db:"started_at"`
	CompletedAt    *string `db:"completed_at"`
	// UpdatedAt is the moment the row's state last changed. For office
	// IDLE sessions (which never set completed_at), this is the
	// stable "turn ended" timestamp the dashboard duration math uses.
	UpdatedAt string `db:"updated_at"`
}

// ListRecentSessionsByAgentBatch returns up to perAgentLimit most-recent
// task_sessions rows for each of the supplied agent_profile_ids, in a single
// query. Result map keys are agent_profile_id; rows within each slice are
// ordered started_at DESC. Empty input returns an empty map.
//
// Uses the SQLite window function ROW_NUMBER() (requires SQLite ≥ 3.25,
// which mattn/go-sqlite3 v1.14.x bundles). This replaces the per-agent loop
// in dashboard.collectAgentSessions to keep GetAgentSummaries query count
// constant regardless of agent count.
func (r *Repository) ListRecentSessionsByAgentBatch(
	ctx context.Context, agentInstanceIDs []string, perAgentLimit int,
) (map[string][]AgentSessionRow, error) {
	out := make(map[string][]AgentSessionRow, len(agentInstanceIDs))
	if len(agentInstanceIDs) == 0 {
		return out, nil
	}
	if perAgentLimit <= 0 {
		perAgentLimit = 5
	}
	placeholders := make([]string, len(agentInstanceIDs))
	args := make([]interface{}, 0, len(agentInstanceIDs)+1)
	for i, id := range agentInstanceIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, perAgentLimit)
	query := `SELECT id, task_id, agent_profile_id, state, started_at, completed_at, updated_at
	          FROM (
	              SELECT ts.id,
	                     ts.task_id,
	                     COALESCE(ts.agent_profile_id, '') as agent_profile_id,
	                     COALESCE(ts.state, '') as state,
	                     ts.started_at,
	                     ts.completed_at,
	                     ts.updated_at,
	                     ROW_NUMBER() OVER (
	                         PARTITION BY ts.agent_profile_id
	                         ORDER BY ts.started_at DESC
	                     ) AS rn
	              FROM task_sessions ts
	              WHERE ts.agent_profile_id IN (` + strings.Join(placeholders, ",") + `)
	          ) sub
	          WHERE rn <= ?
	          ORDER BY agent_profile_id, started_at DESC`
	var rows []AgentSessionRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AgentProfileID] = append(out[row.AgentProfileID], row)
	}
	return out, nil
}

// SessionCommandCountRow holds a session_id → tool_call count pair.
type SessionCommandCountRow struct {
	SessionID string `db:"session_id"`
	Count     int    `db:"count"`
}

// CountToolCallMessagesBySession returns the number of tool_call messages for
// each session in `sessionIDs`. Sessions with zero tool calls are omitted from
// the result map. The query reads from task_session_messages.
func (r *Repository) CountToolCallMessagesBySession(ctx context.Context, sessionIDs []string) (map[string]int, error) {
	if len(sessionIDs) == 0 {
		return map[string]int{}, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, 0, len(sessionIDs)+1)
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := `SELECT task_session_id as session_id, COUNT(*) as count
	          FROM task_session_messages
	          WHERE type = 'tool_call'
	            AND task_session_id IN (` + strings.Join(placeholders, ",") + `)
	          GROUP BY task_session_id`
	var rows []SessionCommandCountRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.SessionID] = row.Count
	}
	return out, nil
}

// BucketTaskBreakdown converts raw state-count rows into a TaskBreakdown.
func BucketTaskBreakdown(rows []TaskBreakdownRow) models.TaskBreakdown {
	var bd models.TaskBreakdown
	for _, row := range rows {
		switch row.State {
		case "COMPLETED":
			bd.Done += row.Count
		case "IN_PROGRESS", "SCHEDULING":
			bd.InProgress += row.Count
		case "BLOCKED":
			bd.Blocked += row.Count
		default:
			bd.Open += row.Count
		}
	}
	return bd
}
