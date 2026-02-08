package service

import (
	"context"
	"errors"
	"strings"
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
	WorkspaceID            *string
	BoardID                *string
	RepositoryIDs          *[]string
	InitialSetupComplete   *bool
	PreferredShell         *string
	DefaultEditorID        *string
	EnablePreviewOnClick   *bool
	ChatSubmitKey          *string
	ReviewAutoMarkOnScroll *bool
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

func (s *Service) PreferredShell(ctx context.Context) (string, error) {
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return "", err
	}
	return settings.PreferredShell, nil
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
	if req.InitialSetupComplete != nil {
		settings.InitialSetupComplete = *req.InitialSetupComplete
	}
	if req.PreferredShell != nil {
		settings.PreferredShell = strings.TrimSpace(*req.PreferredShell)
	}
	if req.DefaultEditorID != nil {
		settings.DefaultEditorID = strings.TrimSpace(*req.DefaultEditorID)
	}
	if req.EnablePreviewOnClick != nil {
		settings.EnablePreviewOnClick = *req.EnablePreviewOnClick
	}
	if req.ChatSubmitKey != nil {
		key := strings.TrimSpace(*req.ChatSubmitKey)
		if key != "enter" && key != "cmd_enter" {
			return nil, errors.New("chat_submit_key must be 'enter' or 'cmd_enter'")
		}
		s.logger.Info("[Settings] Setting ChatSubmitKey", zap.String("value", key))
		settings.ChatSubmitKey = key
	}
	if req.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *req.ReviewAutoMarkOnScroll
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
		"user_id":                settings.UserID,
		"workspace_id":           settings.WorkspaceID,
		"board_id":               settings.BoardID,
		"repository_ids":         settings.RepositoryIDs,
		"initial_setup_complete": settings.InitialSetupComplete,
		"preferred_shell":        settings.PreferredShell,
		"default_editor_id":      settings.DefaultEditorID,
		"enable_preview_on_click":    settings.EnablePreviewOnClick,
		"chat_submit_key":            settings.ChatSubmitKey,
		"review_auto_mark_on_scroll": settings.ReviewAutoMarkOnScroll,
		"updated_at":                 settings.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, events.UserSettingsUpdated, bus.NewEvent(events.UserSettingsUpdated, "user-service", data)); err != nil {
		s.logger.Error("failed to publish user settings event", zap.Error(err))
	}
}

func (s *Service) ClearDefaultEditorID(ctx context.Context, editorID string) error {
	if editorID == "" {
		return nil
	}
	settings, err := s.repo.GetUserSettings(ctx, s.defaultUser)
	if err != nil {
		return err
	}
	if settings.DefaultEditorID != editorID {
		return nil
	}
	settings.DefaultEditorID = ""
	settings.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpsertUserSettings(ctx, settings); err != nil {
		return err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return nil
}
