package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db/dialect"
)

// parseTimeString parses time strings in various SQLite formats
func parseTimeString(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try various common SQLite datetime formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05.000",
		"2006-01-02T15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// GetTaskStats retrieves aggregated statistics for tasks in a workspace.
func (r *Repository) GetTaskStats(
	ctx context.Context,
	workspaceID string,
	start *time.Time,
	limit int,
) ([]*models.TaskStats, error) {
	var startArg any
	if start != nil {
		startArg = start.UTC().Format(time.RFC3339)
	}
	if limit <= 0 {
		limit = 200
	}

	drv := r.ro.DriverName()
	dur := dialect.DurationMs(drv, "turn.completed_at", "turn.started_at")

	query := fmt.Sprintf(`
		SELECT
			t.id, t.title, t.workspace_id, t.workflow_id, t.state,
			COALESCE(session_stats.session_count, 0) as session_count,
			COALESCE(session_stats.turn_count, 0) as turn_count,
			COALESCE(session_stats.message_count, 0) as message_count,
			COALESCE(session_stats.user_message_count, 0) as user_message_count,
			COALESCE(session_stats.tool_call_count, 0) as tool_call_count,
			COALESCE(turn_stats.active_duration_ms, 0) as total_duration_ms,
			COALESCE(turn_stats.active_duration_ms, 0) as active_duration_ms,
			COALESCE(turn_stats.elapsed_span_ms, 0) as elapsed_span_ms,
			t.created_at, session_stats.last_completed_at
		FROM tasks t
		LEFT JOIN (
			SELECT s.task_id,
				COUNT(DISTINCT s.id) as session_count,
				COUNT(DISTINCT turn.id) as turn_count,
				COUNT(DISTINCT msg.id) as message_count,
				COUNT(DISTINCT CASE WHEN msg.author_type = 'user' THEN msg.id END) as user_message_count,
				COUNT(DISTINCT CASE WHEN msg.type LIKE 'tool_%%' THEN msg.id END) as tool_call_count,
				MAX(s.completed_at) as last_completed_at
			FROM task_sessions s
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			LEFT JOIN task_session_messages msg ON msg.task_session_id = s.id
			WHERE (? IS NULL OR s.started_at >= ?)
			GROUP BY s.task_id
		) session_stats ON session_stats.task_id = t.id
		LEFT JOIN (
			SELECT s.task_id,
				SUM(CASE WHEN turn.completed_at IS NOT NULL THEN %s ELSE 0 END) as active_duration_ms,
				%s as elapsed_span_ms
			FROM task_sessions s
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			WHERE (? IS NULL OR s.started_at >= ?)
			GROUP BY s.task_id
		) turn_stats ON turn_stats.task_id = t.id
		WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR t.created_at >= ?)
		ORDER BY t.updated_at DESC
		LIMIT ?
	`, dur, dialect.DurationMs(
		drv,
		"MAX(CASE WHEN turn.completed_at IS NOT NULL THEN turn.completed_at END)",
		"MIN(CASE WHEN turn.completed_at IS NOT NULL THEN turn.started_at END)",
	))

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query),
		startArg, startArg, startArg, startArg,
		workspaceID, startArg, startArg, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTaskStats(rows)
}

func (r *Repository) scanTaskStats(rows *sql.Rows) ([]*models.TaskStats, error) {
	var results []*models.TaskStats
	for rows.Next() {
		var stat models.TaskStats
		var completedAtStr sql.NullString
		var createdAtStr string
		var totalDurationMs float64
		var activeDurationMs float64
		var elapsedSpanMs float64
		err := rows.Scan(
			&stat.TaskID, &stat.TaskTitle, &stat.WorkspaceID, &stat.WorkflowID, &stat.State,
			&stat.SessionCount, &stat.TurnCount, &stat.MessageCount,
			&stat.UserMessageCount, &stat.ToolCallCount, &totalDurationMs,
			&activeDurationMs, &elapsedSpanMs,
			&createdAtStr, &completedAtStr,
		)
		if err != nil {
			return nil, err
		}
		stat.TotalDurationMs = int64(totalDurationMs)
		stat.ActiveDurationMs = int64(activeDurationMs)
		stat.ElapsedSpanMs = int64(elapsedSpanMs)
		stat.CreatedAt = parseTimeString(createdAtStr)
		if completedAtStr.Valid && completedAtStr.String != "" {
			parsedTime := parseTimeString(completedAtStr.String)
			if !parsedTime.IsZero() {
				stat.CompletedAt = &parsedTime
			}
		}
		results = append(results, &stat)
	}
	return results, rows.Err()
}

