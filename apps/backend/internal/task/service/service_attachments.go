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

// ClaimQueuedMessageAttachments binds newly staged descriptors to one queue
// admission token so a rejected admission can restore only its own claims.
func (s *Service) ClaimQueuedMessageAttachments(
	ctx context.Context,
	taskID, sessionID, queueID string,
	attachments []v1.MessageAttachment,
) error {
	ids, err := s.attachmentIDs(attachments)
	if err != nil || len(ids) == 0 {
		return err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.ClaimQueued(
		ctx, identity.UserID, task.WorkspaceID, taskID, sessionID, queueID, ids,
	)
}

func (s *Service) RestoreQueuedMessageAttachments(
	ctx context.Context,
	taskID, sessionID, queueID string,
	attachments []v1.MessageAttachment,
) error {
	ids, err := s.attachmentIDs(attachments)
	if err != nil || len(ids) == 0 {
		return err
	}
	if _, err := s.GetTask(ctx, taskID); err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.RestoreQueued(ctx, identity.UserID, taskID, sessionID, queueID, ids)
}

func (s *Service) ClaimDirectMessageAttachments(
	ctx context.Context,
	taskID, sessionID, messageID string,
	attachments []v1.MessageAttachment,
) error {
	ids, err := s.attachmentIDs(attachments)
	if err != nil || len(ids) == 0 {
		return err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.ClaimDirect(
		ctx, identity.UserID, task.WorkspaceID, taskID, sessionID, messageID, ids,
	)
}

func (s *Service) RestoreDirectMessageAttachments(
	ctx context.Context,
	taskID, sessionID, messageID string,
	attachments []v1.MessageAttachment,
) error {
	ids, err := s.attachmentIDs(attachments)
	if err != nil || len(ids) == 0 {
		return err
	}
	if _, err := s.GetTask(ctx, taskID); err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.RestoreDirect(ctx, identity.UserID, taskID, sessionID, messageID, ids)
}

func (s *Service) attachmentIDs(attachments []v1.MessageAttachment) ([]string, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	if s.attachmentSvc == nil {
		for _, attachment := range attachments {
			if attachment.AttachmentID != "" {
				return nil, errors.New("file-backed attachments are unavailable")
			}
		}
		return nil, nil
	}
	ids := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	return ids, nil
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
