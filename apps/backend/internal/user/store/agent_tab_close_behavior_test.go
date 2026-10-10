package store

import (
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestAgentTabCloseBehaviorRoundTripsThroughUserSettingsPayload(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{
		AgentTabCloseBehavior: models.AgentTabCloseBehaviorHidePanel,
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	settings, err := scanUserSettings(settingsScanner{raw: string(raw)}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan settings: %v", err)
	}
	if settings.AgentTabCloseBehavior != models.AgentTabCloseBehaviorHidePanel {
		t.Fatalf("AgentTabCloseBehavior = %q, want %q", settings.AgentTabCloseBehavior, models.AgentTabCloseBehaviorHidePanel)
	}
}
