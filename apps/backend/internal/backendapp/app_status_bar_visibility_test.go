package backendapp

import (
	"encoding/json"
	"testing"

	userdto "github.com/kandev/kandev/internal/user/dto"
)

func TestMapUserSettingsStateIncludesAppStatusBarVisibility(t *testing.T) {
	var response userdto.UserSettingsResponse
	if err := json.Unmarshal([]byte(`{"settings":{"app_status_bar_enabled":false}}`), &response); err != nil {
		t.Fatalf("decode user settings response: %v", err)
	}
	state := mapUserSettingsState(response, "workspace-1")

	got, ok := state["appStatusBarEnabled"].(bool)
	if !ok || got {
		t.Fatalf("appStatusBarEnabled = %#v, want false", state["appStatusBarEnabled"])
	}
}
