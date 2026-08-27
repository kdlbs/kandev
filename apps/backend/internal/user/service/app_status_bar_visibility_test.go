package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
)

func TestApplyBasicSettingsAppStatusBarVisibility(t *testing.T) {
	t.Run("omission preserves enabled", func(t *testing.T) {
		settings := decodeStatusBarSettings(t, true)
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if got := statusBarEnabledFromSettings(t, settings); !got {
			t.Fatal("AppStatusBarEnabled = false, want true")
		}
	})

	t.Run("explicit false is applied", func(t *testing.T) {
		settings := decodeStatusBarSettings(t, true)
		disabled := false
		req := UpdateUserSettingsRequest{AppStatusBarEnabled: &disabled}
		if err := applyBasicSettings(settings, &req); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if got := statusBarEnabledFromSettings(t, settings); got {
			t.Fatal("AppStatusBarEnabled = true, want false")
		}
	})
}

func TestPublishUserSettingsEventIncludesAppStatusBarVisibility(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), decodeStatusBarSettings(t, false))

	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got, ok := eventData["app_status_bar_enabled"].(bool); !ok || got {
		t.Fatalf("app_status_bar_enabled = %#v, want false", eventData["app_status_bar_enabled"])
	}
}

func decodeStatusBarSettings(t *testing.T, enabled bool) *models.UserSettings {
	t.Helper()
	var settings models.UserSettings
	raw := `{"app_status_bar_enabled":false}`
	if enabled {
		raw = `{"app_status_bar_enabled":true}`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return &settings
}

func statusBarEnabledFromSettings(t *testing.T, settings *models.UserSettings) bool {
	t.Helper()
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode settings payload: %v", err)
	}
	got, ok := payload["app_status_bar_enabled"].(bool)
	if !ok {
		t.Fatalf("app_status_bar_enabled = %#v, want bool", payload["app_status_bar_enabled"])
	}
	return got
}
