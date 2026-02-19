// Package service provides workflow business logic operations.
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// WorkflowProvider gives the workflow service access to workflows owned by the task domain.
type WorkflowProvider interface {
	ListWorkflows(ctx context.Context, workspaceID string) ([]*taskmodels.Workflow, error)
	GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error)
	CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error)
}

// Service provides workflow business logic
type Service struct {
	repo             *repository.Repository
	logger           *logger.Logger
	workflowProvider WorkflowProvider
}

// SetWorkflowProvider wires the workflow provider (set during service init to break circular deps).
func (s *Service) SetWorkflowProvider(wp WorkflowProvider) {
	s.workflowProvider = wp
}

// NewService creates a new workflow service
func NewService(repo *repository.Repository, log *logger.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "workflow-service")),
	}
}

// ============================================================================
// Template Operations
// ============================================================================

// ListTemplates returns all workflow templates.
func (s *Service) ListTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	templates, err := s.repo.ListTemplates(ctx)
	if err != nil {
		s.logger.Error("failed to list templates", zap.Error(err))
		return nil, err
	}
	return templates, nil
}

// GetTemplate retrieves a workflow template by ID.
func (s *Service) GetTemplate(ctx context.Context, id string) (*models.WorkflowTemplate, error) {
	template, err := s.repo.GetTemplate(ctx, id)
	if err != nil {
		s.logger.Error("failed to get template", zap.String("template_id", id), zap.Error(err))
		return nil, err
	}
	return template, nil
}

// GetSystemTemplates returns only system workflow templates.
func (s *Service) GetSystemTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	templates, err := s.repo.GetSystemTemplates(ctx)
	if err != nil {
		s.logger.Error("failed to get system templates", zap.Error(err))
		return nil, err
	}
	return templates, nil
}

// ============================================================================
// Step Operations
// ============================================================================

// ListStepsByWorkflow returns all workflow steps for a workflow.
func (s *Service) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps by workflow", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	return steps, nil
}

// GetStep retrieves a workflow step by ID.
func (s *Service) GetStep(ctx context.Context, stepID string) (*models.WorkflowStep, error) {
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Error("failed to get step", zap.String("step_id", stepID), zap.Error(err))
		return nil, err
	}
	return step, nil
}

// GetNextStepByPosition returns the next step after the given position for a workflow.
// Steps are ordered by position, so this finds the step with the next higher position.
// Returns nil if there is no next step (i.e., current step is the last one).
func (s *Service) GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for next step lookup",
			zap.String("workflow_id", workflowID),
			zap.Error(err))
		return nil, err
	}

	// Steps are already ordered by position from ListStepsByWorkflow
	for _, step := range steps {
		if step.Position > currentPosition {
			return step, nil
		}
	}

	return nil, nil // No next step found (current step is the last one)
}

// GetPreviousStepByPosition returns the previous step before the given position for a workflow.
// Returns nil if there is no previous step (i.e., current step is the first one).
func (s *Service) GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for previous step lookup",
			zap.String("workflow_id", workflowID),
			zap.Error(err))
		return nil, err
	}

	// Steps are ordered by position ascending. Walk backwards to find the step just before current.
	var prev *models.WorkflowStep
	for _, step := range steps {
		if step.Position >= currentPosition {
			break
		}
		prev = step
	}

	return prev, nil
}

// CreateStepsFromTemplate creates workflow steps for a workflow from a template.
func (s *Service) CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error {
	template, err := s.repo.GetTemplate(ctx, templateID)
	if err != nil {
		s.logger.Error("failed to get template for step creation",
			zap.String("template_id", templateID),
			zap.Error(err))
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Build mapping from template step ID to new UUID
	idMap := make(map[string]string, len(template.Steps))
	for _, stepDef := range template.Steps {
		idMap[stepDef.ID] = uuid.New().String()
	}

	// Create each step from the template, remapping step_id references in events
	for _, stepDef := range template.Steps {
		events := models.RemapStepEvents(stepDef.Events, idMap)
		step := &models.WorkflowStep{
			ID:              idMap[stepDef.ID],
			WorkflowID:      workflowID,
			Name:            stepDef.Name,
			Position:        stepDef.Position,
			Color:           stepDef.Color,
			Prompt:          stepDef.Prompt,
			Events:          events,
			AllowManualMove: stepDef.AllowManualMove,
			IsStartStep:     stepDef.IsStartStep,
		}

		if err := s.repo.CreateStep(ctx, step); err != nil {
			s.logger.Error("failed to create step from template",
				zap.String("workflow_id", workflowID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			return fmt.Errorf("failed to create step %s: %w", step.Name, err)
		}
	}

	s.logger.Info("created workflow steps from template",
		zap.String("workflow_id", workflowID),
		zap.String("template_id", templateID),
		zap.Int("step_count", len(template.Steps)))

	return nil
}

// ResolveStartStep resolves which step a task should start in for a workflow.
// Fallback chain: is_start_step=true → first step with auto_start_agent → first step by position.
func (s *Service) ResolveStartStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	// Try explicit start step
	startStep, err := s.repo.GetStartStep(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to get start step", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	if startStep != nil {
		return startStep, nil
	}

	// Fallback: first step with auto_start_agent
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to list steps for start step resolution", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow %s has no steps", workflowID)
	}

	for _, step := range steps {
		if step.HasOnEnterAction(models.OnEnterAutoStartAgent) {
			return step, nil
		}
	}

	// Fallback: first step by position
	return steps[0], nil
}

