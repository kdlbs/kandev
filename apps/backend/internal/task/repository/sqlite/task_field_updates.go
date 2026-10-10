package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
)

type taskFieldMetadataOmittedKey struct{}

// UpdateTaskFieldsWithParentAdmission applies only supplied fields to the
// current row after workspace, step and task-row serialization.
func (r *Repository) UpdateTaskFieldsWithParentAdmission(ctx context.Context, id string, update models.TaskFieldUpdate, validate hierarchy.TaskParentValidator) (*models.TaskFieldUpdateResult, error) {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := r.lockTaskFieldUpdate(ctx, tx, id, update)
	if err != nil {
		return nil, err
	}
	if update.ParentID != nil {
		if err := validate(ctx, taskHierarchyReader{r, tx}, task, *update.ParentID); err != nil {
			return nil, err
		}
	}
	result := &models.TaskFieldUpdateResult{
		Task: task, PriorState: task.State, PriorWorkflowStepID: task.WorkflowStepID,
		ParentChanged: update.ParentID != nil && *update.ParentID != task.ParentID,
	}
	currentMetadata := task.Metadata
	applyTaskFieldUpdate(task, update)
	preserveHierarchyWorkspace(task, currentMetadata)
	if result.ParentChanged {
		workspace, _ := currentMetadata["workspace"].(map[string]interface{})
		if workspace["mode"] == taskWorkspaceModeInheritParent {
			preserveWorkspaceIdentity(task, workspace, taskWorkspaceModeSharedGroup)
		}
	}
	updateCtx := context.WithValue(ctx, admittedTaskParentKey{}, task.ParentID)
	updateCtx = context.WithValue(updateCtx, taskFieldMetadataOmittedKey{}, update.Metadata == nil)
	preservePosition := update.Position == nil
	entry, marker, err := r.updateTaskTx(updateCtx, tx, task, "", preservePosition, preservePosition, nil)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.dispatchStepEntry(ctx, task.ID, task.WorkflowID, task.WorkflowStepID, entry, marker)
	return result, nil
}

func (r *Repository) lockTaskFieldUpdate(ctx context.Context, tx *sql.Tx, id string, update models.TaskFieldUpdate) (*models.Task, error) {
	ids := []string{id}
	if update.ParentID != nil {
		ids = append(ids, *update.ParentID)
	}
	if err := r.lockTaskHierarchy(ctx, tx, nil, ids); err != nil {
		return nil, err
	}
	destination := &models.Task{ID: id}
	if update.WorkflowStepID != nil {
		destination.WorkflowStepID = *update.WorkflowStepID
	}
	if err := r.lockTaskUpdateSteps(ctx, tx, destination); err != nil {
		return nil, err
	}
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx, r.db.Rebind(query), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	return task, err
}

func applyTaskFieldUpdate(task *models.Task, update models.TaskFieldUpdate) {
	if update.Description != nil {
		task.Description = *update.Description
	}
	if update.Priority != nil {
		task.Priority = *update.Priority
	}
	if update.State != nil {
		task.State = *update.State
	}
	if update.WorkflowStepID != nil {
		task.WorkflowStepID = *update.WorkflowStepID
	}
	if update.Position != nil {
		task.Position = *update.Position
	}
	if update.ParentID != nil {
		task.ParentID = *update.ParentID
	}
	if update.AssigneeUserID != nil {
		task.AssigneeUserID = *update.AssigneeUserID
	}
	if update.Metadata != nil {
		task.Metadata = models.ProtectedTaskMetadataUpdate(task.Metadata, update.Metadata)
	}
	if update.Title != nil {
		task.Title = *update.Title
		delete(task.Metadata, models.MetaKeyAgentTitlePending)
		delete(task.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
	}
}
