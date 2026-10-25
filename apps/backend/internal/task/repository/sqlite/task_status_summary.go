package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/statussummary"
)

// LoadTaskStatusSummaries returns the rows present for taskIDs. Missing rows
// are intentionally omitted so callers can retain their coarse task fallback.
// The input is chunked to keep the query portable across SQLite builds.
func (r *Repository) LoadTaskStatusSummaries(ctx context.Context, taskIDs []string) (map[string]*statussummary.TaskStatusSummary, error) {
	result := make(map[string]*statussummary.TaskStatusSummary, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	for _, chunk := range chunkIDs(taskIDs, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		query := `
			SELECT task_id, revision, summary, updated_at
			FROM task_status_summaries
			WHERE task_id IN (` + placeholders + `)`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("load task status summaries: %w", err)
		}

		for rows.Next() {
			var (
				taskID     string
				revision   int64
				rawSummary string
				updatedAt  time.Time
			)
			if err := rows.Scan(&taskID, &revision, &rawSummary, &updatedAt); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan task status summary: %w", err)
			}
			if revision < 0 {
				_ = rows.Close()
				return nil, fmt.Errorf("task status summary %q has negative revision %d", taskID, revision)
			}

			var summary statussummary.TaskStatusSummary
			if err := json.Unmarshal([]byte(rawSummary), &summary); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("decode task status summary %q: %w", taskID, err)
			}
			summary.Revision = uint64(revision)
			summary.UpdatedAt = updatedAt
			if err := summary.Validate(); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("validate task status summary %q: %w", taskID, err)
			}
			result[taskID] = &summary
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read task status summaries: %w", err)
		}
		_ = rows.Close()
	}
	return result, nil
}

// LoadTaskLastActivity returns the latest bounded activity timestamp for each
// requested task. Every source is filtered by task ID inside its union branch
// so the batch remains scoped and does not scan unrelated task history.
func (r *Repository) LoadTaskLastActivity(ctx context.Context, taskIDs []string) (map[string]time.Time, error) {
	result := make(map[string]time.Time, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}

	for _, chunk := range chunkIDs(taskIDs, sqliteMaxHostParams/6) {
		placeholders, ids := buildInPlaceholders(chunk)
		lifecycleOnlyPredicate := turnLifecycleOnlyPredicate(r.ro.DriverName(), "task_session_turns")
		query := `
			WITH activity AS (
				SELECT id AS task_id, created_at AS activity_at
				FROM tasks
				WHERE id IN (` + placeholders + `)
				UNION ALL
				SELECT id AS task_id, updated_at AS activity_at
				FROM tasks
				WHERE id IN (` + placeholders + `)
				UNION ALL
				SELECT task_id, created_at AS activity_at
				FROM task_session_messages
				WHERE task_id IN (` + placeholders + `)
				  AND author_type = 'user'
				UNION ALL
				SELECT task_id, queued_at AS activity_at
				FROM queued_messages
				WHERE task_id IN (` + placeholders + `)
				  AND queued_by NOT IN ('agent', 'workflow', 'server')
				UNION ALL
				SELECT task_id, started_at AS activity_at
				FROM task_session_turns
				WHERE task_id IN (` + placeholders + `)
				  AND started_at IS NOT NULL
				  AND NOT (` + lifecycleOnlyPredicate + `)
				UNION ALL
				SELECT task_id, completed_at AS activity_at
				FROM task_session_turns
				WHERE task_id IN (` + placeholders + `)
				  AND completed_at IS NOT NULL
				  AND NOT (` + lifecycleOnlyPredicate + `)
			)
			SELECT task_id, MAX(activity_at) AS activity_at
			FROM activity
			GROUP BY task_id`
		args := make([]interface{}, 0, len(ids)*6)
		for range 6 {
			args = append(args, ids...)
		}
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("load task last activity: %w", err)
		}
		for rows.Next() {
			var (
				taskID        string
				rawActivityAt interface{}
			)
			if err := rows.Scan(&taskID, &rawActivityAt); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan task last activity: %w", err)
			}
			activityAt, err := parseTaskActivityTime(rawActivityAt)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("parse task last activity %q: %w", taskID, err)
			}
			result[taskID] = activityAt
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read task last activity: %w", err)
		}
		_ = rows.Close()
	}
	return result, nil
}

func parseTaskActivityTime(value interface{}) (time.Time, error) {
	switch value := value.(type) {
	case time.Time:
		return value, nil
	case string:
		return parseTaskActivityTimeString(value)
	case []byte:
		return parseTaskActivityTimeString(string(value))
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", value)
	}
}

func parseTaskActivityTimeString(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	if normalized := strings.Replace(value, " ", "T", 1); normalized != value {
		if parsed, err := time.Parse(time.RFC3339Nano, normalized); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}

// CompareAndUpdateTaskStatusSummary inserts a new row or replaces an older
// semantic value. The revision and semantic comparison live in the same SQL
// statement, so concurrent source observations cannot overwrite a newer row.
// RowsAffected is false for an older revision or a semantic no-op.
func (r *Repository) CompareAndUpdateTaskStatusSummary(ctx context.Context, stored *statussummary.StoredTaskStatusSummary) (bool, error) {
	if stored == nil {
		return false, fmt.Errorf("task status summary is nil")
	}
	if stored.TaskID == "" {
		return false, fmt.Errorf("task status summary task_id is required")
	}
	if stored.WorkspaceID == "" {
		return false, fmt.Errorf("task status summary workspace_id is required")
	}
	if stored.Summary.Revision == 0 {
		return false, fmt.Errorf("task status summary revision must be positive")
	}

	updatedAt := stored.Summary.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	semanticJSON, err := stored.Summary.SemanticJSON()
	if err != nil {
		return false, fmt.Errorf("serialize task status summary: %w", err)
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_status_summaries (
			task_id, workspace_id, revision, summary, updated_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (task_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			revision = excluded.revision,
			summary = excluded.summary,
			updated_at = excluded.updated_at
		WHERE task_status_summaries.revision < excluded.revision
			AND (
				task_status_summaries.workspace_id <> excluded.workspace_id
				OR task_status_summaries.summary <> excluded.summary
			)
	`), stored.TaskID, stored.WorkspaceID, stored.Summary.Revision, string(semanticJSON), updatedAt)
	if err != nil {
		return false, fmt.Errorf("compare and update task status summary: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) DeleteTaskStatusSummary(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_status_summaries WHERE task_id = ?`), taskID)
	return err
}
