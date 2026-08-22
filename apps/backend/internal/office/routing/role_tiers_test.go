package routing_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
)

// roleTiersCfg returns a minimal enabled config with one provider mapping
// all three tiers, used as the base for role_tiers validation cases.
func roleTiersCfg() routing.WorkspaceConfig {
	return routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {
				ExecutionProfileIDs: routing.ExecutionProfileIDs{
					Frontier: "claude-opus-profile",
					Balanced: "claude-sonnet-profile",
					Economy:  "claude-haiku-profile",
				},
				TierMap: routing.TierMap{
					Frontier: "opus",
					Balanced: "sonnet",
					Economy:  "haiku",
				},
			},
		},
	}
}

// AC-8: an unknown role key is rejected with a per-key ValidationDetail.
func TestValidateWorkspaceConfig_RoleTiers_UnknownRole(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = routing.RoleTierMap{"astronaut": routing.TierEconomy}

	err := routing.ValidateWorkspaceConfig(cfg, testKnown)
	var ve *routing.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if ve.Field != "role_tiers" {
		t.Fatalf("Field = %q, want role_tiers", ve.Field)
	}
	if len(ve.Details) != 1 || ve.Details[0].Field != "astronaut" {
		t.Fatalf("Details = %+v, want one entry for astronaut", ve.Details)
	}
}

// AC-9: an invalid tier value is rejected. The message must not carry
// "default_tier" or the generic "routing config invalid:" prefix that
// validateTier's own Error() produces — those would misattribute the
// field to the workspace default-tier validator instead of role_tiers.
func TestValidateWorkspaceConfig_RoleTiers_InvalidTierValue(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = routing.RoleTierMap{"qa": routing.Tier("nonsense")}

	err := routing.ValidateWorkspaceConfig(cfg, testKnown)
	var ve *routing.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if ve.Field != "role_tiers" {
		t.Fatalf("Field = %q, want role_tiers", ve.Field)
	}
	if len(ve.Details) != 1 || ve.Details[0].Field != "qa" {
		t.Fatalf("Details = %+v, want one entry for qa", ve.Details)
	}
	if strings.Contains(ve.Details[0].Message, "default_tier") {
		t.Fatalf("detail message leaked default_tier field: %q", ve.Details[0].Message)
	}
	full := ve.Error()
	if strings.Contains(full, "routing config invalid: default_tier") {
		t.Fatalf("rendered error misattributes to default_tier: %q", full)
	}
}

// AC-10/AC-10a: a role tier that is valid but not mapped on any provider
// in the workspace order is rejected, accumulating per-role details the
// same way checkTierPerReasonMapped does (not the single-value shape
// checkTierMapped uses for the agent-override tier).
func TestValidateWorkspaceConfig_RoleTiers_UnmappedTier(t *testing.T) {
	cfg := roleTiersCfg()
	// claude-acp has no frontier mapping in this variant.
	prof := cfg.ProviderProfiles["claude-acp"]
	prof.ExecutionProfileIDs.Frontier = ""
	prof.TierMap.Frontier = ""
	cfg.ProviderProfiles["claude-acp"] = prof
	cfg.RoleTiers = routing.RoleTierMap{"ceo": routing.TierFrontier}

	err := routing.ValidateWorkspaceConfig(cfg, testKnown)
	var ve *routing.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if ve.Field != "role_tiers" {
		t.Fatalf("Field = %q, want role_tiers", ve.Field)
	}
	if len(ve.Details) != 1 || ve.Details[0].Field != "ceo" {
		t.Fatalf("Details = %+v, want one entry for ceo", ve.Details)
	}
}

// AC-11: an empty tier value means "absent" and is silently tolerated,
// both by the unknown-role/invalid-value check and the unmapped-tier
// check.
func TestValidateWorkspaceConfig_RoleTiers_EmptyValueIsAbsent(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = routing.RoleTierMap{"worker": ""}

	if err := routing.ValidateWorkspaceConfig(cfg, testKnown); err != nil {
		t.Fatalf("empty role tier value should validate cleanly, got %v", err)
	}
}

// AC-12: role_tiers is validated identically regardless of cfg.Enabled —
// checkRoleTiersMapped is not gated behind the Enabled=true block, unlike
// the provider-order/default-tier checks in validateEnabledRules.
func TestValidateWorkspaceConfig_RoleTiers_ValidatedWhenDisabled(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.Enabled = false
	cfg.RoleTiers = routing.RoleTierMap{"astronaut": routing.TierEconomy}

	err := routing.ValidateWorkspaceConfig(cfg, testKnown)
	var ve *routing.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected role_tiers to be validated while disabled, got %v", err)
	}
	if ve.Field != "role_tiers" {
		t.Fatalf("Field = %q, want role_tiers", ve.Field)
	}
}

// AC-40 test #3: two bad role_tiers entries in a single write must
// produce two Details entries, not collapse into one flat error.
func TestValidateWorkspaceConfig_RoleTiers_TwoBadEntriesAccumulate(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = routing.RoleTierMap{
		"astronaut": routing.TierEconomy,      // unknown role
		"qa":        routing.Tier("nonsense"), // invalid tier value
	}

	err := routing.ValidateWorkspaceConfig(cfg, testKnown)
	var ve *routing.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if len(ve.Details) != 2 {
		t.Fatalf("Details = %+v, want 2 entries", ve.Details)
	}
	fields := map[string]bool{}
	for _, d := range ve.Details {
		fields[d.Field] = true
	}
	if !fields["astronaut"] || !fields["qa"] {
		t.Fatalf("Details fields = %v, want astronaut and qa", fields)
	}
}

// A fully valid role_tiers map (every AllAgentRoles key, every value
// mapped) must pass cleanly.
func TestValidateWorkspaceConfig_RoleTiers_AllRolesValid(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = routing.RoleTierMap{
		"ceo":        routing.TierFrontier,
		"worker":     routing.TierEconomy,
		"specialist": routing.TierBalanced,
		"assistant":  routing.TierEconomy,
		"security":   routing.TierBalanced,
		"qa":         routing.TierEconomy,
		"devops":     routing.TierBalanced,
	}

	if err := routing.ValidateWorkspaceConfig(cfg, testKnown); err != nil {
		t.Fatalf("all-known-roles config should validate cleanly, got %v", err)
	}
}

// A nil/missing role_tiers map means "no role policy" and must never be
// rejected.
func TestValidateWorkspaceConfig_RoleTiers_NilMapIsValid(t *testing.T) {
	cfg := roleTiersCfg()
	cfg.RoleTiers = nil

	if err := routing.ValidateWorkspaceConfig(cfg, testKnown); err != nil {
		t.Fatalf("nil role_tiers should validate cleanly, got %v", err)
	}
}
