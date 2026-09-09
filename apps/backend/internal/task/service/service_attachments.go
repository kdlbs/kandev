package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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

// PrepareQueueAttachmentClaim authenticates staged attachment ownership
// without mutating it. The queue repository applies the returned claim in the
// same transaction as queue admission.
func (s *Service) PrepareQueueAttachmentClaim(ctx context.Context, taskID string, attachments []v1.MessageAttachment) (messagequeue.QueueAttachmentClaim, error) {
	claim := messagequeue.QueueAttachmentClaim{}
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			claim.IDs = append(claim.IDs, attachment.AttachmentID)
		}
	}
	if len(claim.IDs) == 0 {
		return claim, nil
	}
	if s.attachmentSvc == nil {
		return messagequeue.QueueAttachmentClaim{}, errors.New("file-backed attachments are unavailable")
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return messagequeue.QueueAttachmentClaim{}, err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return messagequeue.QueueAttachmentClaim{}, models.ErrAttachmentForbidden
	}
	claim.OwnerID = identity.UserID
	claim.WorkspaceID = task.WorkspaceID
	return claim, nil
}

// ReleaseMessageAttachments asks the attachment repository to remove candidate
// claims. The repository locks the session and rechecks durable queue and
// transcript references before deleting any descriptor.
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