// Outlier bounds for the "average turn size" metric. Determined empirically
// from prod data: durations under cleanTurnMinDurationMs are no-op/aborted
// turns and durations at or above cleanTurnMaxDurationMs are zombie turns
// whose completed_at was backfilled across an agent restart. Both classes
// badly skew the mean — excluding them drops avg duration from ~1062s to
// ~289s on a ~4k-turn sample. The duration filter is half-open
// [cleanTurnMinDurationMs, cleanTurnMaxDurationMs).
const (
	cleanTurnMinDurationMs = 1000
	cleanTurnMaxDurationMs = 3600000
	cleanTurnMinMessages   = 1
)

// GetGlobalStats retrieves workspace-wide aggregated statistics.
// Implemented as a single query over 5 CTEs (tasks, sessions, turns,
// clean_turn, messages). The four count/sum CTEs each scan their underlying
// table at most once per request; clean_turn additionally issues one
// index-only message-count subquery per qualifying turn (kept correlated to
// stay portable between the SQLite and Postgres dialects).
func (r *Repository) GetGlobalStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GlobalStats, error) {
	var startArg any
	if start != nil {
		startArg = start.UTC().Format(time.RFC3339)
	}

	drv := r.ro.DriverName()
	dur := dialect.DurationMs(drv, "turn.completed_at", "turn.started_at")

	query := fmt.Sprintf(`
		WITH
		task_agg AS (
			SELECT
				COUNT(*) AS total_tasks,
				SUM(CASE WHEN t.archived_at IS NOT NULL
				          OR ws.position = (SELECT MAX(ws2.position) FROM workflow_steps ws2 WHERE ws2.workflow_id = ws.workflow_id)
				         THEN 1 ELSE 0 END) AS completed_tasks,
				SUM(CASE WHEN t.state = 'IN_PROGRESS' AND t.archived_at IS NULL THEN 1 ELSE 0 END) AS in_progress_tasks
			FROM tasks t
			LEFT JOIN workflow_steps ws ON ws.id = t.workflow_step_id
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR t.created_at >= ?)
		),
		session_agg AS (
			SELECT COUNT(*) AS total_sessions
			FROM task_sessions s
			JOIN tasks t ON t.id = s.task_id
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
		),
		turn_agg AS (
			SELECT
				COUNT(*) AS total_turns,
				COALESCE(SUM(CASE WHEN turn.completed_at IS NOT NULL THEN %s ELSE 0 END), 0) AS total_duration_ms
			FROM task_session_turns turn
			JOIN task_sessions s ON s.id = turn.task_session_id
			JOIN tasks t ON t.id = s.task_id
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
		),
		clean_turn_agg AS (
			SELECT
				AVG(dur_ms) AS avg_turn_duration_ms,
				AVG(msg_count) AS avg_messages_per_turn
			FROM (
				SELECT
					%s AS dur_ms,
					(SELECT COUNT(*) FROM task_session_messages m WHERE m.turn_id = turn.id) AS msg_count
				FROM task_session_turns turn
				JOIN task_sessions s ON s.id = turn.task_session_id
				JOIN tasks t ON t.id = s.task_id
				WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
				  AND turn.completed_at IS NOT NULL
			) clean
			WHERE dur_ms >= %d AND dur_ms < %d AND msg_count >= %d
		),
		message_agg AS (
			SELECT
				COUNT(*) AS total_messages,
				SUM(CASE WHEN msg.author_type = 'user' THEN 1 ELSE 0 END) AS total_user_messages,
				SUM(CASE WHEN msg.type LIKE 'tool_%%' THEN 1 ELSE 0 END) AS total_tool_calls
			FROM task_session_messages msg
			JOIN task_sessions s ON s.id = msg.task_session_id
			JOIN tasks t ON t.id = s.task_id
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
		)
		SELECT
			task_agg.total_tasks, task_agg.completed_tasks, task_agg.in_progress_tasks,
			session_agg.total_sessions,
			turn_agg.total_turns,
			message_agg.total_messages, message_agg.total_user_messages, message_agg.total_tool_calls,
			turn_agg.total_duration_ms,
			clean_turn_agg.avg_turn_duration_ms, clean_turn_agg.avg_messages_per_turn
		FROM task_agg, session_agg, turn_agg, clean_turn_agg, message_agg
	`, dur, dur, cleanTurnMinDurationMs, cleanTurnMaxDurationMs, cleanTurnMinMessages)

	var stats models.GlobalStats
	var totalDurationMs float64
	var completedTasks, inProgressTasks sql.NullInt64
	var userMessages, toolCalls sql.NullInt64
	var avgTurnDurationMs, avgMessagesPerTurn sql.NullFloat64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query),
		workspaceID, startArg, startArg, // task_agg
		workspaceID, startArg, startArg, // session_agg
		workspaceID, startArg, startArg, // turn_agg
		workspaceID, startArg, startArg, // clean_turn_agg
		workspaceID, startArg, startArg, // message_agg
	).Scan(
		&stats.TotalTasks, &completedTasks, &inProgressTasks,
		&stats.TotalSessions, &stats.TotalTurns,
		&stats.TotalMessages, &userMessages, &toolCalls,
		&totalDurationMs,
		&avgTurnDurationMs, &avgMessagesPerTurn,
	)
	if err != nil {
		return nil, err
	}
	stats.CompletedTasks = int(completedTasks.Int64)
	stats.InProgressTasks = int(inProgressTasks.Int64)
	stats.TotalUserMessages = int(userMessages.Int64)
	stats.TotalToolCalls = int(toolCalls.Int64)
	stats.TotalDurationMs = int64(totalDurationMs)
	stats.AvgTurnDurationMs = int64(avgTurnDurationMs.Float64)
	stats.AvgMessagesPerTurn = avgMessagesPerTurn.Float64

	if stats.TotalTasks > 0 {
		stats.AvgTurnsPerTask = float64(stats.TotalTurns) / float64(stats.TotalTasks)
		stats.AvgMessagesPerTask = float64(stats.TotalMessages) / float64(stats.TotalTasks)
		stats.AvgDurationMsPerTask = stats.TotalDurationMs / int64(stats.TotalTasks)
	}

	return &stats, nil
}

