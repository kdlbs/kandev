package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestProviderColumns_RoundTrip verifies the OpenAI-compatible provider fields
// survive insert → read → update → read and that a profile saved without them
// reads back empty (native).
func TestProviderColumns_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "codex-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "codex-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:                agent.ID,
		Name:                   "9router",
		AgentDisplayName:       "Codex",
		Model:                  "code",
		ProviderKind:           models.ProviderKindOpenAICompatible,
		ProviderBaseURL:        "http://localhost:20128/v1",
		ProviderAPIKeySecretID: "secret-123",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.ProviderKind != models.ProviderKindOpenAICompatible ||
		got.ProviderBaseURL != "http://localhost:20128/v1" ||
		got.ProviderAPIKeySecretID != "secret-123" {
		t.Fatalf("provider fields mismatch: %+v", got)
	}

	got.ProviderKind = models.ProviderKindNative
	got.ProviderBaseURL = ""
	got.ProviderAPIKeySecretID = ""
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got2, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("re-get profile: %v", err)
	}
	if got2.ProviderKind != "" || got2.ProviderBaseURL != "" || got2.ProviderAPIKeySecretID != "" {
		t.Errorf("expected cleared provider fields, got %+v", got2)
	}
}

// TestProviderColumns_DefaultEmpty verifies a profile created without provider
// fields reads back empty rather than erroring on the NOT NULL columns.
func TestProviderColumns_DefaultEmpty(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{Name: "claude-acp"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "claude-acp")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "default", AgentDisplayName: "Claude"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.ProviderKind != "" || got.ProviderBaseURL != "" || got.ProviderAPIKeySecretID != "" {
		t.Errorf("expected empty provider fields, got %+v", got)
	}
}
