package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

// CreateTaskPlan creates a new task plan.
func (r *Repository) CreateTaskPlan(ctx context.Context, plan *models.TaskPlan) error {
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	plan.CreatedAt = now
	plan.UpdatedAt = now

	if plan.Title == "" {
		plan.Title = "Plan"
	}
	if plan.CreatedBy == "" {
		plan.CreatedBy = "agent"
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), plan.ID, plan.TaskID, plan.Title, plan.Content, plan.CreatedBy, plan.CreatedAt, plan.UpdatedAt)
	return err
}

// GetTaskPlan retrieves a task plan by task ID.
func (r *Repository) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	plan := &models.TaskPlan{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, title, content, created_by, created_at, updated_at
		FROM task_plans WHERE task_id = ?
	`), taskID).Scan(&plan.ID, &plan.TaskID, &plan.Title, &plan.Content, &plan.CreatedBy, &plan.CreatedAt, &plan.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // Return nil, nil when no plan exists
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task plan: %w", err)
	}
	return plan, nil
}

// UpdateTaskPlan updates an existing task plan.
func (r *Repository) UpdateTaskPlan(ctx context.Context, plan *models.TaskPlan) error {
	plan.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plans SET title = ?, content = ?, created_by = ?, updated_at = ?
		WHERE task_id = ?
	`), plan.Title, plan.Content, plan.CreatedBy, plan.UpdatedAt, plan.TaskID)
	if err != nil {
		return fmt.Errorf("failed to update task plan: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan not found for task: %s", plan.TaskID)
	}
	return nil
}

// DeleteTaskPlan deletes a task plan by task ID.
func (r *Repository) DeleteTaskPlan(ctx context.Context, taskID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_plans WHERE task_id = ?`), taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task plan: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan not found for task: %s", taskID)
	}
	return nil
}