// GetDailyActivity retrieves daily activity statistics for the last N days
func (r *Repository) GetDailyActivity(ctx context.Context, workspaceID string, days int) ([]*models.DailyActivity, error) {
	drv := r.ro.DriverName()
	dateStart := dialect.DateNowMinusDays(drv, "?")
	datePlus := dialect.DatePlusOneDay(drv, "date")
	curDate := dialect.CurrentDate(drv)
	dateOfTurn := dialect.DateOf(drv, "turn.started_at")
	dateOfMsg := dialect.DateOf(drv, "msg.created_at")

	query := fmt.Sprintf(`
		WITH RECURSIVE dates(date) AS (
			SELECT %s
			UNION ALL
			SELECT %s FROM dates WHERE date < %s
		)
		SELECT
			d.date,
			COALESCE(activity.turn_count, 0) as turn_count,
			COALESCE(activity.message_count, 0) as message_count,
			COALESCE(activity.task_count, 0) as task_count
		FROM dates d
		LEFT JOIN (
			SELECT
				%s as activity_date,
				COUNT(DISTINCT turn.id) as turn_count,
				COUNT(DISTINCT msg.id) as message_count,
				COUNT(DISTINCT t.id) as task_count
			FROM task_session_turns turn
			JOIN task_sessions s ON s.id = turn.task_session_id
			JOIN tasks t ON t.id = s.task_id
			LEFT JOIN task_session_messages msg ON msg.task_session_id = s.id
				AND %s = %s
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0
			GROUP BY %s
		) activity ON activity.activity_date = d.date
		ORDER BY d.date ASC
	`, dateStart, datePlus, curDate, dateOfTurn, dateOfMsg, dateOfTurn, dateOfTurn)

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), days-1, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.DailyActivity
	for rows.Next() {
		var activity models.DailyActivity
		if err := rows.Scan(&activity.Date, &activity.TurnCount, &activity.MessageCount, &activity.TaskCount); err != nil {
			return nil, err
		}
		results = append(results, &activity)
	}

	return results, rows.Err()
}

