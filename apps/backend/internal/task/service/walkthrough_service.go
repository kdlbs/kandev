package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrTaskWalkthroughNotFound is returned when no walkthrough exists for a task.
var ErrTaskWalkthroughNotFound = errors.New("task walkthrough not found")

// Event-payload keys, hoisted to constants to satisfy goconst.
const (
	wtFieldTaskID    = "task_id"
	wtFieldTitle     = "title"
	wtFieldCreatedAt = "created_at"
	wtFieldUpdatedAt = "updated_at"
)

// walkthroughRepo is the minimal repository surface WalkthroughService needs.
// The SQLite repository satisfies it; declared locally so the service does not
// depend on the full aggregate repository interface.
type walkthroughRepo interface {
	CreateTaskWalkthrough(ctx context.Context, wt *models.TaskWalkthrough) error
	GetTaskWalkthrough(ctx context.Context, taskID string) (*models.TaskWalkthrough, error)
	DeleteTaskWalkthrough(ctx context.Context, taskID string) error
}

// WalkthroughService provides agent-authored code-walkthrough business logic.
// Walkthroughs mirror task plans: one per task, agent-authored, persisted, and
// broadcast to the web UI via the event bus.
type WalkthroughService struct {
	repo     walkthroughRepo
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewWalkthroughService creates a new walkthrough service.
func NewWalkthroughService(repo walkthroughRepo, eventBus bus.EventBus, log *logger.Logger) *WalkthroughService {
	return &WalkthroughService{
		repo:     repo,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "walkthrough-service")),
	}
}

// ShowWalkthroughRequest contains parameters for creating/replacing a walkthrough.
type ShowWalkthroughRequest struct {
	TaskID string
	Title  string
	Steps  []models.WalkthroughStep
}

// ShowWalkthrough upserts a task's walkthrough (replacing any existing one) and
// publishes the matching created/updated event.
func (s *WalkthroughService) ShowWalkthrough(ctx context.Context, req ShowWalkthroughRequest) (*models.TaskWalkthrough, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	if len(req.Steps) == 0 {
		return nil, errors.New("at least one step is required")
	}

	existing, err := s.repo.GetTaskWalkthrough(ctx, req.TaskID)
	if err != nil {
		s.logger.Error("get existing walkthrough", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}
	eventType := events.TaskWalkthroughCreated
	if existing != nil {
		eventType = events.TaskWalkthroughUpdated
	}

	title := req.Title
	if title == "" {
		title = "Walkthrough"
	}
	wt := &models.TaskWalkthrough{
		TaskID:    req.TaskID,
		Title:     title,
		Steps:     req.Steps,
		CreatedBy: createdByAgent,
	}
	if existing != nil {
		wt.ID = existing.ID
		wt.CreatedAt = existing.CreatedAt
	}

	if err := s.repo.CreateTaskWalkthrough(ctx, wt); err != nil {
		s.logger.Error("upsert walkthrough", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}

	s.publishEvent(ctx, eventType, wt)
	return wt, nil
}

// GetWalkthrough returns a task's walkthrough, or nil, nil when none exists.
func (s *WalkthroughService) GetWalkthrough(ctx context.Context, taskID string) (*models.TaskWalkthrough, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	return s.repo.GetTaskWalkthrough(ctx, taskID)
}

// DeleteWalkthrough removes a task's walkthrough and publishes the deleted event.
func (s *WalkthroughService) DeleteWalkthrough(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}
	existing, err := s.repo.GetTaskWalkthrough(ctx, taskID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTaskWalkthroughNotFound
	}
	if err := s.repo.DeleteTaskWalkthrough(ctx, taskID); err != nil {
		return err
	}
	s.publishEvent(ctx, events.TaskWalkthroughDeleted, existing)
	return nil
}

func (s *WalkthroughService) publishEvent(ctx context.Context, eventType string, wt *models.TaskWalkthrough) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]interface{}{
		"id":             wt.ID,
		wtFieldTaskID:    wt.TaskID,
		wtFieldTitle:     wt.Title,
		"steps":          wt.Steps,
		"created_by":     wt.CreatedBy,
		wtFieldCreatedAt: wt.CreatedAt,
		wtFieldUpdatedAt: wt.UpdatedAt,
	}
	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "walkthrough-service", payload)); err != nil {
		s.logger.Error("publish walkthrough event", zap.String("event_type", eventType), zap.Error(err))
	}
}
