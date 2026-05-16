package routing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// fakeRepo is the routing.Repo stub used by every resolver test. The
// resolver only reads — no need for thread-safety or write tracking.
type fakeRepo struct {
	cfg    *routing.WorkspaceConfig
	cfgErr error

	health    []models.ProviderHealth
	healthErr error
}

func (f *fakeRepo) GetWorkspaceRouting(
	_ context.Context, _ string,
) (*routing.WorkspaceConfig, error) {
	return f.cfg, f.cfgErr
}

func (f *fakeRepo) ListProviderHealth(
	_ context.Context, _ string,
) ([]models.ProviderHealth, error) {
	return f.health, f.healthErr
}

const wsID = "ws-1"

var fixedNow = time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

func fixedClock() func() time.Time {
	return func() time.Time { return fixedNow }
}

func newResolver(t *testing.T, repo routing.Repo) *routing.Resolver {
	t.Helper()
	return routing.NewResolver(repo, fixedClock())
}

// twoProviderCfg is the workspace config most tests reuse: claude-acp
// then codex-acp, both with all three tiers mapped, balanced default.
func twoProviderCfg() *routing.WorkspaceConfig {
	return &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {
				TierMap: routing.TierMap{
					Frontier: "opus",
					Balanced: "sonnet",
					Economy:  "haiku",
				},
				Mode:  "default",
				Flags: []string{"--no-color"},
				Env:   map[string]string{"ANTHROPIC_REGION": "us"},
			},
			"codex-acp": {
				TierMap: routing.TierMap{
					Frontier: "gpt-5.5",
					Balanced: "gpt-5",
					Economy:  "gpt-4.1-mini",
				},
				Mode: "plan",
			},
		},
	}
}

func agentWithOverrides(t *testing.T, ov routing.AgentOverrides) settingsmodels.AgentProfile {
	t.Helper()
	settings := ""
	if !ov.IsZero() {
		blob, err := json.Marshal(map[string]interface{}{"routing": ov})
		if err != nil {
			t.Fatalf("marshal overrides: %v", err)
		}
		settings = string(blob)
	}
	return settingsmodels.AgentProfile{ID: "agent-1", Settings: settings}
}

func TestResolveRoutingDisabled(t *testing.T) {
	repo := &fakeRepo{cfg: &routing.WorkspaceConfig{Enabled: false}}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Enabled {
		t.Fatalf("expected disabled resolution")
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %d", len(res.Candidates))
	}
}

func TestResolveRoutingMissingRow(t *testing.T) {
	// Nil config row → treat as disabled (the sqlite repo returns a default
	// disabled blob, but a fake repo returning nil should not panic).
	r := newResolver(t, &fakeRepo{cfg: nil})
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Enabled {
		t.Fatalf("expected disabled resolution for nil config")
	}
}

func TestResolveInheritedDefaults(t *testing.T) {
	repo := &fakeRepo{cfg: twoProviderCfg()}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Enabled || res.RequestedTier != routing.TierBalanced {
		t.Fatalf("unexpected resolution: %+v", res)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(res.Candidates))
	}
	got := res.Candidates[0]
	if got.ProviderID != "claude-acp" || got.Model != "sonnet" || got.Tier != routing.TierBalanced {
		t.Fatalf("candidate[0] mismatch: %+v", got)
	}
	if got.Mode != "default" || len(got.Flags) != 1 || got.Env["ANTHROPIC_REGION"] != "us" {
		t.Fatalf("candidate[0] provider profile not carried verbatim: %+v", got)
	}
	if res.Candidates[1].ProviderID != "codex-acp" || res.Candidates[1].Model != "gpt-5" {
		t.Fatalf("candidate[1] mismatch: %+v", res.Candidates[1])
	}
}

func TestResolveAgentPinsToSingleProvider(t *testing.T) {
	repo := &fakeRepo{cfg: twoProviderCfg()}
	r := newResolver(t, repo)
	ov := routing.AgentOverrides{
		ProviderOrderSource: routing.ProviderOrderSourceOverride,
		ProviderOrder:       []routing.ProviderID{"codex-acp"},
	}
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, ov), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ProviderID != "codex-acp" {
		t.Fatalf("expected single codex candidate, got %+v", res.Candidates)
	}
	if len(res.ProviderOrder) != 1 || res.ProviderOrder[0] != "codex-acp" {
		t.Fatalf("ProviderOrder should reflect override, got %+v", res.ProviderOrder)
	}
}

