package share

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Service coordinates snapshot construction, backend upload, and persistence.
type Service struct {
	repo      *Repository
	taskRepo  TaskReader
	backend   Backend
	log       *logger.Logger
	kandevVer string
}

// New constructs a Service. All arguments are required.
func New(repo *Repository, taskRepo TaskReader, backend Backend, log *logger.Logger, kandevVer string) *Service {
	return &Service{
		repo:      repo,
		taskRepo:  taskRepo,
		backend:   backend,
		log:       log,
		kandevVer: kandevVer,
	}
}

// PreviewSnapshot builds and returns the redacted snapshot that would be
// uploaded for the given session, without uploading or persisting anything.
// Returns ErrSessionNotCompleted if the session is not yet completed.
func (s *Service) PreviewSnapshot(ctx context.Context, taskSessionID string) (*Snapshot, error) {
	return BuildSnapshot(ctx, s.taskRepo, taskSessionID, s.kandevVer)
}

// CreateShare builds a snapshot, uploads it via the configured backend, and
// records the row in the repository.
func (s *Service) CreateShare(ctx context.Context, taskSessionID string) (*Share, error) {
	snap, err := BuildSnapshot(ctx, s.taskRepo, taskSessionID, s.kandevVer)
	if err != nil {
		return nil, err
	}
	// Use the same indented marshal as the gist backend so the size we
	// surface to the UI matches the bytes actually uploaded to GitHub.
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	extID, extURL, err := s.backend.Upload(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("upload snapshot: %w", err)
	}
	row := &Share{
		ID:                uuid.New().String(),
		TaskSessionID:     taskSessionID,
		Backend:           s.backend.Name(),
		ExternalID:        extID,
		ExternalURL:       extURL,
		SnapshotSizeBytes: int64(len(body)),
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		// The gist exists on GitHub but we failed to persist the row.
		// Best-effort: try to clean up the orphaned gist; log if that
		// also fails so the user can manually delete it.
		if delErr := s.backend.Delete(ctx, extID); delErr != nil {
			s.logWarn("orphaned gist after row insert failed",
				zap.String("gist_id", extID), zap.Error(delErr))
		}
		return nil, fmt.Errorf("persist share: %w", err)
	}
	return row, nil
}

// RevokeShare deletes the underlying gist and marks the row revoked. A
// 404 from the gist backend (gist already deleted on GitHub) is treated
// as success, logged at INFO, and still marks the row revoked.
func (s *Service) RevokeShare(ctx context.Context, shareID string) error {
	row, err := s.repo.GetByID(ctx, shareID)
	if err != nil {
		return err
	}
	if row.IsRevoked() {
		return nil
	}
	if delErr := s.backend.Delete(ctx, row.ExternalID); delErr != nil {
		if !IsAlreadyGone(delErr) {
			return fmt.Errorf("delete from backend: %w", delErr)
		}
		s.logInfo("gist already gone on upstream; marking share revoked anyway",
			zap.String("share_id", shareID),
			zap.String("external_id", row.ExternalID))
	}
	if err := s.repo.MarkRevoked(ctx, shareID, time.Now().UTC()); err != nil {
		// Repository failure after a successful upstream delete leaves an
		// inconsistent row. Surface the error so the caller can retry.
		return fmt.Errorf("mark revoked: %w", err)
	}
	return nil
}

// ListBySession returns every share row for a session (including revoked).
func (s *Service) ListBySession(ctx context.Context, taskSessionID string) ([]*Share, error) {
	return s.repo.ListByTaskSession(ctx, taskSessionID)
}

// GetByID exposes repo.GetByID for handlers that need to inspect a row.
func (s *Service) GetByID(ctx context.Context, shareID string) (*Share, error) {
	return s.repo.GetByID(ctx, shareID)
}

func (s *Service) logInfo(msg string, fields ...zap.Field) {
	if s.log == nil {
		return
	}
	s.log.Info(msg, fields...)
}

func (s *Service) logWarn(msg string, fields ...zap.Field) {
	if s.log == nil {
		return
	}
	s.log.Warn(msg, fields...)
}
