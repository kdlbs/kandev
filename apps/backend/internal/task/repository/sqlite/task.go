package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// CreateTask creates a new task
func (r *Repository) CreateTask(ctx context.Context, task *models.Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkspaceID, task.BoardID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to rollback task insert: %w", rollbackErr)
		}
		return err
	}

	return tx.Commit()
}

// GetTask retrieves a task by ID
func (r *Repository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, board_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.WorkflowStepID, &task.Title, &task.Description, &task.State, &task.Priority, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// UpdateTask updates an existing task
func (r *Repository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET workspace_id = ?, board_id = ?, workflow_step_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.WorkspaceID, task.BoardID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.UpdatedAt, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	return nil
}

// DeleteTask deletes a task by ID
func (r *Repository) DeleteTask(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasks returns all tasks for a board
func (r *Repository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE board_id = ?
		ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// ListTasksByWorkflowStep returns all tasks in a workflow step
func (r *Repository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE workflow_step_id = ? ORDER BY position
	`, workflowStepID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// scanTasks is a helper to scan task rows
func (r *Repository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.BoardID,
			&task.WorkflowStepID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&task.Position,
			&metadata,
			&task.CreatedAt,
			&task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

// UpdateTaskState updates the state of a task
func (r *Repository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	result, err := r.db.ExecContext(ctx, `UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`, state, time.Now().UTC(), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

