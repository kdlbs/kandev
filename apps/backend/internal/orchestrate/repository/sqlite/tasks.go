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
