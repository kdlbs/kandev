package service

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

// TestApplyBasicSettingsPromptSuggestions verifies both prompt suggestion flags are preserved when omitted and applied when provided.
func TestApplyBasicSettingsPromptSuggestions(t *testing.T) {
	settings := &models.UserSettings{}
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
		t.Fatalf("apply omitted settings: %v", err)
	}
	if settings.PromptSuggestions || settings.PromptSuggestionsFallback {
		t.Fatal("prompt suggestion flags changed on omitted patch")
	}

	req := &UpdateUserSettingsRequest{PromptSuggestions: ptr(true), PromptSuggestionsFallback: ptr(true)}
	if err := applyBasicSettings(settings, req); err != nil {
		t.Fatalf("apply enabled settings: %v", err)
	}
	if !settings.PromptSuggestions || !settings.PromptSuggestionsFallback {
		t.Fatal("prompt suggestion flags = false, want true")
	}

	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{PromptSuggestionsFallback: ptr(false)}); err != nil {
		t.Fatalf("apply disabled fallback: %v", err)
	}
	if !settings.PromptSuggestions || settings.PromptSuggestionsFallback {
		t.Fatal("expected main on and fallback off")
	}
}

// TestPromptSuggestionsEnabled verifies the launch-time preference read and
// that a settings read failure leaves suggestions off.
func TestPromptSuggestionsEnabled(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	enabled := NewService(&recordingUserRepository{
		getSettings: &models.UserSettings{UserID: store.DefaultUserID, PromptSuggestions: true},
	}, &recordingEventBus{}, log)
	if !enabled.PromptSuggestionsEnabled(context.Background()) {
		t.Fatal("PromptSuggestionsEnabled = false, want true")
	}
	disabled := NewService(&recordingUserRepository{
		getSettings: &models.UserSettings{UserID: store.DefaultUserID},
	}, &recordingEventBus{}, log)
	if disabled.PromptSuggestionsEnabled(context.Background()) {
		t.Fatal("PromptSuggestionsEnabled = true, want false")
	}
}