// GetCompletedTaskActivity retrieves completed task counts for the last N days
func (r *Repository) GetCompletedTaskActivity(ctx context.Context, workspaceID string, days int) ([]*models.CompletedTaskActivity, error) {
	drv := r.ro.DriverName()
	dateStart := dialect.DateNowMinusDays(drv, "?")
	datePlus := dialect.DatePlusOneDay(drv, "date")
	curDate := dialect.CurrentDate(drv)
	dateOfCompleted := dialect.DateOf(drv, "COALESCE(ts.completed_at, t.archived_at)")

	query := fmt.Sprintf(`
		WITH RECURSIVE dates(date) AS (
			SELECT %s
			UNION ALL
			SELECT %s FROM dates WHERE date < %s
		)
		SELECT d.date, COALESCE(activity.completed_tasks, 0) as completed_tasks
		FROM dates d
		LEFT JOIN (
			SELECT %s as activity_date, COUNT(DISTINCT t.id) as completed_tasks
			FROM tasks t
			LEFT JOIN workflow_steps ws ON ws.id = t.workflow_step_id
			LEFT JOIN (
				SELECT task_id, MAX(completed_at) as completed_at
				FROM task_sessions WHERE completed_at IS NOT NULL GROUP BY task_id
			) ts ON ts.task_id = t.id
			WHERE t.workspace_id = ? AND t.is_ephemeral = 0
			  AND (t.archived_at IS NOT NULL
			       OR ws.position = (SELECT MAX(ws2.position) FROM workflow_steps ws2 WHERE ws2.workflow_id = ws.workflow_id))
			  AND COALESCE(ts.completed_at, t.archived_at) IS NOT NULL
			GROUP BY %s
		) activity ON activity.activity_date = d.date
		ORDER BY d.date ASC
	`, dateStart, datePlus, curDate, dateOfCompleted, dateOfCompleted)

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), days-1, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.CompletedTaskActivity
	for rows.Next() {
		var activity models.CompletedTaskActivity
		if err := rows.Scan(&activity.Date, &activity.CompletedTasks); err != nil {
			return nil, err
		}
		results = append(results, &activity)
	}

	return results, rows.Err()
}

// GetRepositoryStats retrieves aggregated statistics for repositories in a workspace
func (r *Repository) GetRepositoryStats(ctx context.Context, workspaceID string, start *time.Time) ([]*models.RepositoryStats, error) {
	var startArg any
	if start != nil {
		startArg = start.UTC().Format(time.RFC3339)
	}

	query := buildRepositoryStatsQuery(r.ro.DriverName())
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query),
		startArg, startArg, startArg, startArg,
		startArg, startArg, startArg, startArg,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.RepositoryStats
	for rows.Next() {
		var stats models.RepositoryStats
		var totalDurationMs float64
		err := rows.Scan(
			&stats.RepositoryID, &stats.RepositoryName,
			&stats.TotalTasks, &stats.CompletedTasks, &stats.InProgressTasks,
			&stats.SessionCount, &stats.TurnCount, &stats.MessageCount,
			&stats.UserMessageCount, &stats.ToolCallCount, &totalDurationMs,
			&stats.TotalCommits, &stats.TotalFilesChanged,
			&stats.TotalInsertions, &stats.TotalDeletions,
		)
		if err != nil {
			return nil, err
		}
		stats.TotalDurationMs = int64(totalDurationMs)
		results = append(results, &stats)
	}

	return results, rows.Err()
}

