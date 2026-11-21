package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"time"
)

func (r *Repository) writeSourceWorkflow(ctx context.Context, tx *sqlx.Tx, plan *models.PromptSourcePlan, now time.Time) error {
	workflow := plan.Workflow
	if plan.ExpectedWorkflow == nil {
		workflow.CreatedByUserID = taskmodels.ExecutorSourceCreator(ctx)
		_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workflows (created_by_user_id, id, workspace_id, name, description, prompt, agent_profile_id, workflow_template_id, sort_order, hidden, style, source, source_path, created_at, updated_at) SELECT ?, ?, ?, ?, ?, ?, '', '', COALESCE(MAX(sort_order),-1)+1, 0, ?, ?, ?, ?, ? FROM workflows WHERE workspace_id = ?`), workflow.CreatedByUserID, workflow.ID, workflow.WorkspaceID, workflow.Name, workflow.Description, workflow.Prompt, workflow.Style, workflow.Source, workflow.SourcePath, now, now, workflow.WorkspaceID)
		if err == nil {
			workflow.CreatedAt, workflow.UpdatedAt = now, now
		}
		return err
	}
	old := plan.ExpectedWorkflow
	if old.Name == workflow.Name && old.Description == workflow.Description && old.Prompt == workflow.Prompt && old.Source == workflow.Source && old.SourcePath == workflow.SourcePath {
		return nil
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workflows SET name = ?, description = ?, prompt = ?, source = ?, source_path = ?, updated_at = ? WHERE id = ?`), workflow.Name, workflow.Description, workflow.Prompt, workflow.Source, workflow.SourcePath, now, workflow.ID)
	if err == nil {
		workflow.UpdatedAt = now
	}
	return err
}
func (r *Repository) writeSourceSteps(ctx context.Context, tx *sqlx.Tx, plan models.PromptSourcePlan, now time.Time) error {
	existing := make(map[string]*models.WorkflowStep)
	for _, step := range plan.ExistingSteps {
		existing[step.ID] = step
	}
	if err := r.resetSourceStart(ctx, tx, plan, now); err != nil {
		return err
	}
	for _, step := range plan.Steps {
		old := existing[step.ID]
		if old != nil && !sourceStepChanged(plan.ChangedFields, step.Name) {
			continue
		}
		if err := r.writeSourceStep(ctx, tx, step, old, now); err != nil {
			return err
		}
	}
	for _, step := range plan.Removed {
		if err := r.removeSourceStep(ctx, tx, plan.Workflow.ID, step.ID); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) resetSourceStart(ctx context.Context, tx *sqlx.Tx, plan models.PromptSourcePlan, now time.Time) error {
	var oldStart, newStart string
	for _, step := range plan.ExistingSteps {
		if step.IsStartStep {
			oldStart = step.ID
		}
	}
	for _, step := range plan.Steps {
		if step.IsStartStep {
			newStart = step.ID
		}
	}
	if oldStart == "" || oldStart == newStart {
		return nil
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workflow_steps SET is_start_step = 0, updated_at = ? WHERE id = ?`), now, oldStart)
	return err
}
func (r *Repository) writeSourceStep(ctx context.Context, tx *sqlx.Tx, step, old *models.WorkflowStep, now time.Time) error {
	step.UpdatedAt = now
	step.CreatedAt = now
	if old != nil {
		step.CreatedAt = old.CreatedAt
	}
	events, err := json.Marshal(step.Events)
	if err != nil {
		return err
	}
	target, err := marshalSessionTarget(step.SessionTarget)
	if err != nil {
		return err
	}
	if old == nil {
		return r.insertStepRecord(ctx, tx, step, events, target, nil)
	}
	step.IsStartStep, err = r.writeStepUpdate(ctx, tx, step, &step.IsStartStep, nil, nil, events, target)
	if err != nil {
		return err
	}
	return tx.QueryRowContext(ctx, tx.Rebind(`SELECT order_revision FROM workflow_steps WHERE id = ?`), step.ID).Scan(&step.OrderRevision)
}
func (r *Repository) removeSourceStep(ctx context.Context, tx *sqlx.Tx, workflowID, stepID string) error {
	if err := r.checkSourceStepRemoval(ctx, tx, stepID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workflow_steps WHERE id = ? AND workflow_id = ?`), stepID, workflowID)
	return err
}

// CheckPromptSourceStepRemoval gives preview the same occupancy rule as apply.
// Apply repeats this check in its transaction because preview reserves nothing.
func (r *Repository) CheckPromptSourceStepRemoval(ctx context.Context, stepID string) error {
	return r.checkSourceStepRemoval(ctx, r.ro, stepID)
}

func (r *Repository) checkSourceStepRemoval(ctx context.Context, reader sourceGuardQuery, stepID string) error {
	var count int
	if err := reader.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ? OR queued_for_step_id = ?`), stepID, stepID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: removed step still has tasks", models.ErrPromptSourceConflict)
	}
	return nil
}
func sourceStepChanged(fields []string, name string) bool {
	for _, field := range fields {
		if field == "steps."+name {
			return true
		}
	}
	return false
}
