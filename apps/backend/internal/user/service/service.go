package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/lsp/installer"
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
	WorkspaceID             *string
	KanbanViewMode          *string
	WorkflowFilterID        *string
	RepositoryIDs           *[]string
	InitialSetupComplete    *bool
	PreferredShell          *string
	DefaultEditorID         *string
	EnablePreviewOnClick    *bool
	ChatSubmitKey           *string
	ReviewAutoMarkOnScroll  *bool
	LspAutoStartLanguages   *[]string
	LspAutoInstallLanguages *[]string
	LspServerConfigs        *map[string]map[string]interface{}
	SavedLayouts            *[]models.SavedLayout
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
	if err := applyBasicSettings(settings, req); err != nil {
		return nil, err
	}
	if err := s.applyChatSubmitKey(settings, req); err != nil {
		return nil, err
	}
	if err := applyLSPSettings(settings, req); err != nil {
		return nil, err
	}
	if err := applySavedLayouts(settings, req); err != nil {
		return nil, err
	}
	settings.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpsertUserSettings(ctx, settings); err != nil {
		return nil, err
	}
	s.publishUserSettingsEvent(ctx, settings)
	return settings, nil
}

// applyBasicSettings copies simple (non-validated) fields from req to settings.
func applyBasicSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.WorkspaceID != nil {
		settings.WorkspaceID = *req.WorkspaceID
	}
	if req.KanbanViewMode != nil {
		settings.KanbanViewMode = *req.KanbanViewMode
	}
	if req.WorkflowFilterID != nil {
		settings.WorkflowFilterID = *req.WorkflowFilterID
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
	if req.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *req.ReviewAutoMarkOnScroll
	}
	return nil
}

// applyChatSubmitKey validates and applies the chat_submit_key setting.
func (s *Service) applyChatSubmitKey(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.ChatSubmitKey == nil {
		return nil
	}
	key := strings.TrimSpace(*req.ChatSubmitKey)
	if key != "enter" && key != "cmd_enter" {
		return errors.New("chat_submit_key must be 'enter' or 'cmd_enter'")
	}
	s.logger.Info("[Settings] Setting ChatSubmitKey", zap.String("value", key))
	settings.ChatSubmitKey = key
	return nil
}

// applyLSPSettings validates and applies LSP-related settings.
func applyLSPSettings(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.LspAutoStartLanguages != nil {
		if err := validateLSPLanguages(*req.LspAutoStartLanguages); err != nil {
			return fmt.Errorf("lsp_auto_start_languages: %w", err)
		}
		settings.LspAutoStartLanguages = *req.LspAutoStartLanguages
	}
	if req.LspAutoInstallLanguages != nil {
		if err := validateLSPLanguages(*req.LspAutoInstallLanguages); err != nil {
			return fmt.Errorf("lsp_auto_install_languages: %w", err)
		}
		settings.LspAutoInstallLanguages = *req.LspAutoInstallLanguages
	}
	if req.LspServerConfigs != nil {
		settings.LspServerConfigs = *req.LspServerConfigs
	}
	return nil
}

const maxSavedLayouts = 20

// applySavedLayouts validates and applies the saved_layouts setting.
func applySavedLayouts(settings *models.UserSettings, req *UpdateUserSettingsRequest) error {
	if req.SavedLayouts == nil {
		return nil
	}
	layouts := *req.SavedLayouts
	if len(layouts) > maxSavedLayouts {
		return fmt.Errorf("saved_layouts: max %d layouts allowed", maxSavedLayouts)
	}
	for i := range layouts {
		if strings.TrimSpace(layouts[i].Name) == "" {
			return errors.New("saved_layouts: layout name must not be empty")
		}
	}
	settings.SavedLayouts = layouts
	return nil
}

func (s *Service) publishUserSettingsEvent(ctx context.Context, settings *models.UserSettings) {
	if s.eventBus == nil || settings == nil {
		return
	}
	data := map[string]interface{}{
		"user_id":                    settings.UserID,
		"workspace_id":               settings.WorkspaceID,
		"kanban_view_mode":           settings.KanbanViewMode,
		"workflow_filter_id":         settings.WorkflowFilterID,
		"repository_ids":             settings.RepositoryIDs,
		"initial_setup_complete":     settings.InitialSetupComplete,
		"preferred_shell":            settings.PreferredShell,
		"default_editor_id":          settings.DefaultEditorID,
		"enable_preview_on_click":    settings.EnablePreviewOnClick,
		"chat_submit_key":            settings.ChatSubmitKey,
		"review_auto_mark_on_scroll": settings.ReviewAutoMarkOnScroll,
		"lsp_auto_start_languages":   settings.LspAutoStartLanguages,
		"lsp_auto_install_languages": settings.LspAutoInstallLanguages,
		"lsp_server_configs":         settings.LspServerConfigs,
		"saved_layouts":              settings.SavedLayouts,
		"updated_at":                 settings.UpdatedAt.Format(time.RFC3339),
	}
	if err := s.eventBus.Publish(ctx, events.UserSettingsUpdated, bus.NewEvent(events.UserSettingsUpdated, "user-service", data)); err != nil {
		s.logger.Error("failed to publish user settings event", zap.Error(err))
	}
}

func validateLSPLanguages(langs []string) error {
	supported := installer.SupportedLanguages()
	for _, lang := range langs {
		if _, ok := supported[lang]; !ok {
			return fmt.Errorf("unsupported language: %s", lang)
		}
	}
	return nil
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
