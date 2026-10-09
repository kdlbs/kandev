package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestApplyMessageTimeDisplayValidatesAndPreservesOmittedSetting(t *testing.T) {
	for _, display := range []string{
		models.MessageTimeDisplayRelative,
		models.MessageTimeDisplayAbsoluteShort,
		models.MessageTimeDisplayAbsoluteLong,
	} {
		settings := &models.UserSettings{MessageTimeDisplay: models.MessageTimeDisplayRelative}
		if err := applyMessageTimeDisplay(settings, &display); err != nil {
			t.Fatalf("apply %q: %v", display, err)
		}
		if settings.MessageTimeDisplay != display {
			t.Fatalf("display = %q, want %q", settings.MessageTimeDisplay, display)
		}
	}

	unchanged := &models.UserSettings{MessageTimeDisplay: models.MessageTimeDisplayAbsoluteLong}
	if err := applyMessageTimeDisplay(unchanged, nil); err != nil {
		t.Fatalf("omitted patch: %v", err)
	}
	if unchanged.MessageTimeDisplay != models.MessageTimeDisplayAbsoluteLong {
		t.Fatalf("omitted patch changed display to %q", unchanged.MessageTimeDisplay)
	}

	invalid := "unknown"
	if err := applyMessageTimeDisplay(unchanged, &invalid); err == nil {
		t.Fatal("unknown display was accepted")
	}
}

func TestPublishedSettingsSnapshotNormalizesMessageTimeDisplay(t *testing.T) {
	eventBus := &casEventBus{}
	svc := newCASService(&casFakeRepo{row: &models.UserSettings{}}, eventBus)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{MessageTimeDisplay: "unknown"})
	if eventBus.count() != 1 {
		t.Fatalf("published events = %d, want 1", eventBus.count())
	}
	data, ok := eventBus.events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("published data has type %T", eventBus.events[0].Data)
	}
	if got := data["message_time_display"]; got != "relative" {
		t.Fatalf("published display = %#v, want relative", got)
	}
}
