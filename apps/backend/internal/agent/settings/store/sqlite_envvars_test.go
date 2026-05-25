package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestAgentProfileEnvVars_Roundtrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "claude-acp-env-test"}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "claude-acp-env-test")
	if err != nil {
		t.Fatalf("GetAgentByName: %v", err)
	}

	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "with-env",
		AgentDisplayName: "Claude",
		Model:            "default",
		EnvVars: []models.ProfileEnvVar{
			{Key: "ANTHROPIC_BASE_URL", Value: "https://example.test"},
			{Key: "MY_TOKEN", SecretID: "sec-abc"},
		},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("CreateAgentProfile: %v", err)
	}

	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if len(got.EnvVars) != 2 {
		t.Fatalf("env_vars len: got %d", len(got.EnvVars))
	}
	if got.EnvVars[0].Key != "ANTHROPIC_BASE_URL" || got.EnvVars[0].Value != "https://example.test" {
		t.Fatalf("unexpected first env var: %+v", got.EnvVars[0])
	}
	if got.EnvVars[1].SecretID != "sec-abc" {
		t.Fatalf("unexpected secret env var: %+v", got.EnvVars[1])
	}

	got.EnvVars = []models.ProfileEnvVar{{Key: "ONLY", Value: "one"}}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("UpdateAgentProfile: %v", err)
	}
	updated, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetAgentProfile after update: %v", err)
	}
	if len(updated.EnvVars) != 1 || updated.EnvVars[0].Key != "ONLY" {
		t.Fatalf("after update: %+v", updated.EnvVars)
	}
}
