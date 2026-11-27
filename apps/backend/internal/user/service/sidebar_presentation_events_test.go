package service

import (
	"context"
	"github.com/kandev/kandev/internal/user/models"
	"testing"
)

func TestSidebarPresentationEventIncludesExplicitFalse(t *testing.T) {
	bus := &recordingEventBus{}
	svc := &Service{eventBus: bus}
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{SidebarFastActionsEnabled: false, SidebarNewTaskStyle: "compact"})
	if len(bus.publishedEvents) != 1 {
		t.Fatal("missing settings event")
	}
	data := bus.publishedEvents[0].Data.(map[string]interface{})
	if data["sidebar_fast_actions_enabled"] != false || data["sidebar_new_task_style"] != "compact" {
		t.Fatalf("event: %v", data)
	}
}
