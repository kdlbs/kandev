package controller

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
)

type settingsRepository struct {
	settings *models.UserSettings
}

func (r *settingsRepository) GetUser(context.Context, string) (*models.User, error) {
	return nil, errors.New("unexpected GetUser call")
}

func (r *settingsRepository) GetDefaultUser(context.Context) (*models.User, error) {
	return nil, errors.New("unexpected GetDefaultUser call")
}

func (r *settingsRepository) GetUserSettings(context.Context, string) (*models.UserSettings, error) {
	copy := *r.settings
	return &copy, nil
}

func (r *settingsRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	_ context.Context,
	settings *models.UserSettings,
	_ *models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	copy := *settings
	r.settings = &copy
	return &copy, nil
}

func (r *settingsRepository) UpdateTaskCreateLastUsed(context.Context, string, models.TaskCreateLastUsed) (*models.UserSettings, error) {
	return nil, errors.New("unexpected UpdateTaskCreateLastUsed call")
}

func (r *settingsRepository) Close() error { return nil }

func TestUpdateUserSettingsMapsMCPTaskAgentProfileDefault(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo := &settingsRepository{settings: &models.UserSettings{
		MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultCurrentTask,
	}}
	controller := NewController(service.NewService(repo, nil, log))
	want := models.MCPTaskAgentProfileDefaultWorkspaceDefault

	response, err := controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		MCPTaskAgentProfileDefault: &want,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if response.Settings.MCPTaskAgentProfileDefault != want {
		t.Fatalf("MCPTaskAgentProfileDefault = %q, want %q", response.Settings.MCPTaskAgentProfileDefault, want)
	}
}
