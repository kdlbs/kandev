package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestFallbackModel_RoundTrip verifies fallback_model and auto_fallback
// survive insert → read → update → read.
func TestFallbackModel_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "omp-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "omp-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "hybrid",
		AgentDisplayName: "OMP",
		Model:            "claude-sonnet-4-5",
		FallbackModel:    "gpt-5",
		AutoFallback:     false,
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.FallbackModel != "gpt-5" {
		t.Errorf("fallback_model mismatch: got %q, want %q", got.FallbackModel, "gpt-5")
	}
	if got.AutoFallback {
		t.Errorf("auto_fallback mismatch: got true, want false")
	}

	// Update: flip the toggle on and change the fallback.
	got.AutoFallback = true
	got.FallbackModel = "gpt-5.2"
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got2, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("re-get profile: %v", err)
	}
	if got2.FallbackModel != "gpt-5.2" {
		t.Errorf("fallback_model after update mismatch: got %q", got2.FallbackModel)
	}
	if !got2.AutoFallback {
		t.Errorf("auto_fallback after update mismatch: got false, want true")
	}
}

// TestFallbackModel_DefaultsStrict verifies a profile created without the new
// fields reads back strict mode (empty fallback, toggle off) — the safe
// default for existing rows.
func TestFallbackModel_DefaultsStrict(t *testing.T) {
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
		Name:             "default",
		AgentDisplayName: "Claude",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.FallbackModel != "" {
		t.Errorf("expected empty fallback_model, got %q", got.FallbackModel)
	}
	if got.AutoFallback {
		t.Errorf("expected auto_fallback off, got true")
	}
}
