package routing_test

import (
	"context"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// agentWithRole builds a resolver-ready agent profile carrying role and an
// optional overrides blob, mirroring agentWithOverrides but adding the Role
// field the widened effectiveTier precedence needs.
func agentWithRole(t *testing.T, role settingsmodels.AgentRole, ov routing.AgentOverrides) settingsmodels.AgentProfile {
	t.Helper()
	agent := agentWithOverrides(t, ov)
	agent.Role = role
	return agent
}

// AC-3/AC-7 style: with no wake reason, no per-agent override, and no
// matching role entry, the workspace default_tier supplies the tier and
// TierSource reports "workspace".
func TestResolve_TierSource_WorkspaceDefault_NoRoleEntry(t *testing.T) {
	cfg := twoProviderCfg()
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != cfg.DefaultTier {
		t.Errorf("RequestedTier = %q, want workspace default %q", res.RequestedTier, cfg.DefaultTier)
	}
	if res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWorkspace)
	}
}

// AC-11 fallthrough at the resolve layer: a role_tiers entry present but
// set to "" behaves identically to the role being entirely absent from
// the map — falls through to the workspace default.
func TestResolve_TierSource_EmptyRoleEntryFallsThroughToWorkspace(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"qa": ""}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != cfg.DefaultTier || res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("got (%q,%q), want (%q,%q)",
			res.RequestedTier, res.TierSource, cfg.DefaultTier, routing.TierSourceWorkspace)
	}
}

// The new level 3: a role_tiers entry matching the agent's role supplies
// the tier, below any per-agent override and above the workspace default.
func TestResolve_TierSource_RoleTierApplies(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"qa": routing.TierEconomy}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != routing.TierEconomy {
		t.Errorf("RequestedTier = %q, want economy", res.RequestedTier)
	}
	if res.TierSource != routing.TierSourceRole {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceRole)
	}
}

// A role_tiers entry for a different role than the agent's own must not
// apply — it falls through to the workspace default exactly as if
// role_tiers were empty.
func TestResolve_TierSource_RoleTierForDifferentRoleDoesNotApply(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"devops": routing.TierEconomy}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != cfg.DefaultTier || res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("got (%q,%q), want workspace default", res.RequestedTier, res.TierSource)
	}
}

// Level 2 beats level 3: a per-agent tier override outranks the agent's
// role tier.
func TestResolve_TierSource_AgentOverrideBeatsRoleTier(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"qa": routing.TierEconomy}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	ov := routing.AgentOverrides{TierSource: routing.TierSourceOverride, Tier: routing.TierFrontier}
	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, ov),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != routing.TierFrontier {
		t.Errorf("RequestedTier = %q, want frontier (agent override)", res.RequestedTier)
	}
	if res.TierSource != routing.TierSourceOverride {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceOverride)
	}
}

// Level 1 beats everything: a matching wake-reason policy outranks both
// the per-agent override and the role tier.
func TestResolve_TierSource_WakeReasonBeatsOverrideAndRoleTier(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.TierPerReason = routing.TierPerReason{routing.WakeReasonHeartbeat: routing.TierEconomy}
	cfg.RoleTiers = routing.RoleTierMap{"qa": routing.TierFrontier}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	ov := routing.AgentOverrides{TierSource: routing.TierSourceOverride, Tier: routing.TierBalanced}
	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, ov),
		routing.ResolveOptions{Reason: routing.WakeReasonHeartbeat})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != routing.TierEconomy {
		t.Errorf("RequestedTier = %q, want economy (wake reason)", res.RequestedTier)
	}
	if res.TierSource != routing.TierSourceWakeReason {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWakeReason)
	}
}

// AC-34: an agent whose role is outside the seven-value enum (a row
// written by an older build or by hand) must never consult role_tiers,
// even when the persisted map happens to hold a matching non-enum key
// (AC-34b permits such a key to exist on the read path). Resolution
// falls through to the workspace default exactly as if role_tiers were
// empty for this agent.
func TestResolve_TierSource_NonEnumRoleIgnoresMatchingRoleTiersKey(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"legacy-role": routing.TierFrontier}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRole("legacy-role"), routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != cfg.DefaultTier {
		t.Errorf("RequestedTier = %q, want workspace default %q (non-enum role must not match role_tiers)",
			res.RequestedTier, cfg.DefaultTier)
	}
	if res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWorkspace)
	}
}

// AC-6: an agent whose role is the empty string must never consult
// role_tiers at all, regardless of the map's contents. This is the
// strongest form of the guard: role_tiers holds an entry keyed by the
// empty string (unreachable via the write path, since AC-8 restricts
// keys to the seven-value enum, but a resolver read-path lookup does not
// itself know that) to prove the `role != ""` gate in effectiveTier, not
// merely the absence of an empty key in a realistic fixture.
func TestResolve_TierSource_EmptyRoleNeverConsultsRoleTiers(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.RoleTiers = routing.RoleTierMap{"": routing.TierFrontier}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRole(""), routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.RequestedTier != cfg.DefaultTier {
		t.Errorf("RequestedTier = %q, want workspace default %q (empty role must not match role_tiers)",
			res.RequestedTier, cfg.DefaultTier)
	}
	if res.TierSource != routing.TierSourceWorkspace {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceWorkspace)
	}
}

// AC-12a: a disabled workspace with a non-empty provider order still
// resolves the role tier — Enabled only gates automatic fallback, not
// tier resolution. This is the branch that does NOT hit Resolve's
// "disabled and empty order" early return.
func TestResolve_TierSource_RoleTierAppliesWhenDisabled(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.Enabled = false
	cfg.RoleTiers = routing.RoleTierMap{"qa": routing.TierEconomy}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)

	res, err := r.Resolve(context.Background(), wsID,
		agentWithRole(t, settingsmodels.AgentRoleQA, routing.AgentOverrides{}),
		routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Enabled {
		t.Fatalf("expected Enabled=false to be preserved")
	}
	if res.RequestedTier != routing.TierEconomy {
		t.Errorf("RequestedTier = %q, want economy even though disabled", res.RequestedTier)
	}
	if res.TierSource != routing.TierSourceRole {
		t.Errorf("TierSource = %q, want %q", res.TierSource, routing.TierSourceRole)
	}
}
