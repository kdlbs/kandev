package sqlite

import (
	"context"
	"database/sql"

	"github.com/kandev/kandev/internal/task/models"
)

// GetTaskStats retrieves aggregated statistics for all tasks in a workspace
func (r *Repository) GetTaskStats(ctx context.Context, workspaceID string) ([]*models.TaskStats, error) {
	// Use separate subqueries for counts and duration to avoid row multiplication
	query := `
		SELECT
			t.id,
			t.title,
			t.workspace_id,
			t.board_id,
			t.state,
			COALESCE(session_stats.session_count, 0) as session_count,
			COALESCE(session_stats.turn_count, 0) as turn_count,
			COALESCE(session_stats.message_count, 0) as message_count,
			COALESCE(session_stats.user_message_count, 0) as user_message_count,
			COALESCE(session_stats.tool_call_count, 0) as tool_call_count,
			COALESCE(turn_stats.total_duration_ms, 0) as total_duration_ms,
			t.created_at,
			session_stats.last_completed_at
		FROM tasks t
		LEFT JOIN (
			SELECT
				s.task_id,
				COUNT(DISTINCT s.id) as session_count,
				COUNT(DISTINCT turn.id) as turn_count,
				COUNT(DISTINCT msg.id) as message_count,
				COUNT(DISTINCT CASE WHEN msg.author_type = 'user' THEN msg.id END) as user_message_count,
				COUNT(DISTINCT CASE WHEN msg.type LIKE 'tool_%' THEN msg.id END) as tool_call_count,
				MAX(s.completed_at) as last_completed_at
			FROM task_sessions s
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			LEFT JOIN task_session_messages msg ON msg.task_session_id = s.id
			GROUP BY s.task_id
		) session_stats ON session_stats.task_id = t.id
		LEFT JOIN (
			SELECT
				s.task_id,
				SUM(CASE
					WHEN turn.completed_at IS NOT NULL
					THEN (julianday(turn.completed_at) - julianday(turn.started_at)) * 86400000
					ELSE 0
				END) as total_duration_ms
			FROM task_sessions s
			LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
			GROUP BY s.task_id
		) turn_stats ON turn_stats.task_id = t.id
		WHERE t.workspace_id = ?
		ORDER BY t.updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.TaskStats
	for rows.Next() {
		var stat models.TaskStats
		var completedAt sql.NullTime
		var totalDurationMs float64 // SQLite returns float from julianday math
		err := rows.Scan(
			&stat.TaskID,
			&stat.TaskTitle,
			&stat.WorkspaceID,
			&stat.BoardID,
			&stat.State,
			&stat.SessionCount,
			&stat.TurnCount,
			&stat.MessageCount,
			&stat.UserMessageCount,
			&stat.ToolCallCount,
			&totalDurationMs,
			&stat.CreatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, err
		}
		stat.TotalDurationMs = int64(totalDurationMs)
		if completedAt.Valid {
			stat.CompletedAt = &completedAt.Time
		}
		results = append(results, &stat)
	}

	return results, rows.Err()
}

// GetGlobalStats retrieves workspace-wide aggregated statistics
func (r *Repository) GetGlobalStats(ctx context.Context, workspaceID string) (*models.GlobalStats, error) {
	// Use separate subqueries to avoid row multiplication from JOINs
	// Count tasks in "done" workflow step (step_type = 'done') as completed
	query := `
		SELECT
			(SELECT COUNT(*) FROM tasks WHERE workspace_id = ?) as total_tasks,
			(SELECT COUNT(*) FROM tasks t
			 JOIN workflow_steps ws ON ws.id = t.workflow_step_id
			 WHERE t.workspace_id = ? AND ws.step_type = 'done') as completed_tasks,
			(SELECT COUNT(*) FROM tasks WHERE workspace_id = ? AND state = 'IN_PROGRESS') as in_progress_tasks,
			(SELECT COUNT(*) FROM task_sessions s JOIN tasks t ON t.id = s.task_id WHERE t.workspace_id = ?) as total_sessions,
			(SELECT COUNT(*) FROM task_session_turns turn
			 JOIN task_sessions s ON s.id = turn.task_session_id
			 JOIN tasks t ON t.id = s.task_id
			 WHERE t.workspace_id = ?) as total_turns,
			(SELECT COUNT(*) FROM task_session_messages msg
			 JOIN task_sessions s ON s.id = msg.task_session_id
			 JOIN tasks t ON t.id = s.task_id
			 WHERE t.workspace_id = ?) as total_messages,
			(SELECT COUNT(*) FROM task_session_messages msg
			 JOIN task_sessions s ON s.id = msg.task_session_id
			 JOIN tasks t ON t.id = s.task_id
			 WHERE t.workspace_id = ? AND msg.author_type = 'user') as total_user_messages,
			(SELECT COUNT(*) FROM task_session_messages msg
			 JOIN task_sessions s ON s.id = msg.task_session_id
			 JOIN tasks t ON t.id = s.task_id
			 WHERE t.workspace_id = ? AND msg.type LIKE 'tool_%') as total_tool_calls,
			(SELECT COALESCE(SUM(
				CASE WHEN turn.completed_at IS NOT NULL
				THEN (julianday(turn.completed_at) - julianday(turn.started_at)) * 86400000
				ELSE 0 END
			), 0) FROM task_session_turns turn
			 JOIN task_sessions s ON s.id = turn.task_session_id
			 JOIN tasks t ON t.id = s.task_id
			 WHERE t.workspace_id = ?) as total_duration_ms
	`

	var stats models.GlobalStats
	var totalDurationMs float64 // SQLite returns float from julianday math
	err := r.db.QueryRowContext(ctx, query,
		workspaceID, workspaceID, workspaceID, workspaceID,
		workspaceID, workspaceID, workspaceID, workspaceID, workspaceID,
	).Scan(
		&stats.TotalTasks,
		&stats.CompletedTasks,
		&stats.InProgressTasks,
		&stats.TotalSessions,
		&stats.TotalTurns,
		&stats.TotalMessages,
		&stats.TotalUserMessages,
		&stats.TotalToolCalls,
		&totalDurationMs,
	)
	if err != nil {
		return nil, err
	}
	stats.TotalDurationMs = int64(totalDurationMs)

	// Calculate averages
	if stats.TotalTasks > 0 {
		stats.AvgTurnsPerTask = float64(stats.TotalTurns) / float64(stats.TotalTasks)
		stats.AvgMessagesPerTask = float64(stats.TotalMessages) / float64(stats.TotalTasks)
		stats.AvgDurationMsPerTask = stats.TotalDurationMs / int64(stats.TotalTasks)
	}

	return &stats, nil
}

// GetDailyActivity retrieves daily activity statistics for the last N days
func (r *Repository) GetDailyActivity(ctx context.Context, workspaceID string, days int) ([]*models.DailyActivity, error) {
	query := `
		WITH RECURSIVE dates(date) AS (
			SELECT date('now', '-' || ? || ' days')
			UNION ALL
			SELECT date(date, '+1 day')
			FROM dates
			WHERE date < date('now')
		)
		SELECT
			d.date,
			COALESCE(activity.turn_count, 0) as turn_count,
			COALESCE(activity.message_count, 0) as message_count,
			COALESCE(activity.task_count, 0) as task_count
		FROM dates d
		LEFT JOIN (
			SELECT
				date(turn.started_at) as activity_date,
				COUNT(DISTINCT turn.id) as turn_count,
				COUNT(DISTINCT msg.id) as message_count,
				COUNT(DISTINCT t.id) as task_count
			FROM task_session_turns turn
			JOIN task_sessions s ON s.id = turn.task_session_id
			JOIN tasks t ON t.id = s.task_id
			LEFT JOIN task_session_messages msg ON msg.task_session_id = s.id
				AND date(msg.created_at) = date(turn.started_at)
			WHERE t.workspace_id = ?
			GROUP BY date(turn.started_at)
		) activity ON activity.activity_date = d.date
		ORDER BY d.date ASC
	`

	rows, err := r.db.QueryContext(ctx, query, days-1, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.DailyActivity
	for rows.Next() {
		var activity models.DailyActivity
		err := rows.Scan(
			&activity.Date,
			&activity.TurnCount,
			&activity.MessageCount,
			&activity.TaskCount,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, &activity)
	}

	return results, rows.Err()
}

// GetAgentUsage retrieves usage statistics per agent profile
func (r *Repository) GetAgentUsage(ctx context.Context, workspaceID string, limit int) ([]*models.AgentUsage, error) {
	query := `
		SELECT
			s.agent_profile_id,
			COALESCE(
				json_extract(s.agent_profile_snapshot, '$.name'),
				json_extract(s.agent_profile_snapshot, '$.agent_display_name'),
				s.agent_profile_id
			) as agent_profile_name,
			COUNT(DISTINCT s.id) as session_count,
			COUNT(DISTINCT turn.id) as turn_count,
			COALESCE(SUM(CASE
				WHEN turn.completed_at IS NOT NULL
				THEN (julianday(turn.completed_at) - julianday(turn.started_at)) * 86400000
				ELSE 0
			END), 0) as total_duration_ms
		FROM task_sessions s
		JOIN tasks t ON t.id = s.task_id
		LEFT JOIN task_session_turns turn ON turn.task_session_id = s.id
		WHERE t.workspace_id = ? AND s.agent_profile_id != ''
		GROUP BY s.agent_profile_id
		ORDER BY session_count DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.AgentUsage
	for rows.Next() {
		var usage models.AgentUsage
		var totalDurationMs float64
		err := rows.Scan(
			&usage.AgentProfileID,
			&usage.AgentProfileName,
			&usage.SessionCount,
			&usage.TurnCount,
			&totalDurationMs,
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
func (r *Repository) GetGitStats(ctx context.Context, workspaceID string) (*models.GitStats, error) {
	query := `
		SELECT
			COUNT(DISTINCT c.id) as total_commits,
			COALESCE(SUM(c.files_changed), 0) as total_files_changed,
			COALESCE(SUM(c.insertions), 0) as total_insertions,
			COALESCE(SUM(c.deletions), 0) as total_deletions
		FROM task_session_commits c
		JOIN task_sessions s ON s.id = c.session_id
		JOIN tasks t ON t.id = s.task_id
		WHERE t.workspace_id = ?
	`

	var stats models.GitStats
	err := r.db.QueryRowContext(ctx, query, workspaceID).Scan(
		&stats.TotalCommits,
		&stats.TotalFilesChanged,
		&stats.TotalInsertions,
		&stats.TotalDeletions,
	)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}
