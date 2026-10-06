package store

import (
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestMessageTimeDisplayDefaultsAndToleratesStoredValues(t *testing.T) {
	if got := defaultUserSettings(DefaultUserID).MessageTimeDisplay; got != models.MessageTimeDisplayRelative {
		t.Fatalf("default display = %q, want %q", got, models.MessageTimeDisplayRelative)
	}
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing", raw: `{}`, want: "relative"},
		{name: "short", raw: `{"message_time_display":"absolute_short"}`, want: "absolute_short"},
		{name: "long", raw: `{"message_time_display":"absolute_long"}`, want: "absolute_long"},
		{name: "unknown", raw: `{"message_time_display":"later"}`, want: "relative"},
		{name: "non-string", raw: `{"message_time_display":false}`, want: "relative"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: test.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.MessageTimeDisplay != test.want {
				t.Fatalf("display = %q, want %q", settings.MessageTimeDisplay, test.want)
			}
		})
	}
}

func TestMessageTimeDisplayPersistsThroughSettingsCodec(t *testing.T) {
	for _, display := range []string{"relative", "absolute_short", "absolute_long"} {
		raw, err := marshalUserSettingsPayload(&models.UserSettings{MessageTimeDisplay: display})
		if err != nil {
			t.Fatalf("marshal %q: %v", display, err)
		}
		settings, err := scanUserSettings(settingsScanner{raw: string(raw)}, DefaultUserID)
		if err != nil {
			t.Fatalf("scan %q: %v", display, err)
		}
		if settings.MessageTimeDisplay != display {
			t.Errorf("round trip display = %q, want %q", settings.MessageTimeDisplay, display)
		}
	}
}
