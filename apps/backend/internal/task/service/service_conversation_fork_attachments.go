package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) copyConversationForkAttachments(
	ctx context.Context,
	ownerID, workspaceID, sourceTaskID string,
	selected []*models.TaskMessageAttachment,
) ([]models.ConversationForkAttachment, []*models.TaskMessageAttachment, error) {
	if len(selected) == 0 {
		return []models.ConversationForkAttachment{}, nil, nil
	}
	if s.attachmentSvc == nil {
		return nil, nil, models.ErrConversationForkAttachmentMissing
	}
	if err := validateConversationForkAttachmentSources(workspaceID, sourceTaskID, selected); err != nil {
		return nil, nil, err
	}
	copies := make([]*models.TaskMessageAttachment, 0, len(selected))
	descriptors := make([]models.ConversationForkAttachment, 0, len(selected))
	for _, source := range selected {
		descriptor, attachmentCopy, staged, err := s.copyOneConversationForkAttachment(ctx, ownerID, workspaceID, sourceTaskID, source)
		if staged {
			copies = append(copies, attachmentCopy)
		}
		if err != nil {
			return nil, nil, s.failConversationForkAttachmentCopy(ctx, ownerID, copies, err)
		}
		descriptors = append(descriptors, descriptor)
	}
	return descriptors, copies, nil
}

func validateConversationForkAttachmentSources(workspaceID, sourceTaskID string, selected []*models.TaskMessageAttachment) error {
	sizes := make([]int64, 0, len(selected))
	for _, source := range selected {
		if source == nil || source.ID == "" || source.WorkspaceID != workspaceID || source.TaskID != sourceTaskID || source.State != models.AttachmentStateClaimed {
			return models.ErrConversationForkAttachmentMissing
		}
		sizes = append(sizes, source.SizeBytes)
	}
	if err := ValidateAttachmentBatch(sizes); err != nil {
		if errors.Is(err, ErrTooManyAttachments) || errors.Is(err, ErrAttachmentTotalTooLarge) || errors.Is(err, ErrAttachmentTooLarge) {
			return models.ErrConversationForkLimitExceeded
		}
		return err
	}
	return nil
}

func (s *Service) copyOneConversationForkAttachment(
	ctx context.Context,
	ownerID, workspaceID, sourceTaskID string,
	source *models.TaskMessageAttachment,
) (models.ConversationForkAttachment, *models.TaskMessageAttachment, bool, error) {
	opened, file, err := s.attachmentSvc.Open(ctx, source.OwnerID, source.ID)
	if err != nil || opened == nil || file == nil || opened.ID != source.ID || opened.TaskID != sourceTaskID || opened.WorkspaceID != workspaceID || (opened.SessionID != "" && opened.SessionID != source.SessionID) {
		if file != nil {
			_ = file.Close()
		}
		return models.ConversationForkAttachment{}, nil, false, forkAttachmentError(err)
	}
	attachmentCopy, stageErr := s.attachmentSvc.Stage(ctx, ownerID, workspaceID, opened.Name, opened.MimeType, opened.Kind, opened.DeliveryMode, file)
	closeErr := file.Close()
	if stageErr != nil {
		return models.ConversationForkAttachment{}, nil, false, forkAttachmentError(stageErr)
	}
	if closeErr != nil {
		return models.ConversationForkAttachment{}, attachmentCopy, true, fmt.Errorf("close fork source attachment: %w", closeErr)
	}
	if attachmentCopy.SizeBytes != source.SizeBytes {
		return models.ConversationForkAttachment{}, attachmentCopy, true, models.ErrConversationForkAttachmentMissing
	}
	descriptor := models.ConversationForkAttachment{
		ID: attachmentCopy.ID, SourceID: source.ID, Name: attachmentCopy.Name, MediaType: attachmentCopy.MimeType,
		Kind: attachmentCopy.Kind, DeliveryMode: attachmentCopy.DeliveryMode, Size: attachmentCopy.SizeBytes, Available: true,
	}
	return descriptor, attachmentCopy, true, nil
}

func (s *Service) failConversationForkAttachmentCopy(
	ctx context.Context,
	ownerID string,
	copies []*models.TaskMessageAttachment,
	original error,
) error {
	cleanupErr := deleteStagedForkAttachments(ctx, s.attachmentSvc, ownerID, copies)
	return errors.Join(original, cleanupErr)
}

func deleteStagedForkAttachments(ctx context.Context, attachments *AttachmentService, ownerID string, copies []*models.TaskMessageAttachment) error {
	if attachments == nil {
		return nil
	}
	var cleanupErr error
	for _, copy := range copies {
		if copy == nil || copy.ID == "" {
			continue
		}
		if err := attachments.Delete(ctx, ownerID, copy.ID); err != nil && !errors.Is(err, ErrAttachmentNotFound) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func forkAttachmentError(err error) error {
	switch {
	case err == nil:
		return models.ErrConversationForkAttachmentMissing
	case errors.Is(err, ErrAttachmentNotFound), errors.Is(err, ErrAttachmentForbidden), errors.Is(err, ErrAttachmentClaimConflict), errors.Is(err, ErrAttachmentInvalid):
		return models.ErrConversationForkAttachmentMissing
	case errors.Is(err, ErrAttachmentTooLarge), errors.Is(err, ErrAttachmentTotalTooLarge), errors.Is(err, ErrTooManyAttachments):
		return models.ErrConversationForkLimitExceeded
	default:
		return err
	}
}

func sortConversationForkAttachments(messages []*models.Message, attachments []*models.TaskMessageAttachment) {
	messageOrder := make(map[string]int, len(messages))
	for index, message := range messages {
		if message != nil {
			messageOrder[message.ID] = index
		}
	}
	sort.Slice(attachments, func(i, j int) bool {
		left, right := attachments[i], attachments[j]
		leftOrder, leftOK := messageOrder[left.MessageID]
		rightOrder, rightOK := messageOrder[right.MessageID]
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return left.ID < right.ID
	})
}
