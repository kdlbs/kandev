package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// TaskExecutionFields holds the execution-related fields from a task row.
type TaskExecutionFields struct {
	ID                      string `db:"id"`
	ExecutionPolicy         string `db:"execution_policy"`
	ExecutionState          string `db:"execution_state"`
	AssigneeAgentInstanceID string `db:"assignee_agent_instance_id"`
	State                   string `db:"state"`
	WorkspaceID             string `db:"workspace_id"`
}

// GetTaskExecutionFields returns the execution-related fields for a task.
func (r *Repository) GetTaskExecutionFields(ctx context.Context, taskID string) (*TaskExecutionFields, error) {
	var fields TaskExecutionFields
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT id, COALESCE(execution_policy, '') as execution_policy,
		       COALESCE(execution_state, '') as execution_state,
		       COALESCE(assignee_agent_instance_id, '') as assignee_agent_instance_id,
		       COALESCE(state, '') as state,
		       COALESCE(workspace_id, '') as workspace_id
		FROM tasks WHERE id = ?
	`), taskID).StructScan(&fields)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, err
	}
	return &fields, nil
}

// UpdateTaskExecutionState updates only the execution_state column on a task.
func (r *Repository) UpdateTaskExecutionState(ctx context.Context, taskID, executionState string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET execution_state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), executionState, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// UpdateTaskState updates the state column on a task (e.g. "IN_PROGRESS", "COMPLETED").
func (r *Repository) UpdateTaskState(ctx context.Context, taskID, state string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), state, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// UpdateTaskExecutionPolicy updates only the execution_policy column on a task.
func (r *Repository) UpdateTaskExecutionPolicy(ctx context.Context, taskID, executionPolicy string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET execution_policy = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), executionPolicy, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// UpdateTaskAssignee updates the assignee_agent_instance_id column on a task.
func (r *Repository) UpdateTaskAssignee(ctx context.Context, taskID, assigneeID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET assignee_agent_instance_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), assigneeID, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// TaskBasicInfo contains the minimal task fields needed for prompt building.
type TaskBasicInfo struct {
	Title       string `db:"title"`
	Description string `db:"description"`
	Identifier  string `db:"identifier"`
	Priority    int    `db:"priority"`
	ProjectID   string `db:"project_id"`
}

// GetTaskBasicInfo returns minimal task data for prompt context.
// Returns nil, nil if the task is not found.
func (r *Repository) GetTaskBasicInfo(ctx context.Context, taskID string) (*TaskBasicInfo, error) {
	var info TaskBasicInfo
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COALESCE(title, '') AS title,
			COALESCE(description, '') AS description,
			COALESCE(identifier, '') AS identifier,
			COALESCE(priority, 0) AS priority,
			COALESCE(project_id, '') AS project_id
		FROM tasks WHERE id = ?
	`), taskID).StructScan(&info)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// TaskSearchResult contains fields returned by a task search query.
type TaskSearchResult struct {
	ID                      string `db:"id"`
	WorkspaceID             string `db:"workspace_id"`
	Identifier              string `db:"identifier"`
	Title                   string `db:"title"`
	Description             string `db:"description"`
	Status                  string `db:"status"`
	Priority                int    `db:"priority"`
	ParentID                string `db:"parent_id"`
	ProjectID               string `db:"project_id"`
	AssigneeAgentInstanceID string `db:"assignee_agent_instance_id"`
	Labels                  string `db:"labels"`
	CreatedAt               string `db:"created_at"`
	UpdatedAt               string `db:"updated_at"`
}

// SearchTasks performs a LIKE-based search on title, description, and identifier
// for non-archived, non-ephemeral tasks in the given workspace.
func (r *Repository) SearchTasks(ctx context.Context, workspaceID, query string, limit int) ([]*TaskSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := "%" + query + "%"
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id,
		       COALESCE(workspace_id, '') AS workspace_id,
		       COALESCE(identifier, '') AS identifier,
		       COALESCE(title, '') AS title,
		       COALESCE(description, '') AS description,
		       COALESCE(state, '') AS status,
		       COALESCE(priority, 0) AS priority,
		       COALESCE(parent_id, '') AS parent_id,
		       COALESCE(project_id, '') AS project_id,
		       COALESCE(assignee_agent_instance_id, '') AS assignee_agent_instance_id,
		       COALESCE(labels, '[]') AS labels,
		       created_at,
		       updated_at
		FROM tasks
		WHERE workspace_id = ?
		  AND archived_at IS NULL
		  AND is_ephemeral = 0
		  AND (title LIKE ? OR description LIKE ? OR identifier LIKE ?)
		ORDER BY updated_at DESC
		LIMIT ?
	`), workspaceID, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*TaskSearchResult
	for rows.Next() {
		var r TaskSearchResult
		if err := rows.StructScan(&r); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, &r)
	}
	return results, rows.Err()
}

// CheckoutTask atomically acquires an exclusive lock on a task for an agent.
// Returns true if the lock was acquired, false if another agent holds it.
func (r *Repository) CheckoutTask(ctx context.Context, taskID, agentID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET checkout_agent_id = ?, checkout_at = datetime('now')
		WHERE id = ? AND (checkout_agent_id IS NULL OR checkout_agent_id = '' OR checkout_agent_id = ?)
	`), agentID, taskID, agentID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ReleaseTaskCheckout releases the exclusive lock on a task.
func (r *Repository) ReleaseTaskCheckout(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET checkout_agent_id = NULL, checkout_at = NULL WHERE id = ?
	`), taskID)
	return err
}
