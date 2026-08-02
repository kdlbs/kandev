package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
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
