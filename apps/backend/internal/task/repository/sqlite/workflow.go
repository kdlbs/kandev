package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// AddTaskToWorkflow adds a task to a workflow with placement
func (r *Repository) AddTaskToWorkflow(ctx context.Context, taskID, workflowID, workflowStepID string, position int) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workflow_id = ?, workflow_step_id = ?, position = ?, updated_at = ? WHERE id = ?
	`), workflowID, workflowStepID, position, time.Now().UTC(), taskID)
	return err
}

// RemoveTaskFromWorkflow removes a task from a workflow
func (r *Repository) RemoveTaskFromWorkflow(ctx context.Context, taskID, workflowID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workflow_id = '', workflow_step_id = '', position = 0, updated_at = ? WHERE id = ? AND workflow_id = ?
	`), time.Now().UTC(), taskID, workflowID)
	return err
}

// Workflow operations

// CreateWorkflow creates a new workflow
func (r *Repository) CreateWorkflow(ctx context.Context, workflow *models.Workflow) error {
	if workflow.ID == "" {
		workflow.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	workflow.CreatedAt = now
	workflow.UpdatedAt = now

	// Auto-assign sort_order as max+1 within the workspace.
	// Use writer connection (r.db) to avoid stale reads under concurrent creation.
	var maxOrder int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(
		`SELECT COALESCE(MAX(sort_order), -1) FROM workflows WHERE workspace_id = ?`,
	), workflow.WorkspaceID).Scan(&maxOrder)
	if err != nil {
		return fmt.Errorf("failed to get max sort_order: %w", err)
	}
	workflow.SortOrder = maxOrder + 1

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, description, workflow_template_id, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), workflow.ID, workflow.WorkspaceID, workflow.Name, workflow.Description, workflow.WorkflowTemplateID, workflow.SortOrder, workflow.CreatedAt, workflow.UpdatedAt)

	return err
}

// GetWorkflow retrieves a workflow by ID
func (r *Repository) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	workflow := &models.Workflow{}
	var workflowTemplateID sql.NullString

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, workspace_id, name, description, workflow_template_id, sort_order, created_at, updated_at
		FROM workflows WHERE id = ?
	`), id).Scan(&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description, &workflowTemplateID, &workflow.SortOrder, &workflow.CreatedAt, &workflow.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if workflowTemplateID.Valid {
		workflow.WorkflowTemplateID = &workflowTemplateID.String
	}
	return workflow, nil
}

// UpdateWorkflow updates an existing workflow
func (r *Repository) UpdateWorkflow(ctx context.Context, workflow *models.Workflow) error {
	workflow.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflows SET name = ?, description = ?, workflow_template_id = ?, updated_at = ? WHERE id = ?
	`), workflow.Name, workflow.Description, workflow.WorkflowTemplateID, workflow.UpdatedAt, workflow.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", workflow.ID)
	}
	return nil
}

// DeleteWorkflowsByWorkspace deletes all workflows for a workspace except the excluded IDs (E2E cleanup).
// Relies on CASCADE foreign keys to remove workflow_steps.
func (r *Repository) DeleteWorkflowsByWorkspace(ctx context.Context, workspaceID string, excludeIDs []string) (int64, error) {
	if len(excludeIDs) == 0 {
		result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workflows WHERE workspace_id = ?`), workspaceID)
		if err != nil {
			return 0, err
		}
		rows, _ := result.RowsAffected()
		return rows, nil
	}

	query, args, err := sqlx.In(`DELETE FROM workflows WHERE workspace_id = ? AND id NOT IN (?)`, workspaceID, excludeIDs)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

// DeleteWorkflow deletes a workflow by ID
func (r *Repository) DeleteWorkflow(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workflows WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// ListWorkflows returns all workflows
func (r *Repository) ListWorkflows(ctx context.Context, workspaceID string) ([]*models.Workflow, error) {
	query := `
		SELECT id, workspace_id, name, description, workflow_template_id, sort_order, created_at, updated_at FROM workflows
	`
	var args []interface{}
	if workspaceID != "" {
		query += " WHERE workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY sort_order ASC, created_at ASC"

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Workflow
	for rows.Next() {
		workflow := &models.Workflow{}
		var workflowTemplateID sql.NullString
		err := rows.Scan(&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description, &workflowTemplateID, &workflow.SortOrder, &workflow.CreatedAt, &workflow.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if workflowTemplateID.Valid {
			workflow.WorkflowTemplateID = &workflowTemplateID.String
		}
		result = append(result, workflow)
	}
	return result, rows.Err()
}

// ReorderWorkflows updates sort_order for workflows within a workspace using a transaction.
func (r *Repository) ReorderWorkflows(ctx context.Context, workspaceID string, workflowIDs []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for i, id := range workflowIDs {
		result, err := tx.ExecContext(ctx, r.db.Rebind(
			`UPDATE workflows SET sort_order = ?, updated_at = ? WHERE id = ? AND workspace_id = ?`,
		), i, now, id, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to update sort_order for workflow %s: %w", id, err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("workflow not found in workspace: %s", id)
		}
	}
	return tx.Commit()
}
