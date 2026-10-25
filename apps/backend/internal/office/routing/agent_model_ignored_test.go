package routing_test

import (
	"context"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// Resolver regression: agent.Role routes tier selection; agent.Model is
// never read by Resolve on a routed launch. See:
// docs/specs/office/requirements/office-agent-tier-routing.md §Decision

// Reproduces the Beta fixture: a specialist agent whose Model field is
// set, no per-agent override, workspace default tier balanced. Resolve
// must land on the workspace-default tier and its mapped model —
// ignoring the agent's own Model value entirely.
func TestResolve_AgentModelFieldNeverSelectsModel(t *testing.T) {
	cfg := twoProviderCfg()
	r := resolverWithLiveTierProfiles(t, cfg)

	agent := agentWithOverrides(t, routing.AgentOverrides{})
	agent.Role = settingsmodels.AgentRoleSpecialist
	agent.Model = "opus[1m]"

	res, err := r.Resolve(context.Background(), wsID, agent,
		routing.ResolveOptions{Reason: "review_started"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != routing.TierBalanced {
		t.Errorf("RequestedTier = %q, want workspace default %q", res.RequestedTier, routing.TierBalanced)
	}
	if res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWorkspace)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(res.Candidates))
	}
	candidate := res.Candidates[0]
	if candidate.ExecutionProfileID != "claude-sonnet-profile" {
		t.Errorf("execution profile = %q, want claude-sonnet-profile", candidate.ExecutionProfileID)
	}
	if candidate.Model != "sonnet-4.5" {
		t.Errorf("candidate model = %q, want live profile model %q", candidate.Model, "sonnet-4.5")
	}
	if candidate.Tier != routing.TierBalanced {
		t.Errorf("candidate tier = %q, want %q", candidate.Tier, routing.TierBalanced)
	}
}

// Sibling case: the same agent (same inert Model="opus[1m]") gets a
// per-agent tier override to frontier. The override — not the agent's
// Model field — is what moves the resolved model to the frontier
// mapping; this is the lever an operator should use instead of setting
// Model directly.
func TestResolve_AgentModelFieldIgnoredEvenWithTierOverride(t *testing.T) {
	cfg := twoProviderCfg()
	r := resolverWithLiveTierProfiles(t, cfg)

	ov := routing.AgentOverrides{TierSource: routing.TierSourceOverride, Tier: routing.TierFrontier}
	agent := agentWithOverrides(t, ov)
	agent.Role = settingsmodels.AgentRoleSpecialist
	agent.Model = "opus[1m]"

	res, err := r.Resolve(context.Background(), wsID, agent,
		routing.ResolveOptions{Reason: "review_started"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != routing.TierFrontier {
		t.Errorf("RequestedTier = %q, want %q", res.RequestedTier, routing.TierFrontier)
	}
	if res.TierSource != routing.TierSourceOverride {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceOverride)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(res.Candidates))
	}
	candidate := res.Candidates[0]
	if candidate.ExecutionProfileID != "claude-opus-profile" {
		t.Errorf("execution profile = %q, want claude-opus-profile", candidate.ExecutionProfileID)
	}
	if candidate.Model != "opus-4.1" {
		t.Errorf("candidate model = %q, want live profile model %q", candidate.Model, "opus-4.1")
	}
	if candidate.Tier != routing.TierFrontier {
		t.Errorf("candidate tier = %q, want %q", candidate.Tier, routing.TierFrontier)
	}
}

func TestResolve_AgentModelFieldIgnoredWhenRoutingDisabledWithProviderOrder(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.Enabled = false
	r := resolverWithLiveTierProfiles(t, cfg)

	agent := agentWithOverrides(t, routing.AgentOverrides{})
	agent.Role = settingsmodels.AgentRoleSpecialist
	agent.Model = "opus[1m]"

	res, err := r.Resolve(context.Background(), wsID, agent,
		routing.ResolveOptions{Reason: "review_started"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Enabled {
		t.Fatal("expected routing to remain disabled")
	}
	if res.RequestedTier != routing.TierBalanced {
		t.Errorf("RequestedTier = %q, want workspace default %q", res.RequestedTier, routing.TierBalanced)
	}
	if res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWorkspace)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(res.Candidates))
	}
	candidate := res.Candidates[0]
	if candidate.ExecutionProfileID != "claude-sonnet-profile" {
		t.Errorf("execution profile = %q, want claude-sonnet-profile", candidate.ExecutionProfileID)
	}
	if candidate.Model != "sonnet-4.5" {
		t.Errorf("candidate model = %q, want live profile model %q", candidate.Model, "sonnet-4.5")
	}
	if candidate.Tier != routing.TierBalanced {
		t.Errorf("candidate tier = %q, want %q", candidate.Tier, routing.TierBalanced)
	}
}

func resolverWithLiveTierProfiles(t *testing.T, cfg *routing.WorkspaceConfig) *routing.Resolver {
	t.Helper()
	cfg.ProviderOrder = []routing.ProviderID{"claude-acp"}
	delete(cfg.ProviderProfiles, "codex-acp")
	cfg.ProviderProfiles["claude-acp"] = routing.ProviderProfile{
		ExecutionProfileIDs: routing.ExecutionProfileIDs{
			Frontier: "claude-opus-profile",
			Balanced: "claude-sonnet-profile",
		},
		TierMap: routing.TierMap{
			Frontier: "stale-opus-model",
			Balanced: "stale-sonnet-model",
		},
	}
	store := &resolverProfileStore{
		agents: map[string]*settingsmodels.Agent{
			"claude-agent": {ID: "claude-agent", Name: "claude-acp"},
		},
		profiles: map[string]*settingsmodels.AgentProfile{
			"claude-opus-profile": {
				ID: "claude-opus-profile", AgentID: "claude-agent", Model: "opus-4.1",
			},
			"claude-sonnet-profile": {
				ID: "claude-sonnet-profile", AgentID: "claude-agent", Model: "sonnet-4.5",
			},
		},
	}
	r := newResolver(t, &fakeRepo{cfg: cfg})
	r.SetExecutionProfileStore(store, nil)
	return r
}
