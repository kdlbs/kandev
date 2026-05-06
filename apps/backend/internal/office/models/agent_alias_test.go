package models

import (
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// TestAgentInstance_AliasIsTransparent verifies that office.AgentInstance is a
// type alias for settingsmodels.AgentProfile after ADR 0005 Wave G — the two
// names refer to the same struct, fields are accessible through either, and
// a *AgentProfile can be passed where a *AgentInstance is expected without
// any conversion.
func TestAgentInstance_AliasIsTransparent(t *testing.T) {
	profile := &settingsmodels.AgentProfile{
		ID:          "p-1",
		WorkspaceID: "ws-1",
		Name:        "alias-check",
		Role:        AgentRoleCEO,
		Status:      AgentStatusIdle,
	}

	// Direct assignment without a cast through a function param: only legal
	// because AgentInstance IS settingsmodels.AgentProfile under the alias
	// declaration. The compiler accepting this signature is the assertion.
	instance := acceptInstance(profile)

	if instance.ID != "p-1" {
		t.Errorf("alias lost id: %q", instance.ID)
	}
	if instance.Role != AgentRoleCEO {
		t.Errorf("alias lost typed role: %q", instance.Role)
	}
	if instance.Status != AgentStatusIdle {
		t.Errorf("alias lost typed status: %q", instance.Status)
	}

	// Mutation through the alias is visible on the original pointer.
	instance.Name = "renamed"
	if profile.Name != "renamed" {
		t.Errorf("alias does not share storage: profile.Name=%q", profile.Name)
	}
}

// acceptInstance takes the office-named type. It compiles only because
// AgentInstance is a type alias for settingsmodels.AgentProfile.
func acceptInstance(a *AgentInstance) *AgentInstance { return a }
