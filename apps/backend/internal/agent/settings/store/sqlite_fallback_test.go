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
		AgentID:           agent.ID,
		Name:              "hybrid",
		AgentDisplayName:  "OMP",
		Model:             "claude-sonnet-4-5",
		FallbackModel:     "gpt-5",
		AutoFallback:      false,
		RequireExactModel: true,
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
	if !got.RequireExactModel {
		t.Errorf("require_exact_model mismatch: got false, want true")
	}

	// Update: flip the toggle on and change the fallback.
	got.AutoFallback = true
	got.FallbackModel = "gpt-5.2"
	got.RequireExactModel = false
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
	if got2.RequireExactModel {
		t.Errorf("require_exact_model after explicit clear: got true, want false")
	}

	got2.RequireExactModel = true
	if err := repo.UpdateAgentProfile(ctx, got2); err != nil {
		t.Fatalf("re-enable exact model: %v", err)
	}
	got3, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("re-get profile after re-enable: %v", err)
	}
	if !got3.RequireExactModel {
		t.Errorf("require_exact_model after re-enable: got false, want true")
	}
}

// TestFallbackModel_DefaultsCompatible verifies a profile created without the
// new fields keeps the compatible behavior (empty fallback, both toggles off).
func TestFallbackModel_DefaultsCompatible(t *testing.T) {
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
	if got.RequireExactModel {
		t.Errorf("expected require_exact_model off, got true")
	}
}

// TestFallbackModel_SchemaReplay verifies the fallback_model / auto_fallback
// migration survives a same-DB replay: opening a database that already has
// both columns (an upgraded install) and running initSchema again must be a
// no-op, not an error, and the written data must survive. Per
// apps/backend/AGENTS.md, startup schema changes require fresh-DB plus
// same-DB replay tests.
func TestFallbackModel_SchemaReplay(t *testing.T) {
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
		AgentID:           agent.ID,
		Name:              "hybrid",
		AgentDisplayName:  "OMP",
		Model:             "claude-sonnet-4-5",
		FallbackModel:     "deepseek/deepseek-v4-flash",
		AutoFallback:      true,
		RequireExactModel: true,
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	// Simulate a restart on an upgraded DB: the columns already exist, so the
	// migrate.Apply duplicate-column errors must be swallowed.
	if err := repo.initSchema(); err != nil {
		t.Fatalf("initSchema replay on upgraded DB: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile after replay: %v", err)
	}
	if got.FallbackModel != "deepseek/deepseek-v4-flash" {
		t.Errorf("fallback_model after replay mismatch: got %q", got.FallbackModel)
	}
	if !got.AutoFallback {
		t.Errorf("auto_fallback after replay mismatch: got false, want true")
	}
	if !got.RequireExactModel {
		t.Errorf("require_exact_model after replay mismatch: got false, want true")
	}
}
