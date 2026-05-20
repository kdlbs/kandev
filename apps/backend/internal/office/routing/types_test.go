package routing_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
)

var testKnown = []routing.ProviderID{
	"claude-acp", "codex-acp", "opencode-acp", "copilot-acp", "amp-acp",
}

func TestValidateWorkspaceConfig(t *testing.T) {
	cases := []struct {
		name    string
		cfg     routing.WorkspaceConfig
		wantErr string
	}{
		{
			name: "happy_disabled_empty_order",
			cfg: routing.WorkspaceConfig{
				Enabled:     false,
				DefaultTier: routing.TierBalanced,
			},
		},
		{
			name: "happy_enabled_one_provider",
			cfg: routing.WorkspaceConfig{
				Enabled:       true,
				DefaultTier:   routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{"claude-acp"},
				ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
					"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
				},
			},
		},
		{
			name: "duplicate_provider",
			cfg: routing.WorkspaceConfig{
				DefaultTier:   routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{"claude-acp", "claude-acp"},
			},
			wantErr: "duplicate",
		},
		{
			name: "unknown_provider",
			cfg: routing.WorkspaceConfig{
				DefaultTier:   routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{"unknown-cli"},
			},
			wantErr: "unknown",
		},
		{
			name: "invalid_tier",
			cfg: routing.WorkspaceConfig{
				DefaultTier:   routing.Tier("turbo"),
				ProviderOrder: []routing.ProviderID{"claude-acp"},
			},
			wantErr: "invalid tier",
		},
		{
			name: "empty_provider_id",
			cfg: routing.WorkspaceConfig{
				DefaultTier:   routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{""},
			},
			wantErr: "empty provider id",
		},
		{
			name: "oversize_order",
			cfg: routing.WorkspaceConfig{
				DefaultTier: routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{
					"claude-acp", "codex-acp", "opencode-acp",
					"copilot-acp", "amp-acp", "claude-acp",
				},
			},
			wantErr: "max",
		},
		{
			name: "enabled_empty_order",
			cfg: routing.WorkspaceConfig{
				Enabled:     true,
				DefaultTier: routing.TierBalanced,
			},
			wantErr: "no providers",
		},
		{
			name: "enabled_missing_profile",
			cfg: routing.WorkspaceConfig{
				Enabled:       true,
				DefaultTier:   routing.TierBalanced,
				ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
				ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
					"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
				},
			},
			wantErr: "no profile",
		},
		{
			name: "enabled_missing_default_tier_model",
			cfg: routing.WorkspaceConfig{
				Enabled:       true,
				DefaultTier:   routing.TierFrontier,
				ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
				ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
					"claude-acp": {TierMap: routing.TierMap{Frontier: "opus"}},
					"codex-acp":  {TierMap: routing.TierMap{Balanced: "gpt-5.4"}},
				},
			},
			wantErr: "default-tier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := routing.ValidateWorkspaceConfig(tc.cfg, testKnown)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
			var ve *routing.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func TestValidateAgentOverrides(t *testing.T) {
	cases := []struct {
		name    string
		ov      routing.AgentOverrides
		wantErr string
	}{
		{
			name: "zero_blob_ok",
			ov:   routing.AgentOverrides{},
		},
		{
			name: "tier_only_override",
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.TierFrontier,
			},
		},
		{
			name: "single_provider_override_ok",
			ov: routing.AgentOverrides{
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"claude-acp"},
			},
		},
		{
			name: "override_with_empty_order",
			ov: routing.AgentOverrides{
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
			},
			wantErr: "at least one provider",
		},
		{
			name: "override_with_duplicate",
			ov: routing.AgentOverrides{
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"claude-acp", "claude-acp"},
			},
			wantErr: "duplicate",
		},
		{
			name: "override_with_unknown",
			ov: routing.AgentOverrides{
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"fake-cli"},
			},
			wantErr: "unknown",
		},
		{
			name: "tier_override_invalid",
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.Tier("ultra"),
			},
			wantErr: "invalid tier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := routing.ValidateAgentOverrides(tc.ov, testKnown)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestValidateAgentOverridesAgainstWorkspace pins the save-time
