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

// PrepareMessageAttachmentClaim authenticates staged attachment ownership
// without mutating it. The accepting repository applies the returned claim in
// the same transaction as the message or queue row that references it.
func (s *Service) PrepareMessageAttachmentClaim(ctx context.Context, taskID string, attachments []v1.MessageAttachment) (messagequeue.QueueAttachmentClaim, error) {
	claim := messagequeue.QueueAttachmentClaim{}
	ids, err := s.attachmentIDs(attachments)
	if err != nil {
		return claim, err
	}
	claim.IDs = ids
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

// PrepareQueueAttachmentClaim authenticates staged attachment ownership
// without mutating it. The queue repository applies the returned claim in the
// same transaction as queue admission.
func (s *Service) PrepareQueueAttachmentClaim(ctx context.Context, taskID string, attachments []v1.MessageAttachment) (messagequeue.QueueAttachmentClaim, error) {
	return s.PrepareMessageAttachmentClaim(ctx, taskID, attachments)
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

// ResolveMessageAttachmentOwner recovers the authenticated owner needed to
// replay a legacy durable queue cleanup after its owner column was introduced.
func (s *Service) ResolveMessageAttachmentOwner(
	ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment,
) (string, error) {
	if s.attachmentSvc == nil {
		return "", errors.New("file-backed attachments are unavailable")
	}
	ids := make([]string, 0, len(attachments))
	seen := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := seen[attachment.AttachmentID]; ok {
			continue
		}
		seen[attachment.AttachmentID] = struct{}{}
		ids = append(ids, attachment.AttachmentID)
	}
	if len(ids) == 0 {
		return "", nil
	}
	rows, err := s.attachmentSvc.repo.ListMessageAttachments(ctx, ids)
	if err != nil {
		return "", err
	}
	if len(rows) != len(ids) {
		return "", errors.New("durable cleanup attachment is missing")
	}
	ownerID := ""
	for _, attachment := range rows {
		attachmentOwner, err := validateMessageAttachmentOwner(
			attachment, taskID, sessionID,
		)
		if err != nil {
			return "", err
		}
		if ownerID == "" {
			ownerID = attachmentOwner
		} else if ownerID != attachmentOwner {
			return "", errors.New("durable cleanup attachments have different owners")
		}
	}
	return ownerID, nil
}

func validateMessageAttachmentOwner(
	attachment *models.TaskMessageAttachment, taskID, sessionID string,
) (string, error) {
	// Legacy claim-pending cleanups may still reference staged descriptors.
	if attachment == nil || attachment.TaskID != taskID ||
		(attachment.State != models.AttachmentStateClaimed &&
			attachment.State != models.AttachmentStateStaged) ||
		(attachment.SessionID != "" && attachment.SessionID != sessionID) ||
		attachment.OwnerID == "" {
		return "", errors.New("durable cleanup attachment ownership is invalid")
	}
	return attachment.OwnerID, nil
}

// ReleaseMessageAttachmentsForCleanup provides the durable queue retry path
// with a task/session-scoped release that does not require user auth.
func (s *Service) ReleaseMessageAttachmentsForCleanup(
	ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment,
) error {
	if s.attachmentSvc == nil {
		return errors.New("file-backed attachments are unavailable")
	}
	ids := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	return s.attachmentSvc.ReleaseForCleanup(ctx, taskID, sessionID, ids)
}

// DeleteSessionMessageAttachments removes file-backed prompt attachments
// claimed by a task session that has already been deleted.
func (s *Service) DeleteSessionMessageAttachments(ctx context.Context, taskID, sessionID string) error {
	if s.attachmentSvc == nil {
		return nil
	}
	return s.attachmentSvc.DeleteBySession(ctx, taskID, sessionID)
}

// TransferSessionMessageAttachments keeps claimed prompt attachments aligned
// with a queued session transfer without touching unrelated destination claims.
func (s *Service) TransferSessionMessageAttachments(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	attachmentIDs []string,
) error {
	if s.attachmentSvc == nil {
		if len(attachmentIDs) == 0 {
			return nil
		}
		return errors.New("file-backed attachments are unavailable")
	}
	return s.attachmentSvc.TransferSession(ctx, taskID, oldSessionID, newSessionID, attachmentIDs)
}
