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
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
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
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID, &task.Title, &task.Description, &task.State, &task.Priority, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

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
		UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.UpdatedAt, task.ID)
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

// ListTasks returns all tasks for a workflow
func (r *Repository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE workflow_id = ?
		ORDER BY position
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// CountTasksByWorkflow returns the number of tasks in a workflow
func (r *Repository) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workflow_id = ?`, workflowID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountTasksByWorkflowStep returns the number of tasks in a workflow step
func (r *Repository) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ?`, stepID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListTasksByWorkflowStep returns all tasks in a workflow step
func (r *Repository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE workflow_step_id = ? ORDER BY position
	`, workflowStepID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// ListTasksByWorkspace returns paginated tasks for a workspace with total count
// If query is non-empty, filters by task title, description, repository name, or repository path
func (r *Repository) ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int) ([]*models.Task, int, error) {
	// Calculate offset
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	var total int
	var rows *sql.Rows
	var err error

	if query == "" {
		// No search query - use simple query
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, workspaceID).Scan(&total)
		if err != nil {
			return nil, 0, err
		}

		rows, err = r.db.QueryContext(ctx, `
			SELECT id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, created_at, updated_at
			FROM tasks
			WHERE workspace_id = ?
			ORDER BY updated_at DESC
			LIMIT ? OFFSET ?
		`, workspaceID, pageSize, offset)
	} else {
		// Search query - use JOIN with repositories
		searchPattern := "%" + query + "%"

		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT t.id) FROM tasks t
			LEFT JOIN task_repositories tr ON t.id = tr.task_id
			LEFT JOIN repositories r ON tr.repository_id = r.id
			WHERE t.workspace_id = ?
			AND (
				t.title LIKE ? OR
				t.description LIKE ? OR
				r.name LIKE ? OR
				r.local_path LIKE ?
			)
		`, workspaceID, searchPattern, searchPattern, searchPattern, searchPattern).Scan(&total)
		if err != nil {
			return nil, 0, err
		}

		rows, err = r.db.QueryContext(ctx, `
			SELECT DISTINCT t.id, t.workspace_id, t.workflow_id, t.workflow_step_id, t.title, t.description, t.state, t.priority, t.position, t.metadata, t.created_at, t.updated_at
			FROM tasks t
			LEFT JOIN task_repositories tr ON t.id = tr.task_id
			LEFT JOIN repositories r ON tr.repository_id = r.id
			WHERE t.workspace_id = ?
			AND (
				t.title LIKE ? OR
				t.description LIKE ? OR
				r.name LIKE ? OR
				r.local_path LIKE ?
			)
			ORDER BY t.updated_at DESC
			LIMIT ? OFFSET ?
		`, workspaceID, searchPattern, searchPattern, searchPattern, searchPattern, pageSize, offset)
	}

	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	tasks, err := r.scanTasks(rows)
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
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
			&task.WorkflowID,
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