func buildRepositoryStatsQuery(drv string) string {
	dur := dialect.DurationMs(drv, "turn.completed_at", "turn.started_at")
	return fmt.Sprintf(`
		SELECT
			r.id, r.name,
			COALESCE(task_stats.total_tasks, 0) as total_tasks,
			COALESCE(task_stats.completed_tasks, 0) as completed_tasks,
			COALESCE(task_stats.in_progress_tasks, 0) as in_progress_tasks,
			COALESCE(session_stats.session_count, 0) as session_count,
			COALESCE(session_stats.turn_count, 0) as turn_count,
			COALESCE(session_stats.message_count, 0) as message_count,
			COALESCE(session_stats.user_message_count, 0) as user_message_count,
			COALESCE(session_stats.tool_call_count, 0) as tool_call_count,
			COALESCE(duration_stats.total_duration_ms, 0) as total_duration_ms,
			COALESCE(git_stats.total_commits, 0) as total_commits,
			COALESCE(git_stats.total_files_changed, 0) as total_files_changed,
			COALESCE(git_stats.total_insertions, 0) as total_insertions,
			COALESCE(git_stats.total_deletions, 0) as total_deletions
		FROM repositories r
		LEFT JOIN (
			SELECT tr.repository_id,
				COUNT(DISTINCT t.id) as total_tasks,
				COUNT(DISTINCT CASE WHEN ws.position = (SELECT MAX(ws2.position) FROM workflow_steps ws2 WHERE ws2.workflow_id = ws.workflow_id) THEN t.id END) as completed_tasks,
				COUNT(DISTINCT CASE WHEN t.state = 'IN_PROGRESS' THEN t.id END) as in_progress_tasks
			FROM task_repositories tr
			JOIN tasks t ON t.id = tr.task_id
			LEFT JOIN workflow_steps ws ON ws.id = t.workflow_step_id
			WHERE t.is_ephemeral = 0 AND (? IS NULL OR t.created_at >= ?)
			GROUP BY tr.repository_id
		) task_stats ON task_stats.repository_id = r.id
		LEFT JOIN (
			SELECT tr.repository_id,
				COUNT(DISTINCT s.id) as session_count,
				COUNT(DISTINCT turn.id) as turn_count,
				COUNT(DISTINCT msg.id) as message_count,
				COUNT(DISTINCT CASE WHEN msg.author_type = 'user' THEN msg.id END) as user_message_count,
				COUNT(DISTINCT CASE WHEN msg.type LIKE 'tool_%%' THEN msg.id END) as tool_call_count
			FROM task_repositories tr
			JOIN tasks t ON t.id = tr.task_id
			JOIN task_sessions s ON s.task_id = tr.task_id
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			LEFT JOIN task_session_messages msg ON msg.task_session_id = s.id
			WHERE t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
			GROUP BY tr.repository_id
		) session_stats ON session_stats.repository_id = r.id
		LEFT JOIN (
			SELECT tr.repository_id,
				COALESCE(SUM(CASE WHEN turn.completed_at IS NOT NULL THEN %s ELSE 0 END), 0) as total_duration_ms
			FROM task_repositories tr
			JOIN tasks t ON t.id = tr.task_id
			JOIN task_sessions s ON s.task_id = tr.task_id
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			WHERE t.is_ephemeral = 0 AND (? IS NULL OR s.started_at >= ?)
			GROUP BY tr.repository_id
		) duration_stats ON duration_stats.repository_id = r.id
		LEFT JOIN (
			SELECT s.repository_id,
				COUNT(DISTINCT c.id) as total_commits,
				COALESCE(SUM(c.files_changed), 0) as total_files_changed,
				COALESCE(SUM(c.insertions), 0) as total_insertions,
				COALESCE(SUM(c.deletions), 0) as total_deletions
			FROM task_session_commits c
			JOIN task_sessions s ON s.id = c.session_id
			JOIN tasks t ON t.id = s.task_id
			WHERE t.is_ephemeral = 0 AND s.repository_id != '' AND (? IS NULL OR c.committed_at >= ?)
			GROUP BY s.repository_id
		) git_stats ON git_stats.repository_id = r.id
		WHERE r.workspace_id = ? AND r.deleted_at IS NULL
		ORDER BY total_duration_ms DESC, total_tasks DESC, r.name ASC
	`, dur)
}

