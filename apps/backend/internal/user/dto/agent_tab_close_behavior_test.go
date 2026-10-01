package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestFromUserSettingsDefaultsAgentTabCloseBehaviorToDeleteSession(t *testing.T) {
	payload, err := json.Marshal(FromUserSettings(&models.UserSettings{}))
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if got["agent_tab_close_behavior"] != "delete_session" {
		t.Fatalf("agent_tab_close_behavior = %#v, want delete_session", got["agent_tab_close_behavior"])
	}
}
