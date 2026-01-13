package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

var ErrUserNotFound = errors.New("user not found")

type Service struct {
	repo        store.Repository
	eventBus    bus.EventBus
	logger      *logger.Logger
	defaultUser string
}

type UpdateUserSettingsRequest struct {
	WorkspaceID   *string
	BoardID       *string
	RepositoryIDs *[]string
}

func NewService(repo store.Repository, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		repo:        repo,
		eventBus:    eventBus,
		logger:      log.WithFields(zap.String("component", "user-service")),
		defaultUser: store.DefaultUserID,
	}
}

func (s *Service) GetCurrentUser(ctx context.Context) (*models.User, error) {
	user, err := s.repo.GetUser(ctx, s.defaultUser)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *Service) GetUserSettings(ctx context.Context) (*models.UserSettings, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Service) UpdateUserSettings(ctx context.Context, req *UpdateUserSettingsRequest) (*models.UserSettings, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return nil, err
	}
	if req.WorkspaceID != nil {
		settings.WorkspaceID = *req.WorkspaceID
	}
	if req.BoardID != nil {
		settings.BoardID = *req.BoardID
	}
	if req.RepositoryIDs != nil {
		settings.RepositoryIDs = *req.RepositoryIDs
	}
	settings.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpsertUserSettings(ctx, settings); err != nil {
		return nil, err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return settings, nil
}

func (s *Service) publishUserSettingsEvent(ctx context.Context, settings *models.UserSettings) {
	if s.eventBus == nil || settings == nil {
		return
	}
	data := map[string]interface{}{
		"user_id":        settings.UserID,
		"workspace_id":   settings.WorkspaceID,
		"board_id":       settings.BoardID,
		"repository_ids": settings.RepositoryIDs,
		"updated_at":     settings.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, events.UserSettingsUpdated, bus.NewEvent(events.UserSettingsUpdated, "user-service", data)); err != nil {
		s.logger.Error("failed to publish user settings event", zap.Error(err))
	}
}