// GetAgentUsage retrieves usage statistics per agent profile
func (r *Repository) GetAgentUsage(ctx context.Context, workspaceID string, limit int, start *time.Time) ([]*models.AgentUsage, error) {
	var startArg any
	if start != nil {
		startArg = start.UTC().Format(time.RFC3339)
	}

	drv := r.ro.DriverName()
	dur := dialect.DurationMs(drv, "turn.completed_at", "turn.started_at")
	jeName := dialect.JSONExtract(drv, "s.agent_profile_snapshot", "name")
	jeDisplay := dialect.JSONExtract(drv, "s.agent_profile_snapshot", "agent_display_name")
	jeModel := dialect.JSONExtract(drv, "s.agent_profile_snapshot", "model")
	jeModelName := dialect.JSONExtract(drv, "s.agent_profile_snapshot", "model_name")
	jeLLM := dialect.JSONExtract(drv, "s.agent_profile_snapshot", "llm")

	query := fmt.Sprintf(`
		SELECT
			s.agent_profile_id,
			COALESCE(%s, %s, s.agent_profile_id) as agent_profile_name,
			COALESCE(%s, %s, %s, '') as agent_model,
			COUNT(DISTINCT s.id) as session_count,
			COUNT(DISTINCT turn.id) as turn_count,
			COALESCE(SUM(CASE WHEN turn.completed_at IS NOT NULL THEN %s ELSE 0 END), 0) as total_duration_ms
		FROM task_sessions s
		JOIN tasks t ON t.id = s.task_id
		LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
		WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND s.agent_profile_id != '' AND (? IS NULL OR s.started_at >= ?)
		GROUP BY s.agent_profile_id
		ORDER BY session_count DESC
		LIMIT ?
	`, jeName, jeDisplay, jeModel, jeModelName, jeLLM, dur)

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), workspaceID, startArg, startArg, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.AgentUsage
	for rows.Next() {
		var usage models.AgentUsage
		var totalDurationMs float64
		err := rows.Scan(
			&usage.AgentProfileID, &usage.AgentProfileName, &usage.AgentModel,
			&usage.SessionCount, &usage.TurnCount, &totalDurationMs,
		)
		if err != nil {
			return nil, err
		}
		usage.TotalDurationMs = int64(totalDurationMs)
		results = append(results, &usage)
	}

	return results, rows.Err()
}

// GetGitStats retrieves aggregated git statistics for a workspace
func (r *Repository) GetGitStats(ctx context.Context, workspaceID string, start *time.Time) (*models.GitStats, error) {
	var startArg any
	if start != nil {
		startArg = start.UTC().Format(time.RFC3339)
	}

	query := `
		SELECT
			COUNT(DISTINCT c.id) as total_commits,
			COALESCE(SUM(c.files_changed), 0) as total_files_changed,
			COALESCE(SUM(c.insertions), 0) as total_insertions,
			COALESCE(SUM(c.deletions), 0) as total_deletions
		FROM task_session_commits c
		JOIN task_sessions s ON s.id = c.session_id
		JOIN tasks t ON t.id = s.task_id
		WHERE t.workspace_id = ? AND t.is_ephemeral = 0 AND (? IS NULL OR c.committed_at >= ?)
	`

	var stats models.GitStats
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), workspaceID, startArg, startArg).Scan(
		&stats.TotalCommits, &stats.TotalFilesChanged,
		&stats.TotalInsertions, &stats.TotalDeletions,
	)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// defaultSessionCodeStatsLimit bounds ListSessionCodeStats when the caller
// does not specify one, mirroring GetTaskStats' default page size.
const defaultSessionCodeStatsLimit = 500

// ListSessionCodeStats returns, per session, committed LOC (summed from
// task_session_commits) and PEAK pending-diff LOC (the largest single
// task_session_git_snapshots snapshot, not the latest — the latest snapshot
// is usually a clean tree after a commit, merge, or archive). This is the
// per-session line-of-code aggregation the kandev-plugin-agent-stats plugin
// used to compute by reading the SQLite file directly (see ADR 0042); the
// SQL here mirrors that plugin's sessionsQuery.
//
// Portability: the committed-sum half of this query is plain SQL and works
// unchanged on both drivers. The peak-pending half must walk each snapshot's
// `files` JSON object (keyed by file path, see task/models.GitSnapshot.Files)
// to sum per-file additions/deletions — SQLite does this with json_each,
// Postgres with jsonb_each on the same object; peakPendingSnapshotSubquery
// branches on driver to build the equivalent fragment for each. Both paths
// are covered by SQLite unit tests here; the Postgres path is exercised by
// the ADR 0027-style env-gated Postgres suite (KANDEV_TEST_POSTGRES_DSN) at
// the task/repository layer for the underlying schema, not re-verified here
// against a live Postgres instance — if jsonb_each ever proves not to match
// SQLite's json_each semantics for this shape, guard the Postgres branch
// with a clear error instead of returning silently-wrong pending numbers.
func (r *Repository) ListSessionCodeStats(
	ctx context.Context,
	filter models.SessionCodeStatsFilter,
) ([]*models.SessionCodeStats, error) {
	where, args := buildSessionCodeStatsFilter(filter)
	query := fmt.Sprintf(`
		SELECT
			ts.id AS session_id,
			COALESCE(commit_stats.insertions, 0) AS lines_added_committed,
			COALESCE(commit_stats.deletions, 0) AS lines_deleted_committed,
			COALESCE(peak_stats.peak_additions, 0) AS lines_added_peak_pending,
			COALESCE(peak_stats.peak_deletions, 0) AS lines_deleted_peak_pending
		FROM task_sessions ts
		JOIN tasks t ON t.id = ts.task_id
		LEFT JOIN (
			SELECT c.session_id,
				SUM(c.insertions) AS insertions,
				SUM(c.deletions) AS deletions
			FROM task_session_commits c
			GROUP BY c.session_id
		) commit_stats ON commit_stats.session_id = ts.id
		LEFT JOIN (%s) peak_stats ON peak_stats.session_id = ts.id
		WHERE %s
		ORDER BY ts.started_at ASC, ts.id ASC
		LIMIT ? OFFSET ?
	`, peakPendingSnapshotSubquery(r.ro.DriverName()), where)

	inQuery, inArgs, err := sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}
	inQuery = r.ro.Rebind(inQuery)

	rows, err := r.ro.QueryContext(ctx, inQuery, inArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanSessionCodeStats(rows)
}