func TestResolveAgentTierOverride(t *testing.T) {
	repo := &fakeRepo{cfg: twoProviderCfg()}
	r := newResolver(t, repo)
	ov := routing.AgentOverrides{
		TierSource: routing.TierSourceOverride,
		Tier:       routing.TierFrontier,
	}
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, ov), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequestedTier != routing.TierFrontier {
		t.Fatalf("want frontier, got %s", res.RequestedTier)
	}
	if res.Candidates[0].Model != "opus" || res.Candidates[1].Model != "gpt-5.5" {
		t.Fatalf("frontier models not resolved: %+v", res.Candidates)
	}
}

func TestResolveProviderMissingProfileSkipped(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.ProviderOrder = append(cfg.ProviderOrder, "amp-acp") // no profile
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(res.Candidates))
	}
	if len(res.SkippedDegraded) != 1 || res.SkippedDegraded[0].ProviderID != "amp-acp" ||
		res.SkippedDegraded[0].Reason != routing.SkipReasonMissingModelMapping {
		t.Fatalf("expected amp-acp missing_model_mapping skip, got %+v", res.SkippedDegraded)
	}
}

func TestResolveProfileTierUnmapped(t *testing.T) {
	cfg := twoProviderCfg()
	prof := cfg.ProviderProfiles["codex-acp"]
	prof.TierMap.Balanced = "" // unmap requested tier for codex
	cfg.ProviderProfiles["codex-acp"] = prof
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ProviderID != "claude-acp" {
		t.Fatalf("expected only claude candidate, got %+v", res.Candidates)
	}
	if len(res.SkippedDegraded) != 1 || res.SkippedDegraded[0].Reason != routing.SkipReasonMissingModelMapping {
		t.Fatalf("expected missing_model_mapping skip for codex, got %+v", res.SkippedDegraded)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func degradedRow(provider, scope, scopeVal, code string, retryAt time.Time) models.ProviderHealth {
	return models.ProviderHealth{
		ProviderID: provider,
		Scope:      models.ProviderHealthScope(scope),
		ScopeValue: scopeVal,
		State:      "degraded",
		ErrorCode:  code,
		RetryAt:    timePtr(retryAt),
	}
}

func TestResolveDegradedFutureRetrySkipped(t *testing.T) {
	repo := &fakeRepo{
		cfg: twoProviderCfg(),
		health: []models.ProviderHealth{
			degradedRow("claude-acp", "provider", "", "rate_limited", fixedNow.Add(5*time.Minute)),
		},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ProviderID != "codex-acp" {
		t.Fatalf("expected codex-only candidates, got %+v", res.Candidates)
	}
	if len(res.SkippedDegraded) != 1 || res.SkippedDegraded[0].ProviderID != "claude-acp" {
		t.Fatalf("expected claude skipped_degraded, got %+v", res.SkippedDegraded)
	}
	sc := res.SkippedDegraded[0]
	if sc.Reason != routing.SkipReasonDegraded || !sc.AutoRetry || sc.ErrorCode != "rate_limited" {
		t.Fatalf("skipped meta mismatch: %+v", sc)
	}
}

func TestResolveDegradedPastRetryIsEligible(t *testing.T) {
	repo := &fakeRepo{
		cfg: twoProviderCfg(),
		health: []models.ProviderHealth{
			degradedRow("claude-acp", "provider", "", "rate_limited", fixedNow.Add(-1*time.Minute)),
		},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 2 || res.Candidates[0].ProviderID != "claude-acp" {
		t.Fatalf("expected claude to be eligible (probe-on-next-launch), got %+v", res.Candidates)
	}
	if len(res.SkippedDegraded) != 0 {
		t.Fatalf("expected no skipped, got %+v", res.SkippedDegraded)
	}
}

func TestResolveUserActionBlocked(t *testing.T) {
	cfg := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
		},
	}
	repo := &fakeRepo{
		cfg: cfg,
		health: []models.ProviderHealth{{
			ProviderID: "claude-acp", Scope: "provider", ScopeValue: "",
			State: "user_action_required", ErrorCode: "auth_required",
		}},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected zero candidates, got %+v", res.Candidates)
	}
	if res.BlockReason.Status != routing.StatusBlockedActionRequired {
		t.Fatalf("expected blocked_provider_action_required, got %s", res.BlockReason.Status)
	}
	if !res.BlockReason.EarliestRetry.IsZero() {
		t.Fatalf("expected zero earliest retry for user-action-only, got %v", res.BlockReason.EarliestRetry)
	}
}

func TestResolveAllAutoRetryable(t *testing.T) {
	repo := &fakeRepo{
		cfg: twoProviderCfg(),
		health: []models.ProviderHealth{
			degradedRow("claude-acp", "provider", "", "rate_limited", fixedNow.Add(10*time.Minute)),
			degradedRow("codex-acp", "provider", "", "quota_limited", fixedNow.Add(3*time.Minute)),
		},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %+v", res.Candidates)
	}
	if res.BlockReason.Status != routing.StatusWaitingForCapacity {
		t.Fatalf("want waiting_for_provider_capacity, got %s", res.BlockReason.Status)
	}
	want := fixedNow.Add(3 * time.Minute)
	if !res.BlockReason.EarliestRetry.Equal(want) {
		t.Fatalf("earliest retry want %v, got %v", want, res.BlockReason.EarliestRetry)
	}
}

func TestResolveMixedBlockReason(t *testing.T) {
	repo := &fakeRepo{
		cfg: twoProviderCfg(),
		health: []models.ProviderHealth{
			degradedRow("claude-acp", "provider", "", "rate_limited", fixedNow.Add(7*time.Minute)),
			{
				ProviderID: "codex-acp", Scope: "provider", ScopeValue: "",
				State: "user_action_required", ErrorCode: "auth_required",
			},
		},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected zero candidates, got %+v", res.Candidates)
	}
	if res.BlockReason.Status != routing.StatusWaitingForCapacity {
		t.Fatalf("mixed should still surface waiting_for_provider_capacity, got %s", res.BlockReason.Status)
	}
	var sawAutoRetry, sawUserAction bool
	for _, s := range res.BlockReason.Skipped {
		if s.Reason == routing.SkipReasonDegraded && s.AutoRetry {
			sawAutoRetry = true
		}
		if s.Reason == routing.SkipReasonUserAction {
			sawUserAction = true
		}
	}
	if !sawAutoRetry || !sawUserAction {
		t.Fatalf("expected both skip reasons in BlockReason, got %+v", res.BlockReason.Skipped)
	}
}

func TestResolveExcludeProviders(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.ProviderOrder = append(cfg.ProviderOrder, "opencode-acp")
	cfg.ProviderProfiles["opencode-acp"] = routing.ProviderProfile{
		TierMap: routing.TierMap{Balanced: "oc-large"},
	}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)
	opts := routing.ResolveOptions{ExcludeProviders: []routing.ProviderID{"claude-acp"}}
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("want 2 remaining candidates, got %d", len(res.Candidates))
	}
	if res.Candidates[0].ProviderID != "codex-acp" || res.Candidates[1].ProviderID != "opencode-acp" {
		t.Fatalf("ordering after exclude wrong: %+v", res.Candidates)
	}
}