// CreateStep creates a new workflow step.
func (s *Service) CreateStep(ctx context.Context, step *models.WorkflowStep) error {
	step.ID = uuid.New().String()
	if err := s.repo.CreateStep(ctx, step); err != nil {
		s.logger.Error("failed to create step", zap.String("workflow_id", step.WorkflowID), zap.Error(err))
		return err
	}
	s.logger.Info("created workflow step", zap.String("step_id", step.ID), zap.String("workflow_id", step.WorkflowID))
	return nil
}

// UpdateStep updates an existing workflow step.
func (s *Service) UpdateStep(ctx context.Context, step *models.WorkflowStep) error {
	// If marking as start step, clear the flag on all other steps first
	if step.IsStartStep {
		if err := s.repo.ClearStartStepFlag(ctx, step.WorkflowID, step.ID); err != nil {
			s.logger.Error("failed to clear start step flag", zap.String("workflow_id", step.WorkflowID), zap.Error(err))
			return err
		}
	}
	if err := s.repo.UpdateStep(ctx, step); err != nil {
		s.logger.Error("failed to update step", zap.String("step_id", step.ID), zap.Error(err))
		return err
	}
	s.logger.Info("updated workflow step", zap.String("step_id", step.ID))
	return nil
}

// DeleteStep deletes a workflow step and clears any references to it from other steps.
func (s *Service) DeleteStep(ctx context.Context, stepID string) error {
	// First, get the step to find its workflow ID
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Error("failed to get step for deletion", zap.String("step_id", stepID), zap.Error(err))
		return err
	}

	// Clear any move_to_step references to this step
	if err := s.repo.ClearStepReferences(ctx, step.WorkflowID, stepID); err != nil {
		s.logger.Error("failed to clear step references",
			zap.String("step_id", stepID),
			zap.String("workflow_id", step.WorkflowID),
			zap.Error(err))
		return err
	}

	// Now delete the step
	if err := s.repo.DeleteStep(ctx, stepID); err != nil {
		s.logger.Error("failed to delete step", zap.String("step_id", stepID), zap.Error(err))
		return err
	}

	s.logger.Info("deleted workflow step and cleared references",
		zap.String("step_id", stepID),
		zap.String("workflow_id", step.WorkflowID))
	return nil
}

// ReorderSteps reorders workflow steps for a workflow.
func (s *Service) ReorderSteps(ctx context.Context, workflowID string, stepIDs []string) error {
	for i, stepID := range stepIDs {
		step, err := s.repo.GetStep(ctx, stepID)
		if err != nil {
			s.logger.Error("failed to get step for reorder", zap.String("step_id", stepID), zap.Error(err))
			return err
		}
		step.Position = i
		if err := s.repo.UpdateStep(ctx, step); err != nil {
			s.logger.Error("failed to update step position", zap.String("step_id", stepID), zap.Error(err))
			return err
		}
	}
	s.logger.Info("reordered workflow steps", zap.String("workflow_id", workflowID), zap.Int("count", len(stepIDs)))
	return nil
}

// ============================================================================
// History Operations
// ============================================================================

