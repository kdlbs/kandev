package channels

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
)

// commentPostedData represents a comment event payload.
type commentPostedData struct {
	TaskID     string `json:"task_id"`
	CommentID  string `json:"comment_id"`
	AuthorID   string `json:"author_id"`
	AuthorType string `json:"author_type"`
}

// CreateComment persists a task comment and emits a creation event so
// subscribers (e.g. channel relay) can react.
func (s *ChannelService) CreateComment(ctx context.Context, comment *models.TaskComment) error {
	if err := s.repo.CreateTaskComment(ctx, comment); err != nil {
		return fmt.Errorf("create comment: %w", err)
	}
	s.publishCommentCreated(ctx, comment)
	return nil
}

// ListComments returns all comments for a task.
func (s *ChannelService) ListComments(ctx context.Context, taskID string) ([]*models.TaskComment, error) {
	return s.repo.ListTaskComments(ctx, taskID)
}

// DeleteComment deletes a task comment by ID.
func (s *ChannelService) DeleteComment(ctx context.Context, id string) error {
	return s.repo.DeleteTaskComment(ctx, id)
}

// publishCommentCreated emits an OfficeCommentCreated event.
// If no event bus is configured the call is silently skipped.
func (s *ChannelService) publishCommentCreated(ctx context.Context, comment *models.TaskComment) {
	if s.eb == nil {
		return
	}
	data := commentPostedData{
		TaskID:     comment.TaskID,
		CommentID:  comment.ID,
		AuthorID:   comment.AuthorID,
		AuthorType: comment.AuthorType,
	}
	event := bus.NewEvent(events.OfficeCommentCreated, "channels-service", data)
	if err := s.eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		s.logger.Error("publish comment created event failed", zap.Error(err))
	}
}