func TestResolveScopeOrdering(t *testing.T) {
	// Set provider-scope healthy override? No — ListProviderHealth only
	// returns non-healthy rows. To assert provider-level beats tier-level
	// beats model-level we install rows of decreasing priority and assert
	// the highest-priority row wins.
	cases := []struct {
		name      string
		rows      []models.ProviderHealth
		wantCode  string
		wantScope string
	}{
		{
			name: "provider_beats_tier_and_model",
			rows: []models.ProviderHealth{
				degradedRow("claude-acp", "provider", "", "quota_limited", fixedNow.Add(time.Hour)),
				degradedRow("claude-acp", "tier", "balanced", "rate_limited", fixedNow.Add(time.Hour)),
				degradedRow("claude-acp", "model", "sonnet", "model_unavailable", fixedNow.Add(time.Hour)),
			},
			wantCode: "quota_limited", wantScope: "provider",
		},
		{
			name: "tier_beats_model_when_no_provider",
			rows: []models.ProviderHealth{
				degradedRow("claude-acp", "tier", "balanced", "rate_limited", fixedNow.Add(time.Hour)),
				degradedRow("claude-acp", "model", "sonnet", "model_unavailable", fixedNow.Add(time.Hour)),
			},
			wantCode: "rate_limited", wantScope: "tier",
		},
		{
			name: "model_used_when_only_model_present",
			rows: []models.ProviderHealth{
				degradedRow("claude-acp", "model", "sonnet", "model_unavailable", fixedNow.Add(time.Hour)),
			},
			wantCode: "model_unavailable", wantScope: "model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := twoProviderCfg()
			cfg.ProviderOrder = []routing.ProviderID{"claude-acp"} // isolate
			repo := &fakeRepo{cfg: cfg, health: tc.rows}
			r := newResolver(t, repo)
			res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res.Candidates) != 0 {
				t.Fatalf("expected zero candidates, got %+v", res.Candidates)
			}
			if len(res.SkippedDegraded) != 1 {
				t.Fatalf("expected one skip, got %+v", res.SkippedDegraded)
			}
			sc := res.SkippedDegraded[0]
			if sc.ErrorCode != tc.wantCode || sc.Scope != tc.wantScope {
				t.Fatalf("scope ordering wrong: got code=%s scope=%s, want %s/%s",
					sc.ErrorCode, sc.Scope, tc.wantCode, tc.wantScope)
			}
		})
	}
}

