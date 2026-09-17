package service

import (
	"context"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
	"testing"
)

func TestSidebarHoverSettingsPublishAndReject(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	bus := &recordingEventBus{}
	repo := &recordingUserRepository{getSettings: &models.UserSettings{SidebarHoverEnabled: true, SidebarHoverDelayMs: 500}}
	svc := NewService(repo, bus, log)
	invalid := 5001
	if _, err = svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{SidebarHoverDelayMs: &invalid}); err == nil {
		t.Fatal("accepted invalid delay")
	}
	if repo.preservingInput != nil || len(bus.publishedEvents) != 0 {
		t.Fatal("invalid delay persisted or broadcast")
	}
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{SidebarHoverEnabled: false, SidebarHoverDelayMs: 0})
	data := bus.publishedEvents[0].Data.(map[string]interface{})
	if data["sidebar_hover_enabled"] != false || data["sidebar_hover_delay_ms"] != 0 {
		t.Fatalf("hover event fields: %v, %v", data["sidebar_hover_enabled"], data["sidebar_hover_delay_ms"])
	}
}
