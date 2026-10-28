package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/previewfeedbacktx"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PreviewFeedbackAttachments returns server-owned screenshot descriptors for
// the exact visible snapshot submitted by the composer.
func (s *Service) PreviewFeedbackAttachments(
	ctx context.Context,
	taskID string,
	refs []models.TaskPreviewFeedbackRef,
) ([]v1.MessageAttachment, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if _, err := s.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	repo, ok := s.messages.(interface {
		ListTaskPreviewFeedback(context.Context, string) (*models.TaskPreviewFeedbackSnapshot, error)
	})
	if !ok {
		return nil, errors.New("preview feedback repository is unavailable")
	}
	snapshot, err := repo.ListTaskPreviewFeedback(ctx, taskID)
	if err != nil {
		return nil, err
	}
	wanted, err := wantedPreviewFeedbackVersions(refs, snapshot)
	if err != nil {
		return nil, err
	}
	attachments, matched, err := collectPreviewFeedbackAttachments(snapshot, wanted)
	if err != nil {
		return nil, err
	}
	if matched != len(refs) {
		return nil, &previewfeedbacktx.FeedbackChangedError{Snapshot: snapshot}
	}
	return attachments, nil
}

func wantedPreviewFeedbackVersions(
	refs []models.TaskPreviewFeedbackRef,
	snapshot *models.TaskPreviewFeedbackSnapshot,
) (map[string]int64, error) {
	wanted := make(map[string]int64, len(refs))
	for _, ref := range refs {
		if ref.ID == "" || ref.Version <= 0 {
			return nil, &previewfeedbacktx.FeedbackChangedError{Snapshot: snapshot}
		}
		if _, duplicate := wanted[ref.ID]; duplicate {
			return nil, &previewfeedbacktx.FeedbackChangedError{Snapshot: snapshot}
		}
		wanted[ref.ID] = ref.Version
	}
	return wanted, nil
}

func collectPreviewFeedbackAttachments(
	snapshot *models.TaskPreviewFeedbackSnapshot,
	wanted map[string]int64,
) ([]v1.MessageAttachment, int, error) {
	attachments := make([]v1.MessageAttachment, 0)
	matched := 0
	for _, item := range snapshot.Items {
		version, selected := wanted[item.ID]
		if !selected {
			continue
		}
		if item.Version != version {
			return nil, 0, &previewfeedbacktx.FeedbackChangedError{Snapshot: snapshot}
		}
		matched++
		if item.Kind != models.TaskPreviewFeedbackScreenshot {
			continue
		}
		attachment := item.ScreenshotAttachment
		if attachment == nil || attachment.ID != item.ScreenshotAttachmentID {
			return nil, 0, models.ErrAttachmentClaimConflict
		}
		attachments = append(attachments, v1.MessageAttachment{
			Type: attachment.Kind, AttachmentID: attachment.ID, MimeType: attachment.MimeType,
			Name: attachment.Name, SizeBytes: attachment.SizeBytes, DeliveryMode: attachment.DeliveryMode,
		})
	}
	return attachments, matched, nil
}

// prepareCreationAttachmentClaim authenticates and validates the file-backed
// creation set. The task insert applies it in its own transaction.
func (s *Service) prepareCreationAttachmentClaim(
	ctx context.Context, req *CreateTaskRequest,
) (*models.TaskCreationAttachmentClaim, error) {
	ids := make([]string, 0, len(req.Attachments))
	sizes := make([]int64, 0, len(req.Attachments))
	for i, attachment := range req.Attachments {
		if attachment.AttachmentID == "" {
			continue
		}
		if attachment.Data != "" {
			return nil, fmt.Errorf("%w: attachment[%d] mixes a descriptor with inline data", ErrAttachmentInvalid, i)
		}
		if err := ValidateAttachmentMetadata(
			attachment.Name, attachment.MimeType, attachment.Type, attachment.DeliveryMode,
		); err != nil {
			return nil, err
		}
		if err := ValidateAttachmentSize(attachment.SizeBytes); err != nil {
			return nil, err
		}
		ids = append(ids, attachment.AttachmentID)
		sizes = append(sizes, attachment.SizeBytes)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := ValidateAttachmentBatch(sizes); err != nil {
		return nil, err
	}
	if s.attachmentSvc == nil {
		return nil, errors.New("file-backed attachments are unavailable")
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return nil, models.ErrAttachmentForbidden
	}
	return &models.TaskCreationAttachmentClaim{
		OwnerID: identity.UserID, WorkspaceID: req.WorkspaceID, IDs: ids,
	}, nil
}

// ReadTaskCreationAttachments returns the canonical ordered creation set for a
// task-bound launch. A partial, foreign, or malformed set fails closed.
func (s *Service) ReadTaskCreationAttachments(
	ctx context.Context, taskID string,
) ([]v1.MessageAttachment, error) {
	if taskID == "" {
		return nil, nil
	}
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, taskrepo.ErrTaskNotFound
	}
	reader, ok := s.tasks.(taskrepo.TaskCreationAttachmentRepository)
	if !ok {
		return nil, errors.New("task creation attachment reads are unavailable")
	}
	rows, err := reader.ListTaskCreationAttachments(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > MaxAttachmentCount {
		return nil, ErrTooManyAttachments
	}
	attachments := make([]v1.MessageAttachment, 0, len(rows))
	sizes := make([]int64, 0, len(rows))
	for _, row := range rows {
		attachment, err := taskCreationAttachmentDescriptor(row, taskID, task.WorkspaceID)
		if err != nil {
			return nil, err
		}
		sizes = append(sizes, attachment.SizeBytes)
		attachments = append(attachments, attachment)
	}
	if err := ValidateAttachmentBatch(sizes); err != nil {
		return nil, err
	}
	return attachments, nil
}

func taskCreationAttachmentDescriptor(
	row *models.TaskMessageAttachment, taskID, workspaceID string,
) (v1.MessageAttachment, error) {
	if row == nil || row.State != models.AttachmentStateClaimed ||
		row.TaskID != taskID || row.SessionID != "" || row.WorkspaceID != workspaceID {
		return v1.MessageAttachment{}, ErrAttachmentClaimConflict
	}
	if err := ValidateAttachmentMetadata(row.Name, row.MimeType, row.Kind, row.DeliveryMode); err != nil {
		return v1.MessageAttachment{}, err
	}
	if err := ValidateAttachmentSize(row.SizeBytes); err != nil {
		return v1.MessageAttachment{}, err
	}
	if row.StorageKey == "" || filepath.Base(row.StorageKey) != row.StorageKey {
		return v1.MessageAttachment{}, ErrAttachmentInvalid
	}
	return v1.MessageAttachment{
		Type: row.Kind, AttachmentID: row.ID, MimeType: row.MimeType,
		Name: row.Name, SizeBytes: row.SizeBytes, DeliveryMode: row.DeliveryMode,
	}, nil
}

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

// RestoreLaunchMessageAttachments keeps staged uploads reusable when a
// synchronous launch fails before the caller receives a session identity.
func (s *Service) RestoreLaunchMessageAttachments(
	ctx context.Context,
	taskID, sessionID string,
	attachments []v1.MessageAttachment,
) error {
	ids := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	if len(ids) == 0 || s.attachmentSvc == nil {
		return nil
	}
	if _, err := s.GetTask(ctx, taskID); err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return models.ErrAttachmentForbidden
	}
	return s.attachmentSvc.RestoreLaunchClaim(ctx, identity.UserID, taskID, sessionID, ids)
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
