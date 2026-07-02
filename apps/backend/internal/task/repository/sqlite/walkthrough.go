package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

// walkthroughAuthorAgent matches the task_walkthroughs.created_by column
// DEFAULT — walkthroughs are always agent-authored.
const walkthroughAuthorAgent = "agent"

// CreateTaskWalkthrough upserts a walkthrough for a task (one row per task,
// keyed by task_id). Steps are serialized to a JSON column.
func (r *Repository) CreateTaskWalkthrough(ctx context.Context, wt *models.TaskWalkthrough) error {
	if wt.ID == "" {
		wt.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if wt.CreatedAt.IsZero() {
		wt.CreatedAt = now
	}
	wt.UpdatedAt = now
	if wt.Title == "" {
		wt.Title = "Walkthrough"
	}
	if wt.CreatedBy == "" {
		wt.CreatedBy = walkthroughAuthorAgent
	}

	stepsJSON, err := marshalSteps(wt.Steps)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_walkthroughs (id, task_id, title, steps, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			title = excluded.title,
			steps = excluded.steps,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at
	`), wt.ID, wt.TaskID, wt.Title, stepsJSON, wt.CreatedBy, wt.CreatedAt, wt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert task walkthrough: %w", err)
	}
	return nil
}

// GetTaskWalkthrough retrieves a walkthrough by task ID. Returns nil, nil when
// none exists.
func (r *Repository) GetTaskWalkthrough(ctx context.Context, taskID string) (*models.TaskWalkthrough, error) {
	wt := &models.TaskWalkthrough{}
	var stepsJSON string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, title, steps, created_by, created_at, updated_at
		FROM task_walkthroughs WHERE task_id = ?
	`), taskID).Scan(&wt.ID, &wt.TaskID, &wt.Title, &stepsJSON, &wt.CreatedBy, &wt.CreatedAt, &wt.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task walkthrough: %w", err)
	}
	steps, err := unmarshalSteps(stepsJSON)
	if err != nil {
		return nil, err
	}
	wt.Steps = steps
	return wt, nil
}

// DeleteTaskWalkthrough deletes a walkthrough by task ID.
func (r *Repository) DeleteTaskWalkthrough(ctx context.Context, taskID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_walkthroughs WHERE task_id = ?`), taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task walkthrough: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task walkthrough not found for task: %s", taskID)
	}
	return nil
}

// marshalSteps serializes steps to a JSON array, never returning the "null"
// literal for an empty/nil slice (the column DEFAULT is '[]').
func marshalSteps(steps []models.WalkthroughStep) (string, error) {
	if len(steps) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return "", fmt.Errorf("failed to marshal walkthrough steps: %w", err)
	}
	return string(b), nil
}

func unmarshalSteps(raw string) ([]models.WalkthroughStep, error) {
	if raw == "" {
		return nil, nil
	}
	var steps []models.WalkthroughStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal walkthrough steps: %w", err)
	}
	return steps, nil
}
