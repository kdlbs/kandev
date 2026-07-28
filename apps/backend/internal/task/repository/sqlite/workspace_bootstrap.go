package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const kanbanTemplateID = "simple"

// CreateWorkspaceWithKanban persists a standard Kanban workspace, its default
// workflow, and all template-derived steps in one transaction.
func (r *Repository) CreateWorkspaceWithKanban(ctx context.Context, workspace *models.Workspace) (*models.Workflow, error) {
	template, err := kanbanTemplate()
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	r.prepareWorkspace(workspace)
	if err := r.insertWorkspace(ctx, tx, workspace); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	workflow := &models.Workflow{
		WorkspaceID:        workspace.ID,
		Name:               "Kanban",
		WorkflowTemplateID: &template.ID,
	}
	r.prepareWorkflow(workflow)
	if err := r.insertWorkflow(ctx, tx, workflow); err != nil {
		return nil, fmt.Errorf("create Kanban workflow: %w", err)
	}
	if err := r.insertTemplateSteps(ctx, tx, workflow.ID, template); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return workflow, nil
}

func kanbanTemplate() (*wfmodels.WorkflowTemplate, error) {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load Kanban template: %w", err)
	}
	for _, template := range templates {
		if template.ID == kanbanTemplateID {
			return template, nil
		}
	}
	return nil, fmt.Errorf("kanban template %q not found", kanbanTemplateID)
}

func (r *Repository) insertTemplateSteps(ctx context.Context, tx *sqlx.Tx, workflowID string, template *wfmodels.WorkflowTemplate) error {
	idMap := make(map[string]string, len(template.Steps))
	for _, stepDef := range template.Steps {
		idMap[stepDef.ID] = uuid.NewString()
	}
	now := time.Now().UTC()
	for _, stepDef := range template.Steps {
		events, err := json.Marshal(wfmodels.RemapStepEvents(stepDef.Events, idMap))
		if err != nil {
			return fmt.Errorf("marshal Kanban step %q: %w", stepDef.Name, err)
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO workflow_steps (
				id, workflow_id, name, position, color, prompt, events,
				allow_manual_move, is_start_step, show_in_command_panel,
				auto_archive_after_hours, agent_profile_id, stage_type,
				auto_advance_requires_signal, wip_limit, pull_from_step_id,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), idMap[stepDef.ID], workflowID, stepDef.Name, stepDef.Position, stepDef.Color, stepDef.Prompt, string(events),
			dialect.BoolToInt(stepDef.AllowManualMove), dialect.BoolToInt(stepDef.IsStartStep), dialect.BoolToInt(stepDef.ShowInCommandPanel),
			stepDef.AutoArchiveAfterHours, stepDef.AgentProfileID, normalizeBootstrapStageType(stepDef.StageType),
			dialect.BoolToInt(stepDef.AutoAdvanceRequiresSignal), stepDef.WIPLimit, wfmodels.RemapStepID(stepDef.PullFromStepID, idMap), now, now); err != nil {
			return fmt.Errorf("create Kanban step %q: %w", stepDef.Name, err)
		}
	}
	return nil
}

func normalizeBootstrapStageType(stageType wfmodels.StageType) string {
	switch stageType {
	case wfmodels.StageTypeWork, wfmodels.StageTypeReview, wfmodels.StageTypeApproval, wfmodels.StageTypeCustom:
		return string(stageType)
	default:
		return string(wfmodels.StageTypeCustom)
	}
}
