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

// Revision history

// InsertTaskPlanRevision inserts a new revision row.
func (r *Repository) InsertTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error {
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = now
	}
	rev.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plan_revisions
			(id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		rev.ID, rev.TaskID, rev.RevisionNumber, rev.Title, rev.Content,
		rev.AuthorKind, rev.AuthorName, rev.RevertOfRevisionID, rev.CreatedAt, rev.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert task plan revision: %w", err)
	}
	return nil
}

// UpdateTaskPlanRevision updates title/content/updated_at on an existing revision (coalesce merge).
func (r *Repository) UpdateTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error {
	rev.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plan_revisions
		SET title = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), rev.Title, rev.Content, rev.UpdatedAt, rev.ID)
	if err != nil {
		return fmt.Errorf("failed to update task plan revision: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan revision not found: %s", rev.ID)
	}
	return nil
}

// GetTaskPlanRevision fetches a single revision by ID.
func (r *Repository) GetTaskPlanRevision(ctx context.Context, id string) (*models.TaskPlanRevision, error) {
	return r.scanRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at
		FROM task_plan_revisions WHERE id = ?
	`), id))
}

// GetLatestTaskPlanRevision returns the newest revision for a task (by revision_number DESC).
func (r *Repository) GetLatestTaskPlanRevision(ctx context.Context, taskID string) (*models.TaskPlanRevision, error) {
	return r.scanRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at
		FROM task_plan_revisions
		WHERE task_id = ?
		ORDER BY revision_number DESC
		LIMIT 1
	`), taskID))
}

// ListTaskPlanRevisions returns revisions newest-first. limit <= 0 returns all.
func (r *Repository) ListTaskPlanRevisions(ctx context.Context, taskID string, limit int) ([]*models.TaskPlanRevision, error) {
	query := `
		SELECT id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at
		FROM task_plan_revisions
		WHERE task_id = ?
		ORDER BY revision_number DESC`
	args := []interface{}{taskID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list task plan revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskPlanRevision
	for rows.Next() {
		rev := &models.TaskPlanRevision{}
		var revertOf sql.NullString
		if err := rows.Scan(
			&rev.ID, &rev.TaskID, &rev.RevisionNumber, &rev.Title, &rev.Content,
			&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan task plan revision: %w", err)
		}
		if revertOf.Valid {
			v := revertOf.String
			rev.RevertOfRevisionID = &v
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task plan revisions: %w", err)
	}
	return out, nil
}

// NextTaskPlanRevisionNumber returns max(revision_number)+1 for a task, or 1 if none exist.
func (r *Repository) NextTaskPlanRevisionNumber(ctx context.Context, taskID string) (int, error) {
	var max sql.NullInt64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT MAX(revision_number) FROM task_plan_revisions WHERE task_id = ?
	`), taskID).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("failed to get next revision number: %w", err)
	}
	if !max.Valid {
		return 1, nil
	}
	return int(max.Int64) + 1, nil
}

func (r *Repository) scanRevisionRow(row *sql.Row) (*models.TaskPlanRevision, error) {
	rev := &models.TaskPlanRevision{}
	var revertOf sql.NullString
	err := row.Scan(
		&rev.ID, &rev.TaskID, &rev.RevisionNumber, &rev.Title, &rev.Content,
		&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task plan revision: %w", err)
	}
	if revertOf.Valid {
		v := revertOf.String
		rev.RevertOfRevisionID = &v
	}
	return rev, nil
}
