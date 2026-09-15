package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

const (
	// MaxAttachmentBytes is the raw (not base64-encoded) size accepted for a
	// single prompt attachment.
	MaxAttachmentBytes int64 = models.MaxMessageAttachmentBytes
	// MaxAttachmentCount bounds the number of files in one prompt submission.
	MaxAttachmentCount           = models.MaxMessageAttachmentCount
	attachmentDeliveryModePrompt = "prompt"
	attachmentDeliveryModePath   = "path"
)

var (
	ErrAttachmentTooLarge      = models.ErrAttachmentTooLarge
	ErrAttachmentTotalTooLarge = models.ErrAttachmentTotalTooLarge
	ErrTooManyAttachments      = models.ErrTooManyAttachments
	ErrAttachmentClaimConflict = models.ErrAttachmentClaimConflict
	ErrAttachmentNotFound      = models.ErrAttachmentNotFound
	ErrAttachmentForbidden     = models.ErrAttachmentForbidden
	ErrAttachmentInvalid       = models.ErrAttachmentInvalid
)

const AttachmentStagedTTL = 24 * time.Hour

// AttachmentDescriptor is the safe client-facing representation of an
// attachment. It deliberately contains no storage path or inline bytes.
type AttachmentDescriptor struct {
	ID           string    `json:"attachment_id"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mime_type"`
	Kind         string    `json:"kind"`
	DeliveryMode string    `json:"delivery_mode"`
	SizeBytes    int64     `json:"size_bytes"`
	State        string    `json:"state"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

type attachmentTaskDeletePreparer interface {
	PrepareMessageAttachmentsForTaskDelete(
		ctx context.Context, taskID string,
	) ([]*models.TaskMessageAttachment, error)
}

// AttachmentService owns private attachment bytes and their durable registry.
type AttachmentService struct {
	repo               repository.AttachmentRepository
	root               string
	authorizeWorkspace func(context.Context, string) error
	authorizeTask      func(context.Context, string) error
	log                *logger.Logger
	lifecycleMu        sync.Mutex
	closingWorkspaces  map[string]int
}

func NewAttachmentService(repo repository.AttachmentRepository, root string, authorizeWorkspace func(context.Context, string) error, log *logger.Logger) (*AttachmentService, error) {
	if repo == nil {
		return nil, errors.New("attachment repository is required")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("attachment storage root is required")
	}
	if log == nil {
		return nil, errors.New("attachment logger is required")
	}
	return &AttachmentService{repo: repo, root: filepath.Join(root, "attachments"), authorizeWorkspace: authorizeWorkspace, log: log.WithFields(zap.String("component", "attachment-service"))}, nil
}

func (s *AttachmentService) ensureRoot() error {
	return os.MkdirAll(s.root, 0o700)
}

// SetTaskAuthorizer wires the per-user task visibility check used to serve
// claimed attachments to transcript readers. Staged attachments remain
// owner-only; claimed attachments are authorized through their owning task.
func (s *AttachmentService) SetTaskAuthorizer(authorizer func(context.Context, string) error) {
	s.authorizeTask = authorizer
}
func (s *AttachmentService) beginWorkspaceDeletion(workspaceID string) func() {
	s.lifecycleMu.Lock()
	if s.closingWorkspaces == nil {
		s.closingWorkspaces = make(map[string]int)
	}
	s.closingWorkspaces[workspaceID]++
	s.lifecycleMu.Unlock()
	return func() {
		s.lifecycleMu.Lock()
		if count := s.closingWorkspaces[workspaceID]; count <= 1 {
			delete(s.closingWorkspaces, workspaceID)
		} else {
			s.closingWorkspaces[workspaceID] = count - 1
		}
		s.lifecycleMu.Unlock()
	}
}

// Stage writes one raw file to a temporary file, validates its exact byte
// count, atomically renames it, then commits the descriptor row. Any failure
// removes both the temporary and committed file so callers never observe a
// usable row without bytes.
func (s *AttachmentService) Stage(ctx context.Context, ownerID, workspaceID, name, mimeType, kind, deliveryMode string, src io.Reader) (*models.TaskMessageAttachment, error) {
	if workspaceID == "" || ownerID == "" || src == nil {
		return nil, ErrAttachmentInvalid
	}
	if s.authorizeWorkspace != nil {
		if err := s.authorizeWorkspace(ctx, workspaceID); err != nil {
			return nil, err
		}
	}
	if deliveryMode == "" {
		deliveryMode = attachmentDeliveryModePrompt
	}
	if err := ValidateAttachmentMetadata(name, mimeType, kind, deliveryMode); err != nil {
		return nil, err
	}
	if err := s.ensureRoot(); err != nil {
		return nil, fmt.Errorf("create attachment storage: %w", err)
	}
	id := uuid.NewString()
	storageKey := id
	dest, n, err := s.writeAttachmentBytes(storageKey, src)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: id, OwnerID: ownerID, WorkspaceID: workspaceID, Name: filepath.Base(name),
		MimeType: mimeType, Kind: kind, DeliveryMode: deliveryMode, SizeBytes: n,
		StorageKey: storageKey, State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(AttachmentStagedTTL), CreatedAt: now, UpdatedAt: now,
	}
	s.lifecycleMu.Lock()
	_, closing := s.closingWorkspaces[workspaceID]
	if closing {
		s.lifecycleMu.Unlock()
		_ = os.Remove(dest)
		return nil, ErrAttachmentInvalid
	}
	err = s.repo.CreateMessageAttachment(ctx, attachment)
	s.lifecycleMu.Unlock()
	if err != nil {
		_ = os.Remove(dest)
		return nil, err
	}
	return attachment, nil
}

func (s *AttachmentService) writeAttachmentBytes(storageKey string, src io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("create attachment temp file: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	n, copyErr := io.Copy(tmp, io.LimitReader(src, MaxAttachmentBytes+1))
	if copyErr != nil {
		return "", 0, fmt.Errorf("write attachment: %w", copyErr)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("close attachment: %w", err)
	}
	if err := ValidateAttachmentSize(n); err != nil {
		return "", 0, err
	}
	dest := filepath.Join(s.root, storageKey)
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return "", 0, fmt.Errorf("secure attachment temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", 0, fmt.Errorf("commit attachment bytes: %w", err)
	}
	renamed = true
	return dest, n, nil
}

func ValidateAttachmentMetadata(name, mimeType, kind, deliveryMode string) error {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("%w: invalid name", ErrAttachmentInvalid)
	}
	if strings.TrimSpace(mimeType) == "" {
		return fmt.Errorf("%w: mime type is required", ErrAttachmentInvalid)
	}
	switch kind {
	case "image", "audio", "resource":
	default:
		return fmt.Errorf("%w: unsupported kind %q", ErrAttachmentInvalid, kind)
	}
	if deliveryMode == "" {
		deliveryMode = attachmentDeliveryModePrompt
	}
	if deliveryMode != attachmentDeliveryModePrompt && deliveryMode != attachmentDeliveryModePath {
		return fmt.Errorf("%w: delivery mode must be prompt or path", ErrAttachmentInvalid)
	}
	return nil
}

func (s *AttachmentService) Get(ctx context.Context, ownerID, id string) (*models.TaskMessageAttachment, error) {
	attachment, err := s.repo.GetMessageAttachment(ctx, id)
	if err != nil {
		return nil, err
	}
	if attachment == nil || attachment.OwnerID != ownerID {
		return nil, ErrAttachmentForbidden
	}
	return attachment, nil
}

func (s *AttachmentService) Open(ctx context.Context, ownerID, id string) (*models.TaskMessageAttachment, *os.File, error) {
	attachment, err := s.repo.GetMessageAttachment(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if attachment == nil {
		return nil, nil, ErrAttachmentForbidden
	}
	switch attachment.State {
	case models.AttachmentStateStaged:
		if attachment.OwnerID != ownerID {
			return nil, nil, ErrAttachmentForbidden
		}
	case models.AttachmentStateClaimed:
		if attachment.TaskID == "" || s.authorizeTask == nil {
			return nil, nil, ErrAttachmentForbidden
		}
		if err := s.authorizeTask(ctx, attachment.TaskID); err != nil {
			return nil, nil, ErrAttachmentForbidden
		}
	default:
		return nil, nil, ErrAttachmentForbidden
	}
	if filepath.Base(attachment.StorageKey) != attachment.StorageKey || attachment.StorageKey == "" {
		return nil, nil, ErrAttachmentInvalid
	}
	file, err := os.Open(filepath.Join(s.root, attachment.StorageKey))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrAttachmentNotFound
		}
		return nil, nil, fmt.Errorf("open attachment: %w", err)
	}
	return attachment, file, nil
}

// ValidatePreviewScreenshot verifies task-feedback screenshot bytes before the
// repository changes their staged claim. Already claimed task-feedback bytes
// are accepted so a lost create response can be retried idempotently.
func (s *AttachmentService) ValidatePreviewScreenshot(
	ctx context.Context,
	ownerID, workspaceID, taskID, id string,
) error {
	attachment, file, err := s.Open(ctx, ownerID, id)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if err := validatePreviewScreenshotMetadata(attachment, ownerID, workspaceID, taskID); err != nil {
		return err
	}
	return validatePreviewScreenshotFile(file, attachment.SizeBytes)
}

func validatePreviewScreenshotMetadata(
	attachment *models.TaskMessageAttachment,
	ownerID, workspaceID, taskID string,
) error {
	if !validPreviewScreenshotClaim(attachment, ownerID, workspaceID, taskID) {
		return ErrAttachmentClaimConflict
	}
	if attachment.MimeType != "image/png" || attachment.Kind != "image" ||
		attachment.DeliveryMode != attachmentDeliveryModePrompt {
		return fmt.Errorf("%w: screenshot attachment metadata", previewfeedback.ErrCaptureInvalid)
	}
	if attachment.SizeBytes <= 0 || attachment.SizeBytes > previewfeedback.MaxScreenshotBytes {
		return previewfeedback.ErrCaptureTooLarge
	}
	return nil
}

func validatePreviewScreenshotFile(file *os.File, expectedSize int64) error {
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect preview screenshot: %w", err)
	}
	if stat.Size() != expectedSize || stat.Size() > previewfeedback.MaxScreenshotBytes {
		return fmt.Errorf("%w: screenshot byte count", previewfeedback.ErrCaptureInvalid)
	}

	var signature [8]byte
	if _, err := io.ReadFull(file, signature[:]); err != nil ||
		!bytes.Equal(signature[:], []byte("\x89PNG\r\n\x1a\n")) {
		return fmt.Errorf("%w: invalid PNG signature", previewfeedback.ErrCaptureInvalid)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek preview screenshot: %w", err)
	}
	config, err := png.DecodeConfig(io.LimitReader(file, previewfeedback.MaxScreenshotBytes+1))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("%w: invalid PNG metadata", previewfeedback.ErrCaptureInvalid)
	}
	if uint64(config.Width)*uint64(config.Height) > uint64(previewfeedback.MaxScreenshotPixels) {
		return previewfeedback.ErrCaptureTooLarge
	}
	return nil
}

func validPreviewScreenshotClaim(
	attachment *models.TaskMessageAttachment,
	ownerID, workspaceID, taskID string,
) bool {
	if attachment == nil || attachment.OwnerID != ownerID || attachment.WorkspaceID != workspaceID {
		return false
	}
	if attachment.State == models.AttachmentStateStaged {
		return attachment.TaskID == "" && attachment.SessionID == "" &&
			attachment.MessageID == "" && attachment.QueueID == ""
	}
	return attachment.State == models.AttachmentStateClaimed && attachment.TaskID == taskID &&
		attachment.SessionID == "" && attachment.MessageID == "" && attachment.QueueID == ""
}

// OpenClaimed opens a descriptor for the internal lifecycle delivery path.
// It deliberately authorizes by the claimed task/session binding rather than
// accepting a caller-supplied owner, and never exposes the storage key.
func (s *AttachmentService) OpenClaimed(ctx context.Context, id, taskID, sessionID string) (io.ReadCloser, string, string, int64, error) {
	attachment, err := s.repo.GetMessageAttachment(ctx, id)
	if err != nil {
		return nil, "", "", 0, err
	}
	if attachment == nil || attachment.State != models.AttachmentStateClaimed ||
		attachment.TaskID != taskID || (attachment.SessionID != "" && attachment.SessionID != sessionID) {
		return nil, "", "", 0, ErrAttachmentForbidden
	}
	if filepath.Base(attachment.StorageKey) != attachment.StorageKey || attachment.StorageKey == "" {
		return nil, "", "", 0, ErrAttachmentInvalid
	}
	file, err := os.Open(filepath.Join(s.root, attachment.StorageKey))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", "", 0, ErrAttachmentNotFound
		}
		return nil, "", "", 0, fmt.Errorf("open claimed attachment: %w", err)
	}
	return file, attachment.Name, attachment.MimeType, attachment.SizeBytes, nil
}

func (s *AttachmentService) Delete(ctx context.Context, ownerID, id string) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	attachment, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	if attachment.State != models.AttachmentStateStaged {
		return ErrAttachmentClaimConflict
	}
	if err := s.removeBytes(attachment); err != nil {
		return err
	}
	return s.repo.DeleteMessageAttachment(ctx, id, ownerID)
}

func (s *AttachmentService) Claim(ctx context.Context, ownerID, workspaceID, taskID, sessionID string, ids []string) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.authorizeTask != nil {
		if err := s.authorizeTask(ctx, taskID); err != nil {
			return err
		}
	}
	if s.authorizeWorkspace != nil {
		if err := s.authorizeWorkspace(ctx, workspaceID); err != nil {
			return err
		}
	}
	return s.repo.ClaimMessageAttachments(ctx, ids, ownerID, workspaceID, taskID, sessionID)
}

func (s *AttachmentService) ClaimQueued(
	ctx context.Context,
	ownerID, workspaceID, taskID, sessionID, queueID string,
	ids []string,
) error {
	if s.authorizeWorkspace != nil {
		if err := s.authorizeWorkspace(ctx, workspaceID); err != nil {
			return err
		}
	}
	repo, ok := s.repo.(repository.QueueAttachmentAdmissionRepository)
	if !ok {
		return errors.New("queued attachment admission is unavailable")
	}
	return repo.ClaimQueuedMessageAttachments(ctx, ids, ownerID, workspaceID, taskID, sessionID, queueID)
}

func (s *AttachmentService) RestoreQueued(
	ctx context.Context,
	ownerID, taskID, sessionID, queueID string,
	ids []string,
) error {
	repo, ok := s.repo.(repository.QueueAttachmentAdmissionRepository)
	if !ok {
		return errors.New("queued attachment admission is unavailable")
	}
	return repo.RestoreQueuedMessageAttachments(ctx, ids, ownerID, taskID, sessionID, queueID)
}

// Release removes claimed descriptors that are no longer referenced by a
// queued message. It is used after an atomic queue replacement succeeds.
func (s *AttachmentService) Release(ctx context.Context, ownerID, taskID, sessionID string, ids []string) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	attachments, err := s.repo.PrepareClaimedMessageAttachmentsForRelease(
		ctx, ids, ownerID, taskID, sessionID,
	)
	if err != nil {
		return err
	}
	var removeErrs []error
	for _, attachment := range attachments {
		if err := s.removeBytes(attachment); err != nil {
			removeErrs = append(removeErrs, err)
			continue
		}
		if err := s.repo.DeleteMessageAttachment(ctx, attachment.ID, ownerID); err != nil {
			removeErrs = append(removeErrs, err)
		}
	}
	return errors.Join(removeErrs...)

}

type claimedAttachmentCleanupRepository interface {
	DeleteClaimedMessageAttachmentsByTaskSession(
		context.Context, []string, string, string,
	) ([]*models.TaskMessageAttachment, error)
}

// ReleaseForCleanup removes claimed descriptors using task/session ownership
// rather than a user identity. It is restricted to durable queue cleanup.
func (s *AttachmentService) ReleaseForCleanup(
	ctx context.Context, taskID, sessionID string, ids []string,
) error {
	repo, ok := s.repo.(claimedAttachmentCleanupRepository)
	if !ok {
		return errors.New("attachment cleanup release is unavailable")
	}
	attachments, err := repo.DeleteClaimedMessageAttachmentsByTaskSession(ctx, ids, taskID, sessionID)
	if err != nil {
		return err
	}
	for _, attachment := range attachments {
		s.removeBytes(attachment)
	}
	return nil
}

// DeleteByTask removes all attachment registry rows and private bytes owned by
// a task. Task deletion must clean claimed rows as well as staged rows because
// only staged rows participate in expiry maintenance.
func (s *AttachmentService) DeleteByTask(ctx context.Context, taskID string) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.deleteByTask(ctx, taskID)
}

func (s *AttachmentService) deleteByTask(ctx context.Context, taskID string) error {
	preparer, prepared := s.repo.(attachmentTaskDeletePreparer)
	var attachments []*models.TaskMessageAttachment
	var err error
	if prepared {
		attachments, err = preparer.PrepareMessageAttachmentsForTaskDelete(ctx, taskID)
	} else {
		attachments, err = s.repo.ListMessageAttachmentsByTask(ctx, taskID)
	}
	if err != nil {
		return err
	}
	var cleanupErrs []error
	for _, attachment := range attachments {
		if err := s.removeBytes(attachment); err != nil {
			cleanupErrs = append(cleanupErrs, err)
			continue
		}
		if prepared {
			if err := s.repo.DeleteMessageAttachment(ctx, attachment.ID, attachment.OwnerID); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
		}
	}
	if prepared {
		return errors.Join(cleanupErrs...)
	}
	if err := errors.Join(cleanupErrs...); err != nil {
		return err
	}
	_, err = s.repo.DeleteMessageAttachmentsByTask(ctx, taskID)
	return err
}

// DeleteDescriptors removes the private bytes represented by a previously
// captured attachment snapshot. The registry row may already be gone after a
// workspace cascade, so byte cleanup does not depend on a live task row.
func (s *AttachmentService) DeleteDescriptors(ctx context.Context, attachments []*models.TaskMessageAttachment) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.deleteDescriptors(ctx, attachments)
}

func (s *AttachmentService) deleteDescriptors(ctx context.Context, attachments []*models.TaskMessageAttachment) error {
	var errs []error
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := s.removeBytes(attachment); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := s.repo.DeleteMessageAttachment(ctx, attachment.ID, attachment.OwnerID); err != nil &&
			!errors.Is(err, ErrAttachmentNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DeleteBySession removes all claimed attachment descriptors and private bytes
// owned by a deleted task session. Staged uploads are intentionally excluded
// because they are not bound to a session and expire independently.
func (s *AttachmentService) DeleteBySession(ctx context.Context, taskID, sessionID string) error {
	attachments, err := s.repo.DeleteMessageAttachmentsBySession(ctx, taskID, sessionID)
	if err != nil {
		return err
	}
	for _, attachment := range attachments {
		s.removeBytes(attachment)
	}
	return nil
}

// TransferSession rebinds only the claimed prompt attachments represented by
// the queue transfer operation. The source-session predicate is the CAS guard
// used by both forward transfer and rollback.
func (s *AttachmentService) TransferSession(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	attachmentIDs []string,
) error {
	return s.repo.TransferMessageAttachments(ctx, taskID, oldSessionID, newSessionID, attachmentIDs)
}

type transactionalWorkspaceAttachmentRepository interface {
	DeleteMessageAttachmentsByWorkspaceTx(
		context.Context,
		*sqlx.Tx,
		string,
	) ([]*models.TaskMessageAttachment, error)
}

func (s *AttachmentService) DeleteWorkspaceAttachmentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
) ([]*models.TaskMessageAttachment, error) {
	repo, ok := s.repo.(transactionalWorkspaceAttachmentRepository)
	if !ok {
		return nil, nil
	}
	return repo.DeleteMessageAttachmentsByWorkspaceTx(ctx, tx, workspaceID)
}

func (s *AttachmentService) RemoveBytes(attachments []*models.TaskMessageAttachment) {
	for _, attachment := range attachments {
		s.removeBytes(attachment)
	}
}
func (s *AttachmentService) Descriptor(attachment *models.TaskMessageAttachment) AttachmentDescriptor {
	return AttachmentDescriptor{
		ID: attachment.ID, Name: attachment.Name, MimeType: attachment.MimeType,
		Kind: attachment.Kind, DeliveryMode: attachment.DeliveryMode,
		SizeBytes: attachment.SizeBytes, State: attachment.State, ExpiresAt: attachment.ExpiresAt,
	}
}
func (s *AttachmentService) CleanupExpired(ctx context.Context) (int, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	expired, err := s.repo.MarkExpiredMessageAttachments(ctx, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	var cleanupErrs []error
	for _, attachment := range expired {
		if err := s.removeBytes(attachment); err != nil {
			cleanupErrs = append(cleanupErrs, err)
			continue
		}
		if err := s.repo.DeleteMessageAttachment(ctx, attachment.ID, attachment.OwnerID); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	return len(expired), errors.Join(cleanupErrs...)
}

func (s *AttachmentService) removeBytes(attachment *models.TaskMessageAttachment) error {
	if attachment == nil || attachment.StorageKey == "" {
		return nil
	}
	if err := os.Remove(filepath.Join(s.root, filepath.Base(attachment.StorageKey))); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warn("remove attachment bytes failed", zap.String("attachment_id", attachment.ID), zap.Error(err))
		return fmt.Errorf("remove attachment bytes %s: %w", attachment.ID, err)
	}
	return nil
}

// ValidateAttachmentSize applies the raw-byte per-file limit. The boundary is
// inclusive so a file exactly MaxAttachmentBytes is valid.
func ValidateAttachmentSize(size int64) error {
	if size < 0 || size > MaxAttachmentBytes {
		return fmt.Errorf("%w: %d bytes (maximum %d)", ErrAttachmentTooLarge, size, MaxAttachmentBytes)
	}
	return nil
}

// ValidateAttachmentBatch applies the shared count and aggregate limits.
func ValidateAttachmentBatch(sizes []int64) error {
	if len(sizes) > MaxAttachmentCount {
		return fmt.Errorf("%w: maximum %d", ErrTooManyAttachments, MaxAttachmentCount)
	}
	var total int64
	for _, size := range sizes {
		if err := ValidateAttachmentSize(size); err != nil {
			return err
		}
		total += size
		if total > MaxAttachmentBytes {
			return fmt.Errorf("%w: maximum %d bytes", ErrAttachmentTotalTooLarge, MaxAttachmentBytes)
		}
	}
	return nil
}
