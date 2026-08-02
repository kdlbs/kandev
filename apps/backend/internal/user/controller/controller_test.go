package controller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
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

func TestUpdateUserSettingsMapsStartupPage(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo := &settingsRepository{settings: &models.UserSettings{
		StartupPage: models.StartupPageTaskOverview,
	}}
	controller := NewController(service.NewService(repo, nil, log))
	want := models.StartupPageLastTask

	response, err := controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		StartupPage: &want,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if response.Settings.StartupPage != want {
		t.Fatalf("StartupPage = %q, want %q", response.Settings.StartupPage, want)
	}
}

func TestAzureDevOpsBrowsePreferencesRoundTripThroughSettingsAPI(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("create settings repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	controller := NewController(service.NewService(repo, nil, log))
	preferences := json.RawMessage(`{"workspace-a":{"mode":"board","projectId":"project-a","teamId":"team-a","workItemType":"Bug"}}`)

	var patch dto.UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"azure_devops_browse_preferences":`+string(preferences)+`}`), &patch); err != nil {
		t.Fatalf("decode settings patch: %v", err)
	}
	updated, err := controller.UpdateUserSettings(context.Background(), patch)
	if err != nil {
		t.Fatalf("update user settings: %v", err)
	}
	if string(updated.Settings.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("updated preferences = %s, want %s", updated.Settings.AzureDevOpsBrowsePreferences, preferences)
	}
	readBack, err := controller.GetUserSettings(context.Background())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(readBack.Settings.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("read-back preferences = %s, want %s", readBack.Settings.AzureDevOpsBrowsePreferences, preferences)
	}

	var omitted dto.UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted settings patch: %v", err)
	}
	if _, err := controller.UpdateUserSettings(context.Background(), omitted); err != nil {
		t.Fatalf("apply omitted settings patch: %v", err)
	}
	readBack, err = controller.GetUserSettings(context.Background())
	if err != nil {
		t.Fatalf("read settings after omission: %v", err)
	}
	if string(readBack.Settings.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("omitted preferences = %s, want preserved %s", readBack.Settings.AzureDevOpsBrowsePreferences, preferences)
	}

	var clear dto.UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"azure_devops_browse_preferences":null}`), &clear); err != nil {
		t.Fatalf("decode null settings patch: %v", err)
	}
	if _, err := controller.UpdateUserSettings(context.Background(), clear); err != nil {
		t.Fatalf("apply null settings patch: %v", err)
	}
	readBack, err = controller.GetUserSettings(context.Background())
	if err != nil {
		t.Fatalf("read settings after clear: %v", err)
	}
	if string(readBack.Settings.AzureDevOpsBrowsePreferences) != "null" {
		t.Fatalf("cleared preferences = %s, want JSON null", readBack.Settings.AzureDevOpsBrowsePreferences)
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
