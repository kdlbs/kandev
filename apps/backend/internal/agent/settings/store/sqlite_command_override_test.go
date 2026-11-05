package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestCommandOverride_RoundTrip verifies command_override survives insert, read,
// update and read, and that a profile saved without one reads back empty.
func TestCommandOverride_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "codex-app-server"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "codex-app-server")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	plain := &models.AgentProfile{AgentID: agent.ID, Name: "plain", AgentDisplayName: "Codex"}
	if err := repo.CreateAgentProfile(ctx, plain); err != nil {
		t.Fatalf("create plain profile: %v", err)
	}
	if got, err := repo.GetAgentProfile(ctx, plain.ID); err != nil || got.CommandOverride != "" {
		t.Fatalf("plain override = %q, %v; want empty", got.CommandOverride, err)
	}

	profile := &models.AgentProfile{
		AgentID: agent.ID, Name: "pinned", AgentDisplayName: "Codex",
		CommandOverride: "/opt/pinned/bin/codex",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil || got.CommandOverride != "/opt/pinned/bin/codex" {
		t.Fatalf("override = %q, %v", got.CommandOverride, err)
	}
	got.CommandOverride = ""
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if again, err := repo.GetAgentProfile(ctx, profile.ID); err != nil || again.CommandOverride != "" {
		t.Fatalf("cleared override = %q, %v", again.CommandOverride, err)
	}
}
