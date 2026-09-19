package routing

import (
	"encoding/json"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// agentSettingsWithOverrides marshals ov under the "routing" settings key,
// mirroring the production agent_settings.go shape.
func agentSettingsWithOverrides(t *testing.T, ov AgentOverrides) string {
	t.Helper()
	if ov.IsZero() {
		return ""
	}
	blob, err := json.Marshal(map[string]interface{}{"routing": ov})
	if err != nil {
		t.Fatalf("marshal overrides: %v", err)
	}
	return string(blob)
}

// AC-16/AC-20d: tierSourceForAgent delegates to the same effectiveTier
// producer the resolver uses, so a role_tiers entry reports "role" here
// exactly as it would on a live Resolve call.
func TestTierSourceForAgent_RoleTier(t *testing.T) {
	cfg := &WorkspaceConfig{
		DefaultTier: TierBalanced,
		RoleTiers:   RoleTierMap{"qa": TierEconomy},
	}
	agent := settingsmodels.AgentProfile{Role: settingsmodels.AgentRoleQA}

	if got := tierSourceForAgent(agent, cfg); got != TierSourceRole {
		t.Errorf("tierSourceForAgent = %q, want %q", got, TierSourceRole)
	}
}

// A per-agent override outranks a matching role tier.
func TestTierSourceForAgent_OverrideBeatsRoleTier(t *testing.T) {
	cfg := &WorkspaceConfig{
		DefaultTier: TierBalanced,
		RoleTiers:   RoleTierMap{"qa": TierEconomy},
	}
	ov := AgentOverrides{TierSource: TierSourceOverride, Tier: TierFrontier}
	agent := settingsmodels.AgentProfile{
		Role:     settingsmodels.AgentRoleQA,
		Settings: agentSettingsWithOverrides(t, ov),
	}

	if got := tierSourceForAgent(agent, cfg); got != TierSourceOverride {
		t.Errorf("tierSourceForAgent = %q, want %q", got, TierSourceOverride)
	}
}

// No override, no matching role entry: falls through to the workspace
// default.
func TestTierSourceForAgent_WorkspaceDefault(t *testing.T) {
	cfg := &WorkspaceConfig{DefaultTier: TierBalanced}
	agent := settingsmodels.AgentProfile{Role: settingsmodels.AgentRoleQA}

	if got := tierSourceForAgent(agent, cfg); got != TierSourceWorkspace {
		t.Errorf("tierSourceForAgent = %q, want %q", got, TierSourceWorkspace)
	}
}

// AC-18b: a preview computation never carries wake-reason context.
// tierSourceForAgent always calls effectiveTier with an empty reason, so
// even when the agent's own wake-reason override would shadow the tier
// at runtime, the preview must never report "wake_reason".
func TestTierSourceForAgent_NeverReportsWakeReason(t *testing.T) {
	cfg := &WorkspaceConfig{
		DefaultTier:   TierBalanced,
		TierPerReason: TierPerReason{WakeReasonHeartbeat: TierEconomy},
	}
	ov := AgentOverrides{
		TierPerReasonSource: TierPerReasonSourceOverride,
		TierPerReason:       TierPerReason{WakeReasonHeartbeat: TierFrontier},
	}
	agent := settingsmodels.AgentProfile{
		Role:     settingsmodels.AgentRoleQA,
		Settings: agentSettingsWithOverrides(t, ov),
	}

	got := tierSourceForAgent(agent, cfg)
	if got == TierSourceWakeReason {
		t.Fatalf("tierSourceForAgent reported wake_reason for a reason-less preview")
	}
	if got != TierSourceWorkspace {
		t.Errorf("tierSourceForAgent = %q, want %q", got, TierSourceWorkspace)
	}
}

// A nil cfg (defensive edge case — Provider always constructs a default
// before calling this, but the helper is exported to package-internal
// callers only) must not panic and falls through to the empty-source
// pinned-defensive case in effectiveTier.
func TestTierSourceForAgent_NilConfigDoesNotPanic(t *testing.T) {
	agent := settingsmodels.AgentProfile{Role: settingsmodels.AgentRoleQA}
	if got := tierSourceForAgent(agent, nil); got != "" {
		t.Errorf("tierSourceForAgent(nil cfg) = %q, want empty", got)
	}
}

// A malformed settings blob falls back to zero-value overrides rather
// than erroring the preview (SR-23): the tier source still resolves via
// the normal precedence chain instead of surfacing a fixed sentinel.
func TestTierSourceForAgent_MalformedSettingsFallsBackToZeroOverrides(t *testing.T) {
	cfg := &WorkspaceConfig{DefaultTier: TierBalanced}
	agent := settingsmodels.AgentProfile{
		Role:     settingsmodels.AgentRoleQA,
		Settings: "{not valid json",
	}

	if got := tierSourceForAgent(agent, cfg); got != TierSourceWorkspace {
		t.Errorf("tierSourceForAgent = %q, want %q (fell back to workspace default)", got, TierSourceWorkspace)
	}
}
