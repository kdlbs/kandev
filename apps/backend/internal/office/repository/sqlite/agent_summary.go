package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AgentRunDayRow holds per-day run outcome counts for one agent.
// Status values mirror the runs.status column:
// queued | claimed | finished | failed | cancelled.
type AgentRunDayRow struct {
	Date      string `db:"date"`
	Succeeded int    `db:"succeeded"`
	Failed    int    `db:"failed"`
	Other     int    `db:"other"`
}

// RunCountsByDayForAgent returns per-day run outcome counts for an agent
// over the last `days` days, bucketed by `requested_at`. The cutoff is
// the start of (today - days + 1) so the result range is inclusive of
// today and `days` days back.
//
// finished → succeeded; failed (and timed_out, treated as failed
// upstream) → failed; everything else (cancelled, queued, claimed) →
// other.
func (r *Repository) RunCountsByDayForAgent(
	ctx context.Context, agentID string, days int,
) ([]AgentRunDayRow, error) {
	if days <= 0 {
		days = 14
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var rows []AgentRunDayRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT strftime('%Y-%m-%d', requested_at) AS date,
		       SUM(CASE WHEN status = 'finished' THEN 1 ELSE 0 END) AS succeeded,
		       SUM(CASE WHEN status IN ('failed','timed_out') THEN 1 ELSE 0 END) AS failed,
		       SUM(CASE WHEN status NOT IN ('finished','failed','timed_out') THEN 1 ELSE 0 END) AS other
		FROM runs
		WHERE agent_profile_id = ?
		  AND strftime('%Y-%m-%d', requested_at) >= ?
		GROUP BY strftime('%Y-%m-%d', requested_at)
		ORDER BY date
	`), agentID, cutoff)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AgentTaskPriorityDayRow holds per-day task counts bucketed by priority
// (critical|high|medium|low) for tasks the agent worked on. Activity is
// sourced from office_activity_log entries with target_type='task' and
// the agent's id as actor_id.
type AgentTaskPriorityDayRow struct {
	Date     string `db:"date"`
	Critical int    `db:"critical"`
	High     int    `db:"high"`
	Medium   int    `db:"medium"`
	Low      int    `db:"low"`
}

// TasksByPriorityByDayForAgent returns per-day distinct-task counts
// bucketed by priority. A task is counted on each day it had activity
// (one row per (date, priority) bucket regardless of how many activity
// entries touched it that day).
func (r *Repository) TasksByPriorityByDayForAgent(
	ctx context.Context, agentID string, days int,
) ([]AgentTaskPriorityDayRow, error) {
	if days <= 0 {
		days = 14
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var rows []AgentTaskPriorityDayRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		WITH daily AS (
			SELECT strftime('%Y-%m-%d', a.created_at) AS date,
			       a.target_id AS task_id,
			       COALESCE(t.priority, 'medium') AS priority
			FROM office_activity_log a
			JOIN tasks t ON t.id = a.target_id
			WHERE a.actor_id = ?
			  AND a.target_type = 'task'
			  AND a.target_id != ''
			  AND strftime('%Y-%m-%d', a.created_at) >= ?
			GROUP BY date, a.target_id, t.priority
		)
		SELECT date,
		       SUM(CASE WHEN priority = 'critical' THEN 1 ELSE 0 END) AS critical,
		       SUM(CASE WHEN priority = 'high'     THEN 1 ELSE 0 END) AS high,
		       SUM(CASE WHEN priority = 'medium'   THEN 1 ELSE 0 END) AS medium,
		       SUM(CASE WHEN priority = 'low'      THEN 1 ELSE 0 END) AS low
		FROM daily
		GROUP BY date
		ORDER BY date
	`), agentID, cutoff)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AgentTaskStatusDayRow holds per-day distinct-task counts bucketed by
// task status (todo|in_progress|in_review|done|blocked|cancelled|backlog).
// The status reflects each task's CURRENT status (a snapshot of where
// the task sits today, not its status at the time of activity).
type AgentTaskStatusDayRow struct {
	Date       string `db:"date"`
	Todo       int    `db:"todo"`
	InProgress int    `db:"in_progress"`
	InReview   int    `db:"in_review"`
	Done       int    `db:"done"`
	Blocked    int    `db:"blocked"`
	Cancelled  int    `db:"cancelled"`
	Backlog    int    `db:"backlog"`
}

