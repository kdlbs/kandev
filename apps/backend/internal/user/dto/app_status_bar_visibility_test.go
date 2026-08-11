package dto

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestAppStatusBarVisibilityDTOResponseAndPatchSemantics(t *testing.T) {
	t.Run("response preserves false", func(t *testing.T) {
		var settings models.UserSettings
		if err := json.Unmarshal([]byte(`{"app_status_bar_enabled":false}`), &settings); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		encoded, err := json.Marshal(FromUserSettings(&settings))
		if err != nil {
			t.Fatalf("encode response: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(encoded, &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got, ok := payload["app_status_bar_enabled"].(bool); !ok || got {
			t.Fatalf("app_status_bar_enabled = %#v, want false", payload["app_status_bar_enabled"])
		}
	})

	t.Run("patch distinguishes omission from false", func(t *testing.T) {
		var omitted UpdateUserSettingsRequest
		var disabled UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
			t.Fatalf("decode omitted patch: %v", err)
		}
		if err := json.Unmarshal([]byte(`{"app_status_bar_enabled":false}`), &disabled); err != nil {
			t.Fatalf("decode explicit patch: %v", err)
		}

		omittedField := reflect.ValueOf(omitted).FieldByName("AppStatusBarEnabled")
		disabledField := reflect.ValueOf(disabled).FieldByName("AppStatusBarEnabled")
		if !omittedField.IsValid() || !disabledField.IsValid() {
			t.Fatal("AppStatusBarEnabled patch field is missing")
		}
		if !omittedField.IsNil() {
			t.Fatalf("omitted AppStatusBarEnabled = %#v, want nil", omittedField.Interface())
		}
		if disabledField.IsNil() || disabledField.Elem().Bool() {
			t.Fatalf("explicit AppStatusBarEnabled = %#v, want pointer to false", disabledField.Interface())
		}
	})
}
