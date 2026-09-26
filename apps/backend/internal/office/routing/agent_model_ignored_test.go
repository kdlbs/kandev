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
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

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
	if len(res.Candidates) == 0 || res.Candidates[0].Model != "sonnet" {
		t.Fatalf("candidate model = %+v, want claude-acp balanced model %q", res.Candidates, "sonnet")
	}
}

// Sibling case: the same agent (same inert Model="opus[1m]") gets a
// per-agent tier override to frontier. The override — not the agent's
// Model field — is what moves the resolved model to the frontier
// mapping; this is the lever an operator should use instead of setting
// Model directly.
func TestResolve_AgentModelFieldIgnoredEvenWithTierOverride(t *testing.T) {
	cfg := twoProviderCfg()
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

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
	if len(res.Candidates) == 0 || res.Candidates[0].Model != "opus" {
		t.Fatalf("candidate model = %+v, want claude-acp frontier model %q (from tier_map, not agent.Model)",
			res.Candidates, "opus")
	}
}
