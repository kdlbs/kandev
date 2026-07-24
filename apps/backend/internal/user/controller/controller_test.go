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

func TestSystemMetricsDisplayPatch(t *testing.T) {
	t.Run("nil patch stays nil", func(t *testing.T) {
		if got := systemMetricsDisplayPatch(nil); got != nil {
			t.Fatalf("systemMetricsDisplayPatch(nil) = %#v, want nil", got)
		}
	})

	t.Run("explicit values are retained", func(t *testing.T) {
		showInTopbar := true
		simplified := false
		got := systemMetricsDisplayPatch(&dto.SystemMetricsDisplaySettingsPatch{
			ShowInTopbar: &showInTopbar,
			Simplified:   &simplified,
		})
		if got == nil || got.ShowInTopbar == nil || got.Simplified == nil {
			t.Fatalf("systemMetricsDisplayPatch() = %#v, want both values", got)
		}
		if !*got.ShowInTopbar || *got.Simplified {
			t.Fatalf("systemMetricsDisplayPatch() = %#v, want true and false", got)
		}
	})

	t.Run("omitted simplified stays nil", func(t *testing.T) {
		showInTopbar := true
		got := systemMetricsDisplayPatch(&dto.SystemMetricsDisplaySettingsPatch{ShowInTopbar: &showInTopbar})
		if got == nil || got.ShowInTopbar == nil || !*got.ShowInTopbar {
			t.Fatalf("systemMetricsDisplayPatch() = %#v, want show_in_topbar=true", got)
		}
		if got.Simplified != nil {
			t.Fatalf("Simplified = %v, want nil for omitted field", *got.Simplified)
		}
	})
}
