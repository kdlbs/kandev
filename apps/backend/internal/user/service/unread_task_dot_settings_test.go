package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
)

func TestUnreadTaskDotHideDelayRejectsOutOfRangeAndPublishes(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []int{-1, models.UnreadTaskDotHideDelayMaxSeconds + 1} {
		bus := &recordingEventBus{}
		repo := &recordingUserRepository{getSettings: &models.UserSettings{UnreadTaskDotHideDelaySeconds: 5}}
		svc := NewService(repo, bus, log)
		if _, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{UnreadTaskDotHideDelaySeconds: &invalid}); err == nil {
			t.Fatalf("accepted delay %d", invalid)
		}
		if repo.preservingInput != nil || len(bus.publishedEvents) != 0 {
			t.Fatalf("delay %d persisted or broadcast", invalid)
		}
	}

	bus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, bus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{UnreadTaskDotHideDelaySeconds: 0})
	data := bus.publishedEvents[0].Data.(map[string]interface{})
	if data["unread_task_dot_hide_delay_seconds"] != 0 {
		t.Fatalf("hide delay event field = %v, want 0", data["unread_task_dot_hide_delay_seconds"])
	}
}
