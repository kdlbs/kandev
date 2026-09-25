package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestUpdateAgentProfileModelIfEmptyIsConditional(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "custom-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "default",
		AgentDisplayName: "Custom ACP",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	updated, err := repo.UpdateAgentProfileModelIfEmpty(ctx, profile.ID, "probed-model")
	if err != nil {
		t.Fatalf("adopt model: %v", err)
	}
	if !updated {
		t.Fatal("first model adoption returned updated=false")
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get adopted profile: %v", err)
	}
	if got.Model != "probed-model" {
		t.Fatalf("model = %q, want probed-model", got.Model)
	}

	updated, err = repo.UpdateAgentProfileModelIfEmpty(ctx, profile.ID, "other-model")
	if err != nil {
		t.Fatalf("second model adoption: %v", err)
	}
	if updated {
		t.Fatal("second model adoption returned updated=true for a configured model")
	}
	got, err = repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get preserved profile: %v", err)
	}
	if got.Model != "probed-model" {
		t.Fatalf("model = %q after second adoption, want probed-model", got.Model)
	}
}
