package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// Document-specific sentinel errors.
var (
	ErrDocumentNotFound     = errors.New("document not found")
	ErrDocumentKeyRequired  = errors.New("document key is required")
	ErrDocumentTaskRequired = errors.New("task_id is required")
	ErrAttachmentTooLarge   = errors.New("attachment exceeds maximum size")
)

const (
	maxAttachmentBytes       = 10 * 1024 * 1024 // 10 MB
	defaultDocCoalesceWindow = 5 * time.Minute
	docTypeAttachment        = "attachment"
)

// docRepo is the minimal repository surface DocumentService depends on.
type docRepo interface {
	repository.DocumentRepository
}

// DocumentService provides task document business logic including CRUD,
// revision management, revert, and attachment handling.
type DocumentService struct {
	repo           docRepo
	logger         *logger.Logger
	coalesceWindow time.Duration
}

// NewDocumentService creates a new DocumentService.
func NewDocumentService(repo docRepo, log *logger.Logger) *DocumentService {
	return &DocumentService{
		repo:           repo,
		logger:         log.WithFields(zap.String("component", "document-service")),
		coalesceWindow: defaultDocCoalesceWindow,
	}
}

// CreateOrUpdateDocument upserts the HEAD document and appends or coalesces a revision.
func (s *DocumentService) CreateOrUpdateDocument(
	ctx context.Context,
	taskID, key, docType, title, content, authorKind, authorName string,
) (*models.TaskDocument, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	docType, title, authorKind, authorName = applyDocDefaults(docType, title, key, authorKind, authorName)

	existing, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	head := buildDocHead(taskID, key, docType, title, content, authorKind, authorName, existing)

	latest, err := s.repo.GetLatestDocumentRevision(ctx, taskID, key)
	if err != nil {
		return nil, fmt.Errorf("get latest revision: %w", err)
	}

	now := time.Now().UTC()
	rev, coalesceID := s.buildDocRevision(taskID, key, title, content, authorKind, authorName, latest, now)

	if err := s.repo.WriteDocumentRevision(ctx, head, rev, coalesceID); err != nil {
		s.logger.Error("write document revision", zap.String("task_id", taskID), zap.String("key", key), zap.Error(err))
		return nil, err
	}

	// Reload HEAD to get DB-set timestamps.
	result, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return nil, fmt.Errorf("reload document: %w", err)
	}
	if result == nil {
		return head, nil
	}
	return result, nil
}

// applyDocDefaults fills in default values for optional document fields.
func applyDocDefaults(docType, title, key, authorKind, authorName string) (string, string, string, string) {
	if docType == "" {
		docType = "custom"
	}
	if title == "" {
		title = key
	}
	if authorKind == "" {
		authorKind = createdByAgent
	}
	if authorName == "" {
		if authorKind == createdByAgent {
			authorName = "Agent"
		} else {
			authorName = "User"
		}
	}
	return docType, title, authorKind, authorName
}

// buildDocHead constructs the HEAD document, copying IDs and attachment fields from existing when present.
func buildDocHead(taskID, key, docType, title, content, authorKind, authorName string, existing *models.TaskDocument) *models.TaskDocument {
	head := &models.TaskDocument{
		TaskID:     taskID,
		Key:        key,
		Type:       docType,
		Title:      title,
		Content:    content,
		AuthorKind: authorKind,
		AuthorName: authorName,
	}
	if existing != nil {
		head.ID = existing.ID
		head.CreatedAt = existing.CreatedAt
		// Preserve attachment fields if not overriding content.
		if existing.Type == docTypeAttachment && docType == docTypeAttachment {
			head.Filename = existing.Filename
			head.MimeType = existing.MimeType
			head.SizeBytes = existing.SizeBytes
			head.DiskPath = existing.DiskPath
		}
	}
	return head
}

// buildDocRevision constructs the revision and returns a coalesceID when the latest revision
// was authored by the same author within the coalesce window.
func (s *DocumentService) buildDocRevision(
	taskID, key, title, content, authorKind, authorName string,
	latest *models.TaskDocumentRevision,
	now time.Time,
) (*models.TaskDocumentRevision, *string) {
	rev := &models.TaskDocumentRevision{
		TaskID:      taskID,
		DocumentKey: key,
		Title:       title,
		Content:     content,
		AuthorKind:  authorKind,
		AuthorName:  authorName,
	}
	if !s.canDocCoalesce(latest, authorKind, authorName, now) {
		return rev, nil
	}
	rev.RevisionNumber = latest.RevisionNumber
	rev.AuthorKind = latest.AuthorKind
	rev.AuthorName = latest.AuthorName
	rev.CreatedAt = latest.CreatedAt
	return rev, &latest.ID
}

// GetDocument retrieves a document HEAD by task ID and key.
func (s *DocumentService) GetDocument(ctx context.Context, taskID, key string) (*models.TaskDocument, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	doc, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrDocumentNotFound
	}
	return doc, nil
}

