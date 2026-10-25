package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
)

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.3
func TestPublishThreadsStartupPage(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{StartupPage: "threads"})
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("published %d settings events, want 1", len(eventBus.publishedEvents))
	}
	data, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok || data["startup_page"] != "threads" {
		t.Fatalf("settings event = %#v, want Threads startup page", data)
	}
}

func TestThreadsStartupPagePatch(t *testing.T) {
	settings := &models.UserSettings{StartupPage: "threads"}
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ChangesPanelLayout: ptr("flat")}); err != nil {
		t.Fatal(err)
	}
	if settings.StartupPage != "threads" {
		t.Fatalf("unrelated patch changed startup page to %q", settings.StartupPage)
	}
	if err := applyStartupPage(settings, ptr("invalid")); err == nil || settings.StartupPage != "threads" {
		t.Fatalf("invalid patch: startup page = %q, error = %v", settings.StartupPage, err)
	}
	if err := applyStartupPage(settings, ptr(" threads ")); err != nil || settings.StartupPage != "threads" {
		t.Fatalf("trimmed patch: startup page = %q, error = %v", settings.StartupPage, err)
	}
}
