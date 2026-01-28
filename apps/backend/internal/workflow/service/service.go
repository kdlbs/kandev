// Package service provides workflow business logic operations.
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// Service provides workflow business logic
type Service struct {
	repo   *repository.Repository
	logger *logger.Logger
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

// ListStepsByBoard returns all workflow steps for a board.
func (s *Service) ListStepsByBoard(ctx context.Context, boardID string) ([]*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByBoard(ctx, boardID)
	if err != nil {
		s.logger.Error("failed to list steps by board", zap.String("board_id", boardID), zap.Error(err))
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

// GetNextStep gets the step to transition to based on OnCompleteStepID.
func (s *Service) GetNextStep(ctx context.Context, boardID, currentStepID string) (*models.WorkflowStep, error) {
	currentStep, err := s.repo.GetStep(ctx, currentStepID)
	if err != nil {
		s.logger.Error("failed to get current step", zap.String("step_id", currentStepID), zap.Error(err))
		return nil, err
	}

	if currentStep.OnCompleteStepID == nil {
		return nil, nil // No next step configured
	}

	nextStep, err := s.repo.GetStep(ctx, *currentStep.OnCompleteStepID)
	if err != nil {
		s.logger.Error("failed to get next step",
			zap.String("current_step_id", currentStepID),
			zap.String("next_step_id", *currentStep.OnCompleteStepID),
			zap.Error(err))
		return nil, err
	}

	return nextStep, nil
}

// GetSourceStep finds the step that has on_complete_step_id pointing to the given step.
// This is used to find the "previous" step when moving back from a review step.
// Returns nil if no source step is found.
func (s *Service) GetSourceStep(ctx context.Context, boardID, targetStepID string) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByBoard(ctx, boardID)
	if err != nil {
		s.logger.Error("failed to list steps for source lookup",
			zap.String("board_id", boardID),
			zap.Error(err))
		return nil, err
	}

	for _, step := range steps {
		if step.OnCompleteStepID != nil && *step.OnCompleteStepID == targetStepID {
			return step, nil
		}
	}

	return nil, nil // No source step found
}

// GetNextStepByPosition returns the next step after the given position for a board.
// This is used as a fallback when no explicit transition (OnApprovalStepID, OnCompleteStepID) is configured.
// Steps are ordered by position, so this finds the step with the next higher position.
// Returns nil if there is no next step (i.e., current step is the last one).
func (s *Service) GetNextStepByPosition(ctx context.Context, boardID string, currentPosition int) (*models.WorkflowStep, error) {
	steps, err := s.repo.ListStepsByBoard(ctx, boardID)
	if err != nil {
		s.logger.Error("failed to list steps for next step lookup",
			zap.String("board_id", boardID),
			zap.Error(err))
		return nil, err
	}

	// Steps are already ordered by position from ListStepsByBoard
	for _, step := range steps {
		if step.Position > currentPosition {
			return step, nil
		}
	}

	return nil, nil // No next step found (current step is the last one)
}

// CreateStepsFromTemplate creates workflow steps for a board from a template.
func (s *Service) CreateStepsFromTemplate(ctx context.Context, boardID, templateID string) error {
	template, err := s.repo.GetTemplate(ctx, templateID)
	if err != nil {
		s.logger.Error("failed to get template for step creation",
			zap.String("template_id", templateID),
			zap.Error(err))
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Map template step IDs to generated UUIDs for linking
	idMap := make(map[string]string)
	for _, stepDef := range template.Steps {
		idMap[stepDef.ID] = uuid.New().String()
	}

	// Create each step from the template
	for _, stepDef := range template.Steps {
		step := &models.WorkflowStep{
			ID:              idMap[stepDef.ID],
			BoardID:         boardID,
			Name:            stepDef.Name,
			StepType:        stepDef.StepType,
			Position:        stepDef.Position,
			Color:           stepDef.Color,
			AutoStartAgent:  stepDef.AutoStartAgent,
			PlanMode:        stepDef.PlanMode,
			RequireApproval: stepDef.RequireApproval,
			PromptPrefix:    stepDef.PromptPrefix,
			PromptSuffix:    stepDef.PromptSuffix,
			AllowManualMove: stepDef.AllowManualMove,
		}

		// Map OnCompleteStepID if set
		if stepDef.OnCompleteStepID != "" {
			if mappedID, ok := idMap[stepDef.OnCompleteStepID]; ok {
				step.OnCompleteStepID = &mappedID
			}
		}

		// Map OnApprovalStepID if set
		if stepDef.OnApprovalStepID != "" {
			if mappedID, ok := idMap[stepDef.OnApprovalStepID]; ok {
				step.OnApprovalStepID = &mappedID
			}
		}

		if err := s.repo.CreateStep(ctx, step); err != nil {
			s.logger.Error("failed to create step from template",
				zap.String("board_id", boardID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			return fmt.Errorf("failed to create step %s: %w", step.Name, err)
		}
	}

	s.logger.Info("created workflow steps from template",
		zap.String("board_id", boardID),
		zap.String("template_id", templateID),
		zap.Int("step_count", len(template.Steps)))

	return nil
}

// CreateStep creates a new workflow step.
func (s *Service) CreateStep(ctx context.Context, step *models.WorkflowStep) error {
	step.ID = uuid.New().String()
	if err := s.repo.CreateStep(ctx, step); err != nil {
		s.logger.Error("failed to create step", zap.String("board_id", step.BoardID), zap.Error(err))
		return err
	}
	s.logger.Info("created workflow step", zap.String("step_id", step.ID), zap.String("board_id", step.BoardID))
	return nil
}

// UpdateStep updates an existing workflow step.
func (s *Service) UpdateStep(ctx context.Context, step *models.WorkflowStep) error {
	if err := s.repo.UpdateStep(ctx, step); err != nil {
		s.logger.Error("failed to update step", zap.String("step_id", step.ID), zap.Error(err))
		return err
	}
	s.logger.Info("updated workflow step", zap.String("step_id", step.ID))
	return nil
}

// DeleteStep deletes a workflow step and clears any references to it from other steps.
func (s *Service) DeleteStep(ctx context.Context, stepID string) error {
	// First, get the step to find its board ID
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Error("failed to get step for deletion", zap.String("step_id", stepID), zap.Error(err))
		return err
	}

	// Clear any OnCompleteStepID or OnApprovalStepID references to this step
	if err := s.repo.ClearStepReferences(ctx, step.BoardID, stepID); err != nil {
		s.logger.Error("failed to clear step references",
			zap.String("step_id", stepID),
			zap.String("board_id", step.BoardID),
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
		zap.String("board_id", step.BoardID))
	return nil
}

// ReorderSteps reorders workflow steps for a board.
func (s *Service) ReorderSteps(ctx context.Context, boardID string, stepIDs []string) error {
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
	s.logger.Info("reordered workflow steps", zap.String("board_id", boardID), zap.Int("count", len(stepIDs)))
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