// DeleteDocument removes a document and all its revisions.
func (s *DocumentService) DeleteDocument(ctx context.Context, taskID, key string) error {
	if taskID == "" {
		return ErrDocumentTaskRequired
	}
	if key == "" {
		return ErrDocumentKeyRequired
	}
	existing, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrDocumentNotFound
	}
	return s.repo.DeleteDocument(ctx, taskID, key)
}

// ListDocuments returns all documents for a task.
func (s *DocumentService) ListDocuments(ctx context.Context, taskID string) ([]*models.TaskDocument, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	return s.repo.ListDocuments(ctx, taskID)
}

// ListRevisions returns revision history for a document, newest-first.
func (s *DocumentService) ListRevisions(ctx context.Context, taskID, key string, limit int) ([]*models.TaskDocumentRevision, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	return s.repo.ListDocumentRevisions(ctx, taskID, key, limit)
}

// RevertDocument creates a new revision whose content mirrors the target revision.
func (s *DocumentService) RevertDocument(ctx context.Context, taskID, key, revisionID string) (*models.TaskDocumentRevision, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	if revisionID == "" {
		return nil, errors.New("revision_id is required")
	}

	target, err := s.repo.GetDocumentRevision(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrDocumentNotFound
	}
	if target.TaskID != taskID || target.DocumentKey != key {
		return nil, errors.New("revision does not belong to the given document")
	}

	existing, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return nil, err
	}
	head := &models.TaskDocument{
		TaskID:     taskID,
		Key:        key,
		Title:      target.Title,
		Content:    target.Content,
		AuthorKind: "user",
		AuthorName: "User",
	}
	if existing != nil {
		head.ID = existing.ID
		head.Type = existing.Type
		head.CreatedAt = existing.CreatedAt
	}

	targetID := target.ID
	rev := &models.TaskDocumentRevision{
		TaskID:             taskID,
		DocumentKey:        key,
		Title:              target.Title,
		Content:            target.Content,
		AuthorKind:         "user",
		AuthorName:         "User",
		RevertOfRevisionID: &targetID,
	}

	if err := s.repo.WriteDocumentRevision(ctx, head, rev, nil); err != nil {
		return nil, err
	}
	return rev, nil
}

// UploadAttachment stores a binary attachment on disk and creates/updates the HEAD document.
// basePath is the root directory for attachment storage (e.g., the workspace data dir).
func (s *DocumentService) UploadAttachment(
	ctx context.Context,
	taskID, key, filename, mimeType string,
	data []byte,
	basePath string,
) (*models.TaskDocument, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	if key == "" {
		return nil, ErrDocumentKeyRequired
	}
	if int64(len(data)) > maxAttachmentBytes {
		return nil, ErrAttachmentTooLarge
	}

	ext := filepath.Ext(filename)
	dir := filepath.Join(basePath, "attachments", taskID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create attachment dir: %w", err)
	}
	diskPath := filepath.Join(dir, key+ext)
	if err := os.WriteFile(diskPath, data, 0o640); err != nil {
		return nil, fmt.Errorf("write attachment file: %w", err)
	}

	existing, err := s.repo.GetDocument(ctx, taskID, key)
	if err != nil {
		return nil, err
	}

	head := &models.TaskDocument{
		TaskID:     taskID,
		Key:        key,
		Type:       docTypeAttachment,
		Title:      filename,
		Content:    "",
		AuthorKind: "user",
		AuthorName: "User",
		Filename:   filename,
		MimeType:   mimeType,
		SizeBytes:  int64(len(data)),
		DiskPath:   diskPath,
	}
	if existing != nil {
		head.ID = existing.ID
		head.CreatedAt = existing.CreatedAt
	}

	// Attachments have no revision history — upsert only.
	if existing == nil {
		if err := s.repo.CreateDocument(ctx, head); err != nil {
			return nil, fmt.Errorf("create attachment document: %w", err)
		}
	} else {
		if err := s.repo.UpdateDocument(ctx, head); err != nil {
			return nil, fmt.Errorf("update attachment document: %w", err)
		}
	}
	return head, nil
}

// DownloadAttachment returns the disk path and document metadata for an attachment.
func (s *DocumentService) DownloadAttachment(ctx context.Context, taskID, key string) (string, *models.TaskDocument, error) {
	doc, err := s.GetDocument(ctx, taskID, key)
	if err != nil {
		return "", nil, err
	}
	if doc.Type != docTypeAttachment {
		return "", nil, errors.New("document is not an attachment")
	}
	if doc.DiskPath == "" {
		return "", nil, errors.New("attachment has no disk path")
	}
	return doc.DiskPath, doc, nil
}

func (s *DocumentService) canDocCoalesce(latest *models.TaskDocumentRevision, authorKind, authorName string, now time.Time) bool {
	if latest == nil {
		return false
	}
	if latest.RevertOfRevisionID != nil {
		return false
	}
	if latest.AuthorKind != authorKind || latest.AuthorName != authorName {
		return false
	}
	if s.coalesceWindow <= 0 {
		return false
	}
	return now.Sub(latest.UpdatedAt) < s.coalesceWindow
}