// guardrail that rejects a tier override the workspace has not mapped
// on any provider. Without this, the save succeeds and every launch
// immediately blocks with no_provider_in_tier — the user never sees a
// signal that the agent is now permanently broken.
func TestValidateAgentOverridesAgainstWorkspace(t *testing.T) {
	mappedFrontier := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Frontier: "opus", Balanced: "sonnet"}},
			"codex-acp":  {TierMap: routing.TierMap{Balanced: "gpt-5"}},
		},
	}
	balancedOnly := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
			"codex-acp":  {TierMap: routing.TierMap{Balanced: "gpt-5"}},
		},
	}

	cases := []struct {
		name    string
		cfg     *routing.WorkspaceConfig
		ov      routing.AgentOverrides
		wantErr string
	}{
		{
			name: "tier_mapped_on_workspace_order",
			cfg:  mappedFrontier,
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.TierFrontier,
			},
		},
		{
			name: "tier_not_mapped_on_any_provider",
			cfg:  balancedOnly,
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.TierFrontier,
			},
			wantErr: `no provider has tier "frontier" mapped`,
		},
		{
			name: "tier_inherit_skips_check",
			cfg:  balancedOnly,
			ov:   routing.AgentOverrides{},
		},
		{
			name: "tier_mapped_on_override_order",
			cfg:  mappedFrontier,
			ov: routing.AgentOverrides{
				TierSource:          routing.TierSourceOverride,
				Tier:                routing.TierFrontier,
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"claude-acp"},
			},
		},
		{
			name: "tier_not_mapped_on_pinned_override_order",
			cfg:  mappedFrontier,
			ov: routing.AgentOverrides{
				TierSource:          routing.TierSourceOverride,
				Tier:                routing.TierFrontier,
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"codex-acp"},
			},
			wantErr: `no provider has tier "frontier" mapped`,
		},
		{
			name: "nil_cfg_skips_check",
			cfg:  nil,
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.TierFrontier,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := routing.ValidateAgentOverridesAgainstWorkspace(tc.ov, testKnown, tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			var ve *routing.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *routing.ValidationError, got %T", err)
			}
			if ve.Field != "routing.tier" {
				t.Errorf("expected ve.Field=routing.tier, got %q", ve.Field)
			}
		})
	}
}

func TestValidateTierPerReason(t *testing.T) {
	cases := []struct {
		name    string
		cfg     routing.WorkspaceConfig
		wantErr string
	}{
		{
			name: "empty_map_ok",
			cfg: routing.WorkspaceConfig{
				DefaultTier: routing.TierBalanced,
			},
		},
		{
			name: "valid_map_ok",
			cfg: routing.WorkspaceConfig{
				DefaultTier: routing.TierBalanced,
				TierPerReason: routing.TierPerReason{
					routing.WakeReasonHeartbeat:      routing.TierEconomy,
					routing.WakeReasonRoutineTrigger: routing.TierBalanced,
				},
			},
		},
		{
			name: "unknown_reason",
			cfg: routing.WorkspaceConfig{
				DefaultTier: routing.TierBalanced,
				TierPerReason: routing.TierPerReason{
					"task_assigned": routing.TierEconomy,
				},
			},
			wantErr: "unknown wake reason",
		},
		{
			name: "invalid_tier",
			cfg: routing.WorkspaceConfig{
				DefaultTier: routing.TierBalanced,
				TierPerReason: routing.TierPerReason{
					routing.WakeReasonHeartbeat: routing.Tier("turbo"),
				},
			},
			wantErr: "invalid tier",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := routing.ValidateWorkspaceConfig(tc.cfg, testKnown)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestValidateAgentOverridesAgainstWorkspace_TierPerReason pins the
// save-time check for per-reason tier overrides: each tier value must
// be mapped on at least one provider in the effective order, just
// like the existing single-tier override.
func TestValidateAgentOverridesAgainstWorkspace_TierPerReason(t *testing.T) {
	cfg := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
		},
	}
	ov := routing.AgentOverrides{
		TierPerReasonSource: routing.TierPerReasonSourceOverride,
		TierPerReason: routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierFrontier,
		},
	}
	err := routing.ValidateAgentOverridesAgainstWorkspace(ov, testKnown, cfg)
	if err == nil {
		t.Fatal("expected error for unmapped tier")
	}
	if !strings.Contains(err.Error(), "tier_per_reason") {
		t.Fatalf("expected tier_per_reason in error, got %v", err)
	}
}