// TestResolveWakeReasonTier covers the per-reason tier policy:
// workspace policy wins over the default tier; agent override wins
// over workspace; an unknown reason falls back to the agent's
// effective tier.
func TestResolveWakeReasonTier(t *testing.T) {
	t.Run("workspace_policy_overrides_default_tier", func(t *testing.T) {
		cfg := twoProviderCfg()
		cfg.TierPerReason = routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		}
		repo := &fakeRepo{cfg: cfg}
		r := newResolver(t, repo)
		res, err := r.Resolve(context.Background(), wsID,
			agentWithOverrides(t, routing.AgentOverrides{}),
			routing.ResolveOptions{Reason: routing.WakeReasonHeartbeat})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if res.RequestedTier != routing.TierEconomy {
			t.Fatalf("want economy, got %s", res.RequestedTier)
		}
		if res.Candidates[0].Model != "haiku" {
			t.Fatalf("want haiku, got %s", res.Candidates[0].Model)
		}
	})

	t.Run("agent_override_beats_workspace_policy", func(t *testing.T) {
		cfg := twoProviderCfg()
		cfg.TierPerReason = routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		}
		ov := routing.AgentOverrides{
			TierPerReasonSource: routing.TierPerReasonSourceOverride,
			TierPerReason: routing.TierPerReason{
				routing.WakeReasonHeartbeat: routing.TierFrontier,
			},
		}
		repo := &fakeRepo{cfg: cfg}
		r := newResolver(t, repo)
		res, err := r.Resolve(context.Background(), wsID,
			agentWithOverrides(t, ov),
			routing.ResolveOptions{Reason: routing.WakeReasonHeartbeat})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if res.RequestedTier != routing.TierFrontier {
			t.Fatalf("want frontier, got %s", res.RequestedTier)
		}
	})

	t.Run("reason_without_policy_uses_agent_tier", func(t *testing.T) {
		cfg := twoProviderCfg()
		cfg.TierPerReason = routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		}
		ov := routing.AgentOverrides{
			TierSource: routing.TierSourceOverride,
			Tier:       routing.TierFrontier,
		}
		repo := &fakeRepo{cfg: cfg}
		r := newResolver(t, repo)
		// task_assigned is not in TierPerReason → falls through to
		// the agent's tier override.
		res, err := r.Resolve(context.Background(), wsID,
			agentWithOverrides(t, ov),
			routing.ResolveOptions{Reason: "task_assigned"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if res.RequestedTier != routing.TierFrontier {
			t.Fatalf("want frontier, got %s", res.RequestedTier)
		}
	})

	t.Run("agent_override_with_missing_reason_falls_back", func(t *testing.T) {
		cfg := twoProviderCfg()
		cfg.TierPerReason = routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		}
		ov := routing.AgentOverrides{
			TierPerReasonSource: routing.TierPerReasonSourceOverride,
			TierPerReason:       routing.TierPerReason{
				// Agent overrides the map but does not set heartbeat.
				// Per the spec, an explicit override map replaces the
				// workspace map entirely — so heartbeat falls through
				// to the agent's effective tier (= workspace default
				// when no agent tier override is set).
			},
		}
		repo := &fakeRepo{cfg: cfg}
		r := newResolver(t, repo)
		res, err := r.Resolve(context.Background(), wsID,
			agentWithOverrides(t, ov),
			routing.ResolveOptions{Reason: routing.WakeReasonHeartbeat})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if res.RequestedTier != routing.TierBalanced {
			t.Fatalf("want balanced (workspace default), got %s", res.RequestedTier)
		}
	})

	t.Run("empty_reason_skips_policy", func(t *testing.T) {
		cfg := twoProviderCfg()
		cfg.TierPerReason = routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		}
		repo := &fakeRepo{cfg: cfg}
		r := newResolver(t, repo)
		// No Reason → workspace default tier applies.
		res, err := r.Resolve(context.Background(), wsID,
			agentWithOverrides(t, routing.AgentOverrides{}),
			routing.ResolveOptions{})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if res.RequestedTier != routing.TierBalanced {
			t.Fatalf("want balanced, got %s", res.RequestedTier)
		}
	})
}

func TestResolveEmptyEffectiveOrderErrors(t *testing.T) {
	cfg := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{},
	}
	repo := &fakeRepo{cfg: cfg}
	r := newResolver(t, repo)
	_, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}), routing.ResolveOptions{})
	if !errors.Is(err, routing.ErrEmptyOrder) {
		t.Fatalf("want ErrEmptyOrder, got %v", err)
	}
}
