package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// ensureUnrecordedPluginExecutorsSchema creates the handoff table for plugin
// resources that no inventory row could hold. It deliberately has no foreign
// key: the record must outlive the environment row it names until the resource
// is confirmed destroyed.
func (r *Repository) ensureUnrecordedPluginExecutorsSchema() error {
	if err := r.migrate.Apply("unrecorded_plugin_executors.table", `
		CREATE TABLE IF NOT EXISTS unrecorded_plugin_executors (
			execution_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_environment_id TEXT NOT NULL,
			running TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`); err != nil {
		return fmt.Errorf("create unrecorded plugin executor table: %w", err)
	}
	if err := r.migrate.Apply("unrecorded_plugin_executors.environment_index", `
		CREATE INDEX IF NOT EXISTS idx_unrecorded_plugin_executors_environment
			ON unrecorded_plugin_executors(task_id, task_environment_id)`); err != nil {
		return fmt.Errorf("create unrecorded plugin executor index: %w", err)
	}
	return r.migrate.Err()
}

// RecordUnrecordedPluginExecutor stores the handoff for one execution. It is
// idempotent per execution and does not depend on the environment claim.
func (r *Repository) RecordUnrecordedPluginExecutor(ctx context.Context, unrecorded *models.UnrecordedPluginExecutor) error {
	if unrecorded == nil || unrecorded.Running == nil || unrecorded.ExecutionID == "" || unrecorded.SessionID == "" ||
		unrecorded.TaskID == "" || unrecorded.EnvironmentID == "" {
		return errors.New("unrecorded plugin executor identity is incomplete")
	}
	encoded, err := json.Marshal(unrecorded.Running)
	if err != nil {
		return fmt.Errorf("encode unrecorded plugin executor: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO unrecorded_plugin_executors (execution_id, session_id, task_id, task_environment_id, running, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(execution_id) DO UPDATE SET running = excluded.running
	`), unrecorded.ExecutionID, unrecorded.SessionID, unrecorded.TaskID, unrecorded.EnvironmentID, string(encoded), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("record unrecorded plugin executor: %w", err)
	}
	return nil
}

// ListUnrecordedPluginExecutors returns the handoffs of one environment.
func (r *Repository) ListUnrecordedPluginExecutors(ctx context.Context, taskID, environmentID string) ([]*models.UnrecordedPluginExecutor, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`
		SELECT execution_id, session_id, task_id, task_environment_id, running, created_at
		FROM unrecorded_plugin_executors WHERE task_id = ? AND task_environment_id = ? ORDER BY created_at, execution_id
	`), taskID, environmentID)
	if err != nil {
		return nil, fmt.Errorf("list unrecorded plugin executors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var listed []*models.UnrecordedPluginExecutor
	for rows.Next() {
		item := &models.UnrecordedPluginExecutor{}
		var encoded string
		if err := rows.Scan(&item.ExecutionID, &item.SessionID, &item.TaskID, &item.EnvironmentID, &encoded, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan unrecorded plugin executor: %w", err)
		}
		if err := json.Unmarshal([]byte(encoded), &item.Running); err != nil {
			return nil, fmt.Errorf("decode unrecorded plugin executor %s: %w", item.ExecutionID, err)
		}
		listed = append(listed, item)
	}
	return listed, rows.Err()
}

// DeleteUnrecordedPluginExecutor removes a handoff once its resource is
// confirmed destroyed. Deleting an absent record succeeds.
func (r *Repository) DeleteUnrecordedPluginExecutor(ctx context.Context, executionID string) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM unrecorded_plugin_executors WHERE execution_id = ?`), executionID); err != nil {
		return fmt.Errorf("delete unrecorded plugin executor: %w", err)
	}
	return nil
}