func TestTierMapModel(t *testing.T) {
	m := routing.TierMap{Frontier: "opus", Balanced: "sonnet", Economy: "haiku"}
	cases := map[routing.Tier]string{
		routing.TierFrontier:  "opus",
		routing.TierBalanced:  "sonnet",
		routing.TierEconomy:   "haiku",
		routing.Tier("bogus"): "",
	}
	for tier, want := range cases {
		if got := m.Model(tier); got != want {
			t.Fatalf("Model(%q) = %q, want %q", tier, got, want)
		}
	}
	if !m.IsConfigured(routing.TierBalanced) {
		t.Fatal("IsConfigured(balanced) = false, want true")
	}
	empty := routing.TierMap{}
	if empty.IsConfigured(routing.TierFrontier) {
		t.Fatal("empty.IsConfigured(frontier) = true, want false")
	}
}

func TestAgentOverridesRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		ov       routing.AgentOverrides
	}{
		{
			name:     "empty_settings",
			settings: "",
			ov: routing.AgentOverrides{
				TierSource: routing.TierSourceOverride,
				Tier:       routing.TierFrontier,
			},
		},
		{
			name:     "preserves_unrelated_keys",
			settings: `{"skills":["one","two"],"other":42}`,
			ov: routing.AgentOverrides{
				ProviderOrderSource: routing.ProviderOrderSourceOverride,
				ProviderOrder:       []routing.ProviderID{"claude-acp"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := routing.WriteAgentOverrides(tc.settings, tc.ov)
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := routing.ReadAgentOverrides(out)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if got.TierSource != tc.ov.TierSource || got.Tier != tc.ov.Tier {
				t.Fatalf("tier mismatch: got %+v want %+v", got, tc.ov)
			}
			if len(got.ProviderOrder) != len(tc.ov.ProviderOrder) {
				t.Fatalf("order length mismatch: got %v want %v", got.ProviderOrder, tc.ov.ProviderOrder)
			}
			if tc.settings != "" && !strings.Contains(out, "skills") {
				t.Fatalf("unrelated keys lost: %s", out)
			}
		})
	}
}

func TestWriteAgentOverridesZeroRemovesKey(t *testing.T) {
	in := `{"routing":{"tier_source":"override","tier":"frontier"},"skills":["a"]}`
	out, err := routing.WriteAgentOverrides(in, routing.AgentOverrides{})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if strings.Contains(out, "routing") {
		t.Fatalf("expected routing key removed, got %s", out)
	}
	if !strings.Contains(out, "skills") {
		t.Fatalf("expected skills preserved, got %s", out)
	}
	// Idempotent: write zero again is a no-op equivalent.
	out2, err := routing.WriteAgentOverrides(out, routing.AgentOverrides{})
	if err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if out2 != out {
		t.Fatalf("zero-write not idempotent: %q vs %q", out, out2)
	}
}

func TestReadAgentOverridesEmptyAndMissingKey(t *testing.T) {
	cases := map[string]string{
		"empty_string":   "",
		"empty_object":   "{}",
		"unrelated_only": `{"skills":["a"]}`,
		"null_routing":   `{"routing":null}`,
	}
	for name, settings := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := routing.ReadAgentOverrides(settings)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !got.IsZero() {
				t.Fatalf("expected zero overrides, got %+v", got)
			}
		})
	}
}

func TestReadAgentOverridesBadJSON(t *testing.T) {
	if _, err := routing.ReadAgentOverrides("{not json"); err == nil {
		t.Fatal("expected error on malformed settings")
	}
}