// TasksByStatusByDayForAgent returns per-day distinct-task counts
// bucketed by current task status.
func (r *Repository) TasksByStatusByDayForAgent(
	ctx context.Context, agentID string, days int,
) ([]AgentTaskStatusDayRow, error) {
	if days <= 0 {
		days = 14
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	var rows []AgentTaskStatusDayRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		WITH daily AS (
			SELECT strftime('%Y-%m-%d', a.created_at) AS date,
			       a.target_id AS task_id,
			       UPPER(COALESCE(t.state, 'TODO')) AS state
			FROM office_activity_log a
			JOIN tasks t ON t.id = a.target_id
			WHERE a.actor_id = ?
			  AND a.target_type = 'task'
			  AND a.target_id != ''
			  AND strftime('%Y-%m-%d', a.created_at) >= ?
			GROUP BY date, a.target_id, t.state
		)
		SELECT date,
		       SUM(CASE WHEN state = 'TODO'        THEN 1 ELSE 0 END) AS todo,
		       SUM(CASE WHEN state = 'IN_PROGRESS' THEN 1 ELSE 0 END) AS in_progress,
		       SUM(CASE WHEN state = 'REVIEW'      THEN 1 ELSE 0 END) AS in_review,
		       SUM(CASE WHEN state = 'COMPLETED'   THEN 1 ELSE 0 END) AS done,
		       SUM(CASE WHEN state = 'BLOCKED'     THEN 1 ELSE 0 END) AS blocked,
		       SUM(CASE WHEN state = 'CANCELLED'   THEN 1 ELSE 0 END) AS cancelled,
		       SUM(CASE WHEN state = 'BACKLOG'     THEN 1 ELSE 0 END) AS backlog
		FROM daily
		GROUP BY date
		ORDER BY date
	`), agentID, cutoff)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AgentRecentTaskRow is a slim row for the recent-tasks list on the
// agent dashboard. LastActiveAt is the most recent activity_log
// created_at on this (agent, task) pair.
type AgentRecentTaskRow struct {
	TaskID       string `db:"task_id"`
	Identifier   string `db:"identifier"`
	Title        string `db:"title"`
	State        string `db:"state"`
	LastActiveAt string `db:"last_active_at"`
}

// RecentTasksForAgent returns the `limit` most-recently-active tasks
// the agent has touched, sorted by descending last activity timestamp.
// A task is "touched" when an activity_log row exists with the agent's
// id as actor_id and target_type='task'.
func (r *Repository) RecentTasksForAgent(
	ctx context.Context, agentID string, limit int,
) ([]AgentRecentTaskRow, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AgentRecentTaskRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT t.id AS task_id,
		       COALESCE(t.identifier, '') AS identifier,
		       t.title,
		       COALESCE(t.state, 'TODO') AS state,
		       MAX(a.created_at) AS last_active_at
		FROM office_activity_log a
		JOIN tasks t ON t.id = a.target_id
		WHERE a.actor_id = ?
		  AND a.target_type = 'task'
		  AND a.target_id != ''
		GROUP BY t.id, t.identifier, t.title, t.state
		ORDER BY last_active_at DESC
		LIMIT ?
	`), agentID, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AgentCostAggregate is the all-time cost rollup for one agent.
// TotalCostSubcents stores hundredths of a cent (UI divides by 10000).
type AgentCostAggregate struct {
	InputTokens       int64 `db:"input_tokens"`
	OutputTokens      int64 `db:"output_tokens"`
	CachedTokens      int64 `db:"cached_tokens"`
	TotalCostSubcents int64 `db:"total_cost_subcents"`
}

// CostAggregateForAgent returns the all-time cost rollup for an agent.
// Aggregates over `office_cost_events.agent_profile_id`. Returns a
// zero-value struct (not nil) when the agent has no cost events.
func (r *Repository) CostAggregateForAgent(
	ctx context.Context, agentID string,
) (AgentCostAggregate, error) {
	var agg AgentCostAggregate
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(SUM(tokens_in), 0)         AS input_tokens,
		       COALESCE(SUM(tokens_out), 0)        AS output_tokens,
		       COALESCE(SUM(tokens_cached_in), 0)  AS cached_tokens,
		       COALESCE(SUM(cost_subcents), 0)     AS total_cost_subcents
		FROM office_cost_events
		WHERE agent_profile_id = ?
	`), agentID).StructScan(&agg)
	if err != nil {
		return AgentCostAggregate{}, err
	}
	return agg, nil
}

// AgentRunCostRow holds the per-run cost rollup for the recent-runs
// costs table on the agent dashboard. Cost events are joined to runs
// via session_id (for the run's claimed session) and via task_id (for
// runs whose primary payload is a task). CostSubcents stores hundredths
// of a cent (UI divides by 10000).
type AgentRunCostRow struct {
	RunID        string `db:"run_id"`
	RequestedAt  string `db:"requested_at"`
	InputTokens  int64  `db:"input_tokens"`
	OutputTokens int64  `db:"output_tokens"`
	CostSubcents int64  `db:"cost_subcents"`
}

// RecentRunCostsForAgent returns the `limit` most recent runs that
// have associated cost events, with per-run aggregates. Cost events
// are matched to a run by joining on the run's claimed task_session
// (via task_sessions.task_id = json_extract(runs.payload,
// '$.task_id') and task_sessions.started_at >= runs.claimed_at).
//
// To keep the join cheap and predictable we approximate per-run cost
// as: cost events whose task_id matches the run's payload task_id and
// whose occurred_at is between the run's claimed_at and finished_at
// (or now() when the run is still claimed).
func (r *Repository) RecentRunCostsForAgent(
	ctx context.Context, agentID string, limit int,
) ([]AgentRunCostRow, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AgentRunCostRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		WITH agent_runs AS (
			SELECT id,
			       requested_at,
			       claimed_at,
			       COALESCE(finished_at, datetime('now')) AS upper_at,
			       json_extract(payload, '$.task_id') AS task_id
			FROM runs
			WHERE agent_profile_id = ?
		),
		costs AS (
			SELECT r.id AS run_id,
			       r.requested_at,
			       SUM(c.tokens_in)     AS input_tokens,
			       SUM(c.tokens_out)    AS output_tokens,
			       SUM(c.cost_subcents) AS cost_subcents
			FROM agent_runs r
			JOIN office_cost_events c
			  ON c.agent_profile_id = ?
			 AND c.task_id = r.task_id
			 AND (r.claimed_at IS NULL OR c.occurred_at >= r.claimed_at)
			 AND c.occurred_at <= r.upper_at
			GROUP BY r.id, r.requested_at
		)
		SELECT run_id,
		       requested_at,
		       COALESCE(input_tokens, 0)   AS input_tokens,
		       COALESCE(output_tokens, 0)  AS output_tokens,
		       COALESCE(cost_subcents, 0)  AS cost_subcents
		FROM costs
		ORDER BY requested_at DESC
		LIMIT ?
	`), agentID, agentID, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// LatestRunForAgent returns the most recent run row for an agent
// (by requested_at desc), or nil if the agent has no runs.
//
// We expose this directly so the dashboard summary can render the
// "Latest Run" card without needing the full Runs list.
func (r *Repository) LatestRunForAgent(ctx context.Context, agentID string) (*RunSummaryRow, error) {
	var row RunSummaryRow
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT id,
		       agent_profile_id,
		       reason,
		       payload,
		       status,
		       COALESCE(error_message, '') AS error_message,
		       requested_at,
		       claimed_at,
		       finished_at
		FROM runs
		WHERE agent_profile_id = ?
		ORDER BY requested_at DESC, id DESC
		LIMIT 1
	`), agentID).StructScan(&row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// RunSummaryRow is a slim view of one run for dashboard cards.
type RunSummaryRow struct {
	ID             string     `db:"id"`
	AgentProfileID string     `db:"agent_profile_id"`
	Reason         string     `db:"reason"`
	Payload        string     `db:"payload"`
	Status         string     `db:"status"`
	ErrorMessage   string     `db:"error_message"`
	RequestedAt    time.Time  `db:"requested_at"`
	ClaimedAt      *time.Time `db:"claimed_at"`
	FinishedAt     *time.Time `db:"finished_at"`
}
