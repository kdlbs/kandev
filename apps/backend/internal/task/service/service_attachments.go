package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ClaimMessageAttachments binds staged descriptors to a task/session after
// the normal task and session authorization checks have completed.
func (s *Service) ClaimMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	if s.attachmentSvc == nil {
		for _, attachment := range attachments {
			if attachment.AttachmentID != "" {
				return errors.New("file-backed attachments are unavailable")
			}
		}
		return nil
	}
	ids := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.Claim(ctx, identity.UserID, task.WorkspaceID, taskID, sessionID, ids)
}

// ReleaseMessageAttachments removes claimed descriptors that a queue edit no
// longer references. The caller has already authorized the task session.
func (s *Service) ReleaseMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error {
	if len(attachments) == 0 || s.attachmentSvc == nil {
		return nil
	}
	ids := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := s.GetTask(ctx, taskID); err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.Release(ctx, identity.UserID, taskID, sessionID, ids)
}