// buildSessionCodeStatsFilter turns a SessionCodeStatsFilter into a WHERE
// clause (against the ts/t aliases used by ListSessionCodeStats) and its
// positional args, including the trailing LIMIT/OFFSET values.
func buildSessionCodeStatsFilter(filter models.SessionCodeStatsFilter) (string, []any) {
	var conds []string
	var args []any
	if len(filter.SessionIDs) > 0 {
		conds = append(conds, "ts.id IN (?)")
		args = append(args, filter.SessionIDs)
	}
	if len(filter.TaskIDs) > 0 {
		conds = append(conds, "ts.task_id IN (?)")
		args = append(args, filter.TaskIDs)
	}
	if len(filter.WorkspaceIDs) > 0 {
		conds = append(conds, "t.workspace_id IN (?)")
		args = append(args, filter.WorkspaceIDs)
	}
	if len(filter.States) > 0 {
		conds = append(conds, "ts.state IN (?)")
		args = append(args, filter.States)
	}
	where := "1 = 1"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultSessionCodeStatsLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)

	return where, args
}

// scanSessionCodeStats reads all rows of a ListSessionCodeStats query result.
func scanSessionCodeStats(rows *sql.Rows) ([]*models.SessionCodeStats, error) {
	var results []*models.SessionCodeStats
	for rows.Next() {
		var stat models.SessionCodeStats
		if err := rows.Scan(
			&stat.SessionID,
			&stat.LinesAddedCommitted, &stat.LinesDeletedCommitted,
			&stat.LinesAddedPeakPending, &stat.LinesDeletedPeakPending,
		); err != nil {
			return nil, err
		}
		results = append(results, &stat)
	}
	return results, rows.Err()
}

// peakPendingSnapshotSubquery returns a derived-table SELECT (session_id,
// peak_additions, peak_deletions) that finds, per session, the MAX single
// git-snapshot total across task_session_git_snapshots.files. additions and
// deletions are maximized independently (matching the source plugin), so the
// reported peak-additions and peak-deletions snapshots need not be the same
// snapshot.
func peakPendingSnapshotSubquery(drv string) string {
	if dialect.IsPostgres(drv) {
		return `
			SELECT snap.session_id,
				MAX(snap.additions) AS peak_additions,
				MAX(snap.deletions) AS peak_deletions
			FROM (
				SELECT g.id AS snapshot_id, g.session_id,
					SUM(COALESCE((f.jvalue->>'additions')::numeric, 0)) AS additions,
					SUM(COALESCE((f.jvalue->>'deletions')::numeric, 0)) AS deletions
				FROM task_session_git_snapshots g,
					jsonb_each(g.files::jsonb) AS f(jkey, jvalue)
				GROUP BY g.id, g.session_id
			) snap
			GROUP BY snap.session_id`
	}
	return `
		SELECT snap.session_id,
			MAX(snap.additions) AS peak_additions,
			MAX(snap.deletions) AS peak_deletions
		FROM (
			SELECT g.id AS snapshot_id, g.session_id,
				SUM(COALESCE(json_extract(f.value, '$.additions'), 0)) AS additions,
				SUM(COALESCE(json_extract(f.value, '$.deletions'), 0)) AS deletions
			FROM task_session_git_snapshots g, json_each(g.files) f
			GROUP BY g.id, g.session_id
		) snap
		GROUP BY snap.session_id`
}
