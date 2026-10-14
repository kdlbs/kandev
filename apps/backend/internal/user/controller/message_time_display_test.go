package controller

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
)

func TestUpdateUserSettingsMapsMessageTimeDisplay(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo := &settingsRepository{settings: &models.UserSettings{MessageTimeDisplay: models.MessageTimeDisplayRelative}}
	controller := NewController(service.NewService(repo, nil, log))
	want := models.MessageTimeDisplayAbsoluteLong
	response, err := controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		MessageTimeDisplay: &want,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if response.Settings.MessageTimeDisplay != want {
		t.Fatalf("MessageTimeDisplay = %q, want %q", response.Settings.MessageTimeDisplay, want)
	}
}
