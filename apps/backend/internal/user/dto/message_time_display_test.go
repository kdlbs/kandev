package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestUserSettingsDTONormalizesMessageTimeDisplay(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: "absolute_short", want: "absolute_short"},
		{value: "absolute_long", want: "absolute_long"},
		{value: "future", want: "relative"},
		{value: "", want: "relative"},
	} {
		t.Run(test.value, func(t *testing.T) {
			payload, err := json.Marshal(FromUserSettings(&models.UserSettings{MessageTimeDisplay: test.value}))
			if err != nil {
				t.Fatalf("marshal DTO: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("decode DTO: %v", err)
			}
			if got := decoded["message_time_display"]; got != test.want {
				t.Fatalf("message_time_display = %#v, want %q", got, test.want)
			}
		})
	}
}
