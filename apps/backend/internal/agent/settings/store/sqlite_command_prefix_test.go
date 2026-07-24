package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestCommandPrefix_RoundTrip verifies command_prefix survives insert → read →
// update → read, and that a profile saved without one reads back empty.
func TestCommandPrefix_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "claude-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "sandboxed",
		AgentDisplayName: "Claude",
		CommandPrefix:    "greywall --",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.CommandPrefix != "greywall --" {
		t.Fatalf("command_prefix mismatch: got %q", got.CommandPrefix)
	}

	// Update: clear the prefix.
	got.CommandPrefix = ""
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got2, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("re-get profile: %v", err)
	}
	if got2.CommandPrefix != "" {
		t.Errorf("expected cleared command_prefix, got %q", got2.CommandPrefix)
	}
}

// TestCommandPrefix_DefaultsEmpty verifies a profile created without a prefix
// reads back an empty string rather than erroring on the NOT NULL column.
func TestCommandPrefix_DefaultsEmpty(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "copilot-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "copilot-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "default",
		AgentDisplayName: "Copilot",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.CommandPrefix != "" {
		t.Errorf("expected empty command_prefix, got %q", got.CommandPrefix)
	}
}
