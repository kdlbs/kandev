package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// queueAttachmentsToV1 converts queue attachments to the API v1 representation.
func queueAttachmentsToV1(attachments []messagequeue.MessageAttachment) []v1.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]v1.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, v1.MessageAttachment{
			CommentID: attachment.CommentID, AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return converted
}

func queueAttachmentsFromV1(attachments []v1.MessageAttachment) []messagequeue.MessageAttachment {
	converted := make([]messagequeue.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, messagequeue.MessageAttachment{
			CommentID: attachment.CommentID, AttachmentID: attachment.AttachmentID, Type: attachment.Type, Data: attachment.Data,
			MimeType: attachment.MimeType, Name: attachment.Name, SizeBytes: attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return converted
}

func (h *QueueHandlers) canonicalizeQueuedCommentFiles(
	ctx context.Context, taskID, content string, attachments []messagequeue.MessageAttachment,
) error {
	if len(attachments) > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	files := queueAttachmentsToV1(attachments)
	if !v1.HasCommentAttachments(files) {
		return nil
	}
	if err := v1.ValidateCommentAttachments(content, files); err != nil {
		return err
	}
	resolver, ok := h.attachmentClaimer.(interface {
		CanonicalizeCommentFiles(context.Context, string, []v1.MessageAttachment) error
	})
	if !ok {
		return errors.New("comment files are unavailable")
	}
	if err := resolver.CanonicalizeCommentFiles(ctx, taskID, files); err != nil {
		return err
	}
	copy(attachments, queueAttachmentsFromV1(files))
	return nil
}
