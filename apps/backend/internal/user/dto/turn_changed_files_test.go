package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestTurnChangedFilesSettingsDTOAndPatchPreserveFalse(t *testing.T) {
	encoded, err := json.Marshal(FromUserSettings(&models.UserSettings{ShowTurnChangedFiles: false}))
	if err != nil {
		t.Fatalf("marshal settings DTO: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("decode settings DTO: %v", err)
	}
	if got, ok := response["show_turn_changed_files"].(bool); !ok || got {
		t.Fatalf("response show_turn_changed_files = %#v, want false", response["show_turn_changed_files"])
	}

	var omitted UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted patch: %v", err)
	}
	if omitted.ShowTurnChangedFiles != nil {
		t.Fatal("omitted preference patch must remain nil")
	}
	var disabled UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"show_turn_changed_files":false}`), &disabled); err != nil {
		t.Fatalf("decode disabled patch: %v", err)
	}
	if disabled.ShowTurnChangedFiles == nil || *disabled.ShowTurnChangedFiles {
		t.Fatal("explicit false preference patch was not preserved")
	}
}
