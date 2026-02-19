package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

var (
	ErrTaskPlanNotFound = errors.New("task plan not found")
	ErrTaskIDRequired   = errors.New("task_id is required")
	ErrContentRequired  = errors.New("content is required")
)

// createdByAgent is the default creator for agent-created plans.
const createdByAgent = "agent"

// PlanService provides task plan business logic.
type PlanService struct {
	repo     repository.Repository
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewPlanService creates a new task plan service.
func NewPlanService(repo repository.Repository, eventBus bus.EventBus, log *logger.Logger) *PlanService {
	return &PlanService{
		repo:     repo,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "plan-service")),
	}
}

// CreatePlanRequest contains parameters for creating a task plan.
type CreatePlanRequest struct {
	TaskID    string
	Title     string
	Content   string
	CreatedBy string // "agent" or "user"
}

// CreatePlan creates a new task plan.
func (s *PlanService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*models.TaskPlan, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}

	title := req.Title
	if title == "" {
		title = "Plan"
	}
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = createdByAgent
	}

	plan := &models.TaskPlan{
		TaskID:    req.TaskID,
		Title:     title,
		Content:   req.Content,
		CreatedBy: createdBy,
	}

	if err := s.repo.CreateTaskPlan(ctx, plan); err != nil {
		s.logger.Error("failed to create task plan", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("created task plan", zap.String("task_id", req.TaskID), zap.String("plan_id", plan.ID))
	s.publishEvent(ctx, events.TaskPlanCreated, plan)

	return plan, nil
}

// GetPlan retrieves a task plan by task ID.
// Returns nil, nil if no plan exists.
func (s *PlanService) GetPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}

	plan, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to get task plan", zap.String("task_id", taskID), zap.Error(err))
		return nil, err
	}

	return plan, nil
}

// UpdatePlanRequest contains parameters for updating a task plan.
type UpdatePlanRequest struct {
	TaskID    string
	Title     string // Optional: if empty, preserves existing title
	Content   string
	CreatedBy string // Optional: if empty, preserves existing or defaults to "user"
}

// UpdatePlan updates an existing task plan.
func (s *PlanService) UpdatePlan(ctx context.Context, req UpdatePlanRequest) (*models.TaskPlan, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}

	existing, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		s.logger.Error("failed to get existing task plan", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}
	if existing == nil {
		return nil, ErrTaskPlanNotFound
	}

	// Preserve existing values if not provided
	title := req.Title
	if title == "" {
		title = existing.Title
	}
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = existing.CreatedBy
	}

	plan := &models.TaskPlan{
		ID:        existing.ID,
		TaskID:    req.TaskID,
		Title:     title,
		Content:   req.Content,
		CreatedBy: createdBy,
		CreatedAt: existing.CreatedAt,
	}

	if err := s.repo.UpdateTaskPlan(ctx, plan); err != nil {
		s.logger.Error("failed to update task plan", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}

	s.logger.Info("updated task plan", zap.String("task_id", req.TaskID))
	s.publishEvent(ctx, events.TaskPlanUpdated, plan)

	return plan, nil
}

// DeletePlan deletes a task plan by task ID.
func (s *PlanService) DeletePlan(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}

	// Get plan before deleting for event payload
	existing, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to get task plan for delete", zap.String("task_id", taskID), zap.Error(err))
		return err
	}
	if existing == nil {
		return ErrTaskPlanNotFound
	}

	if err := s.repo.DeleteTaskPlan(ctx, taskID); err != nil {
		s.logger.Error("failed to delete task plan", zap.String("task_id", taskID), zap.Error(err))
		return err
	}

	s.logger.Info("deleted task plan", zap.String("task_id", taskID))
	s.publishEvent(ctx, events.TaskPlanDeleted, existing)

	return nil
}

// publishEvent publishes a task plan event to the event bus.
func (s *PlanService) publishEvent(ctx context.Context, eventType string, plan *models.TaskPlan) {
	if s.eventBus == nil {
		return
	}

	payload := map[string]interface{}{
		"id":         plan.ID,
		"task_id":    plan.TaskID,
		"title":      plan.Title,
		"content":    plan.Content,
		"created_by": plan.CreatedBy,
		"created_at": plan.CreatedAt,
		"updated_at": plan.UpdatedAt,
	}

	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "plan-service", payload)); err != nil {
		s.logger.Error("failed to publish task plan event", zap.String("event_type", eventType), zap.Error(err))
	}
}
