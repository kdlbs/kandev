package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

var (
	ErrPreviewFeedbackIDRequired      = errors.New("preview feedback id is required")
	ErrPreviewFeedbackIDInvalid       = errors.New("preview feedback id must be a UUID")
	ErrPreviewFeedbackVersionRequired = errors.New("preview feedback expected_version must be positive")
	ErrPreviewFeedbackRevisionInvalid = errors.New("preview feedback expected_revision must not be negative")
	ErrTaskPreviewFeedbackChanged     = errors.New("task preview feedback changed")
)

func (s *PlanService) previewFeedbackRepo() (repository.PreviewFeedbackRepository, error) {
	repo, ok := s.repo.(repository.PreviewFeedbackRepository)
	if !ok {
		return nil, errors.New("task preview feedback persistence is unavailable")
	}
	return repo, nil
}

func (s *PlanService) ListPreviewFeedback(
	ctx context.Context,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	repo, err := s.previewFeedbackRepo()
	if err != nil {
		return nil, err
	}
	snapshot, err := repo.ListTaskPreviewFeedback(ctx, taskID)
	return snapshot, mapPreviewFeedbackError(err)
}

func (s *PlanService) CreatePreviewFeedback(
	ctx context.Context,
	item *models.TaskPreviewFeedback,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if err := validatePreviewFeedbackIdentity(item); err != nil {
		return nil, err
	}
	if err := previewfeedback.ValidateItem(item); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, item.TaskID); err != nil {
		return nil, err
	}
	repo, err := s.previewFeedbackRepo()
	if err != nil {
		return nil, err
	}
	ownerID, workspaceID, err := s.previewScreenshotScope(ctx, item)
	if err != nil {
		return nil, err
	}
	if item.ScreenshotAttachmentID != "" {
		if s.previewScreenshotValidator == nil {
			return nil, errors.New("preview screenshot validation is unavailable")
		}
		if err := s.previewScreenshotValidator.ValidatePreviewScreenshot(
			ctx, ownerID, workspaceID, item.TaskID, item.ScreenshotAttachmentID,
		); err != nil {
			return nil, err
		}
	}
	snapshot, err := repo.CreateTaskPreviewFeedback(ctx, item, ownerID, workspaceID)
	if err != nil {
		return snapshot, mapPreviewFeedbackError(err)
	}
	s.publishPreviewFeedbackSnapshot(ctx, "create", snapshot)
	return snapshot, nil
}

func (s *PlanService) UpdatePreviewFeedback(
	ctx context.Context,
	taskID, itemID, comment string,
	expectedVersion int64,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if err := validatePreviewFeedbackMutation(taskID, itemID, expectedVersion); err != nil {
		return nil, err
	}
	if err := previewfeedback.ValidateComment(comment); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	repo, err := s.previewFeedbackRepo()
	if err != nil {
		return nil, err
	}
	snapshot, err := repo.UpdateTaskPreviewFeedback(ctx, taskID, itemID, comment, expectedVersion)
	if err != nil {
		return snapshot, mapPreviewFeedbackError(err)
	}
	s.publishPreviewFeedbackSnapshot(ctx, "update", snapshot)
	return snapshot, nil
}

func (s *PlanService) DeletePreviewFeedback(
	ctx context.Context,
	taskID, itemID string,
	expectedVersion int64,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if err := validatePreviewFeedbackMutation(taskID, itemID, expectedVersion); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	repo, err := s.previewFeedbackRepo()
	if err != nil {
		return nil, err
	}
	snapshot, released, err := repo.DeleteTaskPreviewFeedback(ctx, taskID, itemID, expectedVersion)
	if err != nil {
		return snapshot, mapPreviewFeedbackError(err)
	}
	s.publishPreviewFeedbackSnapshot(ctx, "delete", snapshot)
	s.cleanupPreviewScreenshots(ctx, released)
	return snapshot, nil
}

func (s *PlanService) ClearPreviewFeedback(
	ctx context.Context,
	taskID string,
	expectedRevision int64,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrTaskIDRequired
	}
	if expectedRevision < 0 {
		return nil, ErrPreviewFeedbackRevisionInvalid
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	repo, err := s.previewFeedbackRepo()
	if err != nil {
		return nil, err
	}
	snapshot, released, err := repo.ClearTaskPreviewFeedback(ctx, taskID, expectedRevision)
	if err != nil {
		return snapshot, mapPreviewFeedbackError(err)
	}
	s.publishPreviewFeedbackSnapshot(ctx, "clear", snapshot)
	s.cleanupPreviewScreenshots(ctx, released)
	return snapshot, nil
}

func validatePreviewFeedbackIdentity(item *models.TaskPreviewFeedback) error {
	if item == nil || strings.TrimSpace(item.TaskID) == "" {
		return ErrTaskIDRequired
	}
	if strings.TrimSpace(item.ID) == "" {
		return ErrPreviewFeedbackIDRequired
	}
	if uuid.Validate(item.ID) != nil {
		return ErrPreviewFeedbackIDInvalid
	}
	return nil
}

func validatePreviewFeedbackMutation(taskID, itemID string, expectedVersion int64) error {
	if strings.TrimSpace(taskID) == "" {
		return ErrTaskIDRequired
	}
	if strings.TrimSpace(itemID) == "" {
		return ErrPreviewFeedbackIDRequired
	}
	if uuid.Validate(itemID) != nil {
		return ErrPreviewFeedbackIDInvalid
	}
	if expectedVersion <= 0 {
		return ErrPreviewFeedbackVersionRequired
	}
	return nil
}

func (s *PlanService) previewScreenshotScope(
	ctx context.Context,
	item *models.TaskPreviewFeedback,
) (string, string, error) {
	if item.ScreenshotAttachmentID == "" {
		return "", "", nil
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		return "", "", models.ErrAttachmentForbidden
	}
	task, err := s.repo.GetTask(ctx, item.TaskID)
	if err != nil {
		return "", "", err
	}
	return identity.UserID, task.WorkspaceID, nil
}

func mapPreviewFeedbackError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrTaskPreviewFeedbackChanged) {
		return ErrTaskPreviewFeedbackChanged
	}
	if errors.Is(err, repository.ErrTaskNotFound) {
		return repository.ErrTaskNotFound
	}
	return err
}

func (s *PlanService) publishPreviewFeedbackSnapshot(
	ctx context.Context,
	mutation string,
	snapshot *models.TaskPreviewFeedbackSnapshot,
) {
	if s.eventBus == nil {
		return
	}
	if err := s.eventBus.Publish(ctx, events.TaskPreviewFeedbackChanged,
		bus.NewEvent(events.TaskPreviewFeedbackChanged, "preview-feedback-service", snapshot)); err != nil {
		s.logger.Error("publish preview feedback snapshot",
			zap.String("task_id", snapshot.TaskID),
			zap.Int64("revision", snapshot.Revision),
			zap.String("mutation", mutation),
			zap.Int("item_count", len(snapshot.Items)),
			zap.Error(err),
		)
	}
}

func (s *PlanService) cleanupPreviewScreenshots(
	ctx context.Context,
	attachments []*models.TaskMessageAttachment,
) {
	if s.previewCleaner == nil || len(attachments) == 0 {
		return
	}
	if err := s.previewCleaner.DeleteDescriptors(ctx, attachments); err != nil {
		s.logger.Warn("cleanup preview feedback screenshots",
			zap.Int("attachment_count", len(attachments)), zap.Error(err))
	}
}
