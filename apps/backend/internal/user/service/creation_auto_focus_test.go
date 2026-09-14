package service

import (
	"context"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
	"testing"
)

func TestAutoFocusNewTasksPatchAndPublication(t *testing.T) {
	settings := &models.UserSettings{AutoFocusNewTasks: true}
	disabled := false
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{AutoFocusNewTasks: &disabled}); err != nil {
		t.Fatal(err)
	}
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
		t.Fatal(err)
	}
	if settings.AutoFocusNewTasks || dto.FromUserSettings(settings).AutoFocusNewTasks {
		t.Fatal("explicit false was lost")
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	bus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, bus, log)
	svc.publishUserSettingsEvent(context.Background(), settings)
	data := bus.publishedEvents[0].Data.(map[string]interface{})
	if got := data["auto_focus_new_tasks"]; got != false {
		t.Fatalf("event value = %#v, want false", got)
	}
	enabled := true
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{AutoFocusNewTasks: &enabled}); err != nil {
		t.Fatal(err)
	}
	if !settings.AutoFocusNewTasks {
		t.Fatal("explicit true was lost")
	}
}
