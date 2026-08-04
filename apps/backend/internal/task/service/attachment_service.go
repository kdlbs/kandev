package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
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

// AttachmentService owns private attachment bytes and their durable registry.
type AttachmentService struct {
	repo               repository.AttachmentRepository
	root               string
	authorizeWorkspace func(context.Context, string) error
	log                *logger.Logger
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
	if err := s.repo.CreateMessageAttachment(ctx, attachment); err != nil {
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
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\\\x00`) {
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
	attachment, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return nil, nil, err
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
	attachment, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	if attachment.State != models.AttachmentStateStaged {
		return ErrAttachmentClaimConflict
	}
	if err := s.repo.DeleteMessageAttachment(ctx, id, ownerID); err != nil {
		return err
	}
	s.removeBytes(attachment)
	return nil
}

func (s *AttachmentService) Claim(ctx context.Context, ownerID, workspaceID, taskID, sessionID string, ids []string) error {
	if s.authorizeWorkspace != nil {
		if err := s.authorizeWorkspace(ctx, workspaceID); err != nil {
			return err
		}
	}
	return s.repo.ClaimMessageAttachments(ctx, ids, ownerID, workspaceID, taskID, sessionID)
}

// Release removes claimed descriptors that are no longer referenced by a
// queued message. It is used after an atomic queue replacement succeeds.
func (s *AttachmentService) Release(ctx context.Context, ownerID, taskID, sessionID string, ids []string) error {
	attachments, err := s.repo.DeleteClaimedMessageAttachments(ctx, ids, ownerID, taskID, sessionID)
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
	attachments, err := s.repo.DeleteMessageAttachmentsByTask(ctx, taskID)
	if err != nil {
		return err
	}
	for _, attachment := range attachments {
		s.removeBytes(attachment)
	}
	return nil
}

func (s *AttachmentService) Descriptor(attachment *models.TaskMessageAttachment) AttachmentDescriptor {
	return AttachmentDescriptor{
		ID: attachment.ID, Name: attachment.Name, MimeType: attachment.MimeType,
		Kind: attachment.Kind, DeliveryMode: attachment.DeliveryMode,
		SizeBytes: attachment.SizeBytes, State: attachment.State, ExpiresAt: attachment.ExpiresAt,
	}
}

// CleanupExpired marks staged descriptors expired and removes their bytes.
func (s *AttachmentService) CleanupExpired(ctx context.Context) (int, error) {
	expired, err := s.repo.MarkExpiredMessageAttachments(ctx, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	for _, attachment := range expired {
		s.removeBytes(attachment)
	}
	return len(expired), nil
}

func (s *AttachmentService) removeBytes(attachment *models.TaskMessageAttachment) {
	if attachment == nil || attachment.StorageKey == "" {
		return
	}
	if err := os.Remove(filepath.Join(s.root, filepath.Base(attachment.StorageKey))); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warn("remove attachment bytes failed", zap.String("attachment_id", attachment.ID), zap.Error(err))
	}
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
