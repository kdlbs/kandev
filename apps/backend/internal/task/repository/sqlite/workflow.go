package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// AddTaskToWorkflow adds a task to a workflow with placement
func (r *Repository) AddTaskToWorkflow(ctx context.Context, taskID, workflowID, workflowStepID string, position int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET workflow_id = ?, workflow_step_id = ?, position = ?, updated_at = ? WHERE id = ?
	`, workflowID, workflowStepID, position, time.Now().UTC(), taskID)
	return err
}

// RemoveTaskFromWorkflow removes a task from a workflow
func (r *Repository) RemoveTaskFromWorkflow(ctx context.Context, taskID, workflowID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET workflow_id = '', workflow_step_id = '', position = 0, updated_at = ? WHERE id = ? AND workflow_id = ?
	`, time.Now().UTC(), taskID, workflowID)
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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, description, workflow_template_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, workflow.ID, workflow.WorkspaceID, workflow.Name, workflow.Description, workflow.WorkflowTemplateID, workflow.CreatedAt, workflow.UpdatedAt)

	return err
}

// GetWorkflow retrieves a workflow by ID
func (r *Repository) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	workflow := &models.Workflow{}
	var workflowTemplateID sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, description, workflow_template_id, created_at, updated_at
		FROM workflows WHERE id = ?
	`, id).Scan(&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description, &workflowTemplateID, &workflow.CreatedAt, &workflow.UpdatedAt)

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

	result, err := r.db.ExecContext(ctx, `
		UPDATE workflows SET name = ?, description = ?, workflow_template_id = ?, updated_at = ? WHERE id = ?
	`, workflow.Name, workflow.Description, workflow.WorkflowTemplateID, workflow.UpdatedAt, workflow.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow not found: %s", workflow.ID)
	}
	return nil
}

// DeleteWorkflow deletes a workflow by ID
func (r *Repository) DeleteWorkflow(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id)
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
		SELECT id, workspace_id, name, description, workflow_template_id, created_at, updated_at FROM workflows
	`
	var args []interface{}
	if workspaceID != "" {
		query += " WHERE workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Workflow
	for rows.Next() {
		workflow := &models.Workflow{}
		var workflowTemplateID sql.NullString
		err := rows.Scan(&workflow.ID, &workflow.WorkspaceID, &workflow.Name, &workflow.Description, &workflowTemplateID, &workflow.CreatedAt, &workflow.UpdatedAt)
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