// CreateStepTransition creates a new step transition history entry.
func (s *Service) CreateStepTransition(ctx context.Context, sessionID string, fromStepID, toStepID string, trigger models.StepTransitionTrigger, actorID *string) error {
	history := &models.SessionStepHistory{
		SessionID: sessionID,
		ToStepID:  toStepID,
		Trigger:   trigger,
		ActorID:   actorID,
	}

	if fromStepID != "" {
		history.FromStepID = &fromStepID
	}

	if err := s.repo.CreateHistory(ctx, history); err != nil {
		s.logger.Error("failed to create step transition",
			zap.String("session_id", sessionID),
			zap.String("to_step_id", toStepID),
			zap.Error(err))
		return err
	}

	s.logger.Info("step transition recorded",
		zap.String("session_id", sessionID),
		zap.Stringp("from_step_id", history.FromStepID),
		zap.String("to_step_id", toStepID),
		zap.String("trigger", string(trigger)))

	return nil
}

// ListHistoryBySession returns all step history entries for a session.
func (s *Service) ListHistoryBySession(ctx context.Context, sessionID string) ([]*models.SessionStepHistory, error) {
	history, err := s.repo.ListHistoryBySession(ctx, sessionID)
	if err != nil {
		s.logger.Error("failed to list history by session", zap.String("session_id", sessionID), zap.Error(err))
		return nil, err
	}
	return history, nil
}

// ============================================================================
// Export/Import Operations
// ============================================================================

// ImportResult holds the outcome of an import operation.
type ImportResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
}

// ExportWorkflow exports a single workflow with its steps as portable JSON.
func (s *Service) ExportWorkflow(ctx context.Context, workflowID string) (*models.WorkflowExport, error) {
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	steps, err := s.repo.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list steps: %w", err)
	}
	stepMap := map[string][]*models.WorkflowStep{wf.ID: steps}
	return models.BuildWorkflowExport([]*taskmodels.Workflow{wf}, stepMap), nil
}

// ExportWorkflows exports all workflows for a workspace.
func (s *Service) ExportWorkflows(ctx context.Context, workspaceID string) (*models.WorkflowExport, error) {
	workflows, err := s.workflowProvider.ListWorkflows(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	stepMap := make(map[string][]*models.WorkflowStep, len(workflows))
	for _, wf := range workflows {
		steps, stepsErr := s.repo.ListStepsByWorkflow(ctx, wf.ID)
		if stepsErr != nil {
			return nil, fmt.Errorf("failed to list steps for workflow %s: %w", wf.ID, stepsErr)
		}
		stepMap[wf.ID] = steps
	}
	return models.BuildWorkflowExport(workflows, stepMap), nil
}

// ImportWorkflows imports workflows into a workspace. Deduplicates by name.
func (s *Service) ImportWorkflows(ctx context.Context, workspaceID string, export *models.WorkflowExport) (*ImportResult, error) {
	if err := export.Validate(); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}

	existing, err := s.workflowProvider.ListWorkflows(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing workflows: %w", err)
	}
	existingNames := make(map[string]bool, len(existing))
	for _, wf := range existing {
		existingNames[wf.Name] = true
	}

	result := &ImportResult{}
	for _, pw := range export.Workflows {
		if existingNames[pw.Name] {
			result.Skipped = append(result.Skipped, pw.Name)
			continue
		}
		if err := s.importSingleWorkflow(ctx, workspaceID, pw); err != nil {
			return nil, fmt.Errorf("failed to import workflow %q: %w", pw.Name, err)
		}
		result.Created = append(result.Created, pw.Name)
	}

	s.logger.Info("imported workflows",
		zap.String("workspace_id", workspaceID),
		zap.Int("created", len(result.Created)),
		zap.Int("skipped", len(result.Skipped)))
	return result, nil
}

func (s *Service) importSingleWorkflow(ctx context.Context, workspaceID string, pw models.WorkflowPortable) error {
	wf, err := s.workflowProvider.CreateWorkflow(ctx, workspaceID, pw.Name, pw.Description)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	// Generate UUIDs for all steps and build position→ID map.
	posToID := make(map[int]string, len(pw.Steps))
	for _, sp := range pw.Steps {
		posToID[sp.Position] = uuid.New().String()
	}

	// Create each step with remapped events.
	for _, sp := range pw.Steps {
		step := &models.WorkflowStep{
			ID:                    posToID[sp.Position],
			WorkflowID:            wf.ID,
			Name:                  sp.Name,
			Position:              sp.Position,
			Color:                 sp.Color,
			Prompt:                sp.Prompt,
			Events:                models.ConvertPositionToStepID(sp.Events, posToID),
			IsStartStep:           sp.IsStartStep,
			AllowManualMove:       sp.AllowManualMove,
			AutoArchiveAfterHours: sp.AutoArchiveAfterHours,
		}
		if err := s.repo.CreateStep(ctx, step); err != nil {
			return fmt.Errorf("create step %q: %w", sp.Name, err)
		}
	}
	return nil
}
