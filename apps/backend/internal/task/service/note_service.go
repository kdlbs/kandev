package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

// ErrTaskNoteNotFound is returned when no note exists for a task.
var ErrTaskNoteNotFound = errors.New("task note not found")

type noteRepo interface {
	repository.NoteRepository
}

// NoteService provides task note business logic.
type NoteService struct {
	repo          noteRepo
	eventBus      bus.EventBus
	logger        *logger.Logger
	authorizeTask func(ctx context.Context, taskID string) error
}

// NewNoteService creates a new task note service.
func NewNoteService(repo noteRepo, eventBus bus.EventBus, log *logger.Logger) *NoteService {
	return &NoteService{
		repo:     repo,
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "note-service")),
	}
}

// SetTaskAuthorizer wires the per-user task-access check (opt-in auth).
func (s *NoteService) SetTaskAuthorizer(fn func(ctx context.Context, taskID string) error) {
	s.authorizeTask = fn
}

func (s *NoteService) authorize(ctx context.Context, taskID string) error {
	if s.authorizeTask == nil {
		return nil
	}
	return s.authorizeTask(ctx, taskID)
}

// GetNote returns a task's note, or nil, nil when none exists.
func (s *NoteService) GetNote(ctx context.Context, taskID string) (*models.TaskNote, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.GetTaskNote(ctx, taskID)
}

// UpsertNote creates or replaces a task's note.
func (s *NoteService) UpsertNote(ctx context.Context, taskID, content, updatedBy string) (*models.TaskNote, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	if updatedBy == "" {
		updatedBy = createdByUser
	}

	existing, err := s.repo.GetTaskNote(ctx, taskID)
	if err != nil {
		return nil, err
	}
	note := &models.TaskNote{
		TaskID:    taskID,
		Content:   content,
		UpdatedBy: updatedBy,
	}
	if existing != nil {
		note.ID = existing.ID
		note.CreatedAt = existing.CreatedAt
	}
	if err := s.repo.UpsertTaskNote(ctx, note); err != nil {
		s.logger.Error("upsert task note", zap.String("task_id", taskID), zap.Error(err))
		return nil, err
	}
	saved, err := s.repo.GetTaskNote(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.publishEvent(ctx, events.TaskNoteUpdated, saved)
	return saved, nil
}

// DeleteNote deletes a task's note.
func (s *NoteService) DeleteNote(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return err
	}
	if err := s.repo.DeleteTaskNote(ctx, taskID); err != nil {
		if errors.Is(err, repository.ErrTaskNoteNotFound) {
			return ErrTaskNoteNotFound
		}
		return err
	}
	if s.eventBus != nil {
		s.publishDeletedEvent(ctx, taskID)
	}
	return nil
}

func (s *NoteService) publishEvent(ctx context.Context, eventType string, note *models.TaskNote) {
	if s.eventBus == nil || note == nil {
		return
	}
	payload := map[string]interface{}{
		"id":         note.ID,
		"task_id":    note.TaskID,
		"content":    note.Content,
		"updated_by": note.UpdatedBy,
		"created_at": note.CreatedAt,
		"updated_at": note.UpdatedAt,
	}
	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "note-service", payload)); err != nil {
		s.logger.Error("publish task note event", zap.String("event_type", eventType), zap.Error(err))
	}
}

func (s *NoteService) publishDeletedEvent(ctx context.Context, taskID string) {
	payload := map[string]interface{}{"task_id": taskID}
	if err := s.eventBus.Publish(ctx, events.TaskNoteDeleted, bus.NewEvent(events.TaskNoteDeleted, "note-service", payload)); err != nil {
		s.logger.Error("publish task note delete event", zap.String("task_id", taskID), zap.Error(err))
	}
}
