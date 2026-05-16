// Package routing provides typed routing config for the office provider-
// routing feature: tiers, providers, workspace + agent overrides, and the
// validators that the HTTP/repo layers call before persisting changes.
//
// The package has no behavior of its own in Phase 0 — it ships the types
// and validators that downstream phases (resolver, scheduler, classifier)
// will consume.
package routing

import (
	"errors"
	"fmt"
	"strings"
)

// Tier is a normalized model strength bucket. Workspace and agent
// routing config both reference tiers rather than concrete model IDs so
// switching tiers does not require rewriting every per-provider mapping.
type Tier string

const (
	TierFrontier Tier = "frontier"
	TierBalanced Tier = "balanced"
	TierEconomy  Tier = "economy"
)

// AllTiers enumerates valid Tier values in display order.
var AllTiers = []Tier{TierFrontier, TierBalanced, TierEconomy}

// ProviderOrderSourceOverride is the magic value AgentOverrides uses to
// signal that the override blob's ProviderOrder replaces the workspace
// order entirely (rather than inheriting it).
const ProviderOrderSourceOverride = "override"

// TierSourceOverride is the magic value AgentOverrides uses to signal
// that the override blob's Tier replaces the workspace default.
const TierSourceOverride = "override"

// TierPerReasonSourceOverride is the magic value AgentOverrides uses to
// signal that its TierPerReason map replaces the workspace policy
// entirely. Any other value (default "") inherits the workspace map.
const TierPerReasonSourceOverride = "override"

// Wake-reason keys recognised by TierPerReason. These mirror the
// scheduler's RunReason* constants but are declared here so the routing
// package validates them without importing scheduler (which would create
// an import cycle). The string values must stay in sync with the
// scheduler constants.
const (
	WakeReasonHeartbeat      = "heartbeat"
	WakeReasonRoutineTrigger = "routine_trigger"
	WakeReasonBudgetAlert    = "budget_alert"
)

// AllWakeReasons enumerates the keys TierPerReason maps may carry in v1.
// Anything outside this set is rejected by the validator.
var AllWakeReasons = []string{
	WakeReasonHeartbeat,
	WakeReasonRoutineTrigger,
	WakeReasonBudgetAlert,
}

// TierPerReason maps a wake reason to the tier that should run for it.
// Empty / missing keys mean "no special policy — use the agent's
// effective tier." Stored as JSON on the workspace routing row and
// the agent overrides blob.
type TierPerReason map[string]Tier

// ProviderID identifies a CLI provider (the agent type ID from the
// agent/registry package). First-class IDs in v1 are restricted by
// catalogue.KnownProviders.
type ProviderID string

// TierMap maps each tier to a model ID for one provider. An empty
// value means the tier is unmapped for this provider — the resolver
// treats that as "skip this provider for this tier."
type TierMap struct {
	Frontier string `json:"frontier,omitempty"`
	Balanced string `json:"balanced,omitempty"`
	Economy  string `json:"economy,omitempty"`
}

// Model returns the configured model for tier t, or "" if unset.
func (m TierMap) Model(t Tier) string {
	switch t {
	case TierFrontier:
		return m.Frontier
	case TierBalanced:
		return m.Balanced
	case TierEconomy:
		return m.Economy
	}
	return ""
}

// IsConfigured reports whether tier t has a model assigned.
func (m TierMap) IsConfigured(t Tier) bool { return m.Model(t) != "" }

// ProviderProfile holds the per-provider, per-workspace settings needed
// to launch via that provider. Cross-provider routing means CLI mode,
// flags, and env are provider-scoped — they cannot safely be inherited
// from a base AgentProfile authored against a different provider — so
// they live here alongside the tier→model map.
//
// Note: there is no provider-level permission preset in v1. The base
// AgentProfile's CLIFlags (resolved by cliflags.Resolve) carry every
// permission knob the agent runtime consults at launch time. Adding a
// per-provider permission override would require modelling the existing
// boolean/CLIFlag system as named presets first — out of scope here.
type ProviderProfile struct {
	TierMap TierMap           `json:"tier_map"`
	Mode    string            `json:"mode,omitempty"`
	Flags   []string          `json:"flags,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// WorkspaceConfig is the persisted routing config for one workspace.
// When Enabled is false, downstream code uses the concrete agent
// profile and ignores ProviderOrder / DefaultTier / ProviderProfiles.
type WorkspaceConfig struct {
	Enabled          bool                           `json:"enabled"`
	ProviderOrder    []ProviderID                   `json:"provider_order"`
	DefaultTier      Tier                           `json:"default_tier"`
	ProviderProfiles map[ProviderID]ProviderProfile `json:"provider_profiles"`
	// TierPerReason maps wake reasons (heartbeat, routine_trigger,
	// budget_alert) onto specific tiers so the workspace can cheap-out
	// predictable background work without touching agent profiles. An
	// empty / missing map means "no special policy — every reason uses
	// the agent's effective tier."
	TierPerReason TierPerReason `json:"tier_per_reason,omitempty"`
}

// AgentOverrides is the routing override blob stored on
// AgentProfile.Settings under top-level key "routing". Source fields
// ("inherit" by default, "override" to replace) keep the inheritance
// model explicit so the UI does not have to guess.
type AgentOverrides struct {
	ProviderOrderSource string       `json:"provider_order_source,omitempty"`
	ProviderOrder       []ProviderID `json:"provider_order,omitempty"`
	TierSource          string       `json:"tier_source,omitempty"`
	Tier                Tier         `json:"tier,omitempty"`
	// TierPerReasonSource is "override" when the agent's TierPerReason
	// map replaces the workspace map entirely; any other value (default
	// "") inherits.
	TierPerReasonSource string        `json:"tier_per_reason_source,omitempty"`
	TierPerReason       TierPerReason `json:"tier_per_reason,omitempty"`
}

// IsZero reports whether the override blob carries no overrides.
func (o AgentOverrides) IsZero() bool {
	return o.ProviderOrderSource == "" && len(o.ProviderOrder) == 0 &&
		o.TierSource == "" && o.Tier == "" &&
		o.TierPerReasonSource == "" && len(o.TierPerReason) == 0
}

// ValidationError is the structured error returned by the validators
// so the HTTP layer can surface each problem per-field.
type ValidationError struct {
	Field   string
	Message string
	// Details carries per-row issues (e.g. one entry per provider
	// missing a default-tier mapping when Enabled=true).
	Details []ValidationDetail
}

// ValidationDetail is a single sub-issue inside ValidationError.
type ValidationDetail struct {
	ProviderID ProviderID `json:"provider_id,omitempty"`
	Field      string     `json:"field,omitempty"`
	Message    string     `json:"message"`
}

// Error renders the validation error as a human-readable string. The
// HTTP layer should marshal the structured form rather than relying on
// this for response bodies.
func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return fmt.Sprintf("routing config invalid: %s: %s", e.Field, e.Message)
	}
	parts := make([]string, 0, len(e.Details))
	for _, d := range e.Details {
		parts = append(parts, fmt.Sprintf("%s: %s", d.ProviderID, d.Message))
	}
	return fmt.Sprintf("routing config invalid: %s: %s (%s)",
		e.Field, e.Message, strings.Join(parts, "; "))
}

// ValidateWorkspaceConfig validates cfg against the known-provider
// catalogue. When cfg.Enabled is true, stricter rules apply: the order
// must be non-empty, every provider in the order must have a registered
// ProviderProfile, and every provider must map the DefaultTier to a
// model (so the default tier is always launchable).
func ValidateWorkspaceConfig(cfg WorkspaceConfig, known []ProviderID) error {
	if err := validateOrder(cfg.ProviderOrder, known); err != nil {
		return err
	}
	if err := validateTier(cfg.DefaultTier); err != nil {
		return err
	}
	if err := validateTierPerReason(cfg.TierPerReason, "tier_per_reason"); err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	return validateEnabledRules(cfg)
}

// ValidateAgentOverrides validates the override blob against the
// known-provider catalogue. Same dup/known/tier rules as the workspace
// validator; the override order is allowed to have length 1 (the
// "pin to one provider" case).
func ValidateAgentOverrides(ov AgentOverrides, known []ProviderID) error {
	if ov.ProviderOrderSource == ProviderOrderSourceOverride {
		if err := validateOrder(ov.ProviderOrder, known); err != nil {
			return err
		}
		if len(ov.ProviderOrder) == 0 {
			return &ValidationError{
				Field:   "provider_order",
				Message: "override must list at least one provider",
			}
		}
	}
	if ov.TierSource == TierSourceOverride {
		if err := validateTier(ov.Tier); err != nil {
			return err
		}
	}
	if ov.TierPerReasonSource == TierPerReasonSourceOverride {
		if err := validateTierPerReason(ov.TierPerReason, "routing.tier_per_reason"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAgentOverridesAgainstWorkspace runs the catalogue-only
// validation, then additionally checks that an overridden tier is
// actually mapped by at least one provider in the effective provider
// order for this agent (override order when set, workspace order
// otherwise). Without this check, the save succeeds and every launch
// immediately blocks with no_provider_in_tier — the user sees no
// signal until the next time the agent runs.
func ValidateAgentOverridesAgainstWorkspace(
	ov AgentOverrides, known []ProviderID, cfg *WorkspaceConfig,
) error {
	if err := ValidateAgentOverrides(ov, known); err != nil {
		return err
	}
	if cfg == nil {
		return nil
	}
	order := effectiveOrderForValidation(ov, cfg)
	if err := checkTierMapped(ov, cfg, order); err != nil {
		return err
	}
	if ov.TierPerReasonSource != TierPerReasonSourceOverride {
		return nil
	}
	return checkTierPerReasonMapped(ov.TierPerReason, cfg, order)
}

// effectiveOrderForValidation returns the order the resolver would walk
// for this agent, used to check whether overridden tiers are mapped on
// at least one provider before saving.
func effectiveOrderForValidation(ov AgentOverrides, cfg *WorkspaceConfig) []ProviderID {
	if ov.ProviderOrderSource == ProviderOrderSourceOverride {
		return ov.ProviderOrder
	}
	return cfg.ProviderOrder
}

// checkTierMapped returns nil when the tier override is mapped on at
// least one provider in the effective order. No-op when no tier
// override is set.
func checkTierMapped(ov AgentOverrides, cfg *WorkspaceConfig, order []ProviderID) error {
	if ov.TierSource != TierSourceOverride || ov.Tier == "" {
		return nil
	}
	if tierMappedOnAnyProvider(ov.Tier, order, cfg.ProviderProfiles) {
		return nil
	}
	return &ValidationError{
		Field: "routing.tier",
		Message: fmt.Sprintf(
			"no provider has tier %q mapped in this workspace",
			string(ov.Tier)),
	}
}

// checkTierPerReasonMapped returns nil when every non-empty tier in the
// per-reason override is mapped on at least one provider in the
// effective order.
func checkTierPerReasonMapped(
	m TierPerReason, cfg *WorkspaceConfig, order []ProviderID,
) error {
	var details []ValidationDetail
	for reason, tier := range m {
		if tier == "" {
			continue
		}
		if tierMappedOnAnyProvider(tier, order, cfg.ProviderProfiles) {
			continue
		}
		details = append(details, ValidationDetail{
			Field: reason,
			Message: fmt.Sprintf(
				"tier %q is not mapped on any provider in the effective order",
				string(tier)),
		})
	}
	if len(details) == 0 {
		return nil
	}
	return &ValidationError{
		Field:   "routing.tier_per_reason",
		Message: "wake-reason tier overrides reference unmapped tiers",
		Details: details,
	}
}

// tierMappedOnAnyProvider reports whether at least one provider in
// order has tier mapped to a model. Helper shared by the two override
// checks above.
func tierMappedOnAnyProvider(
	tier Tier, order []ProviderID,
	profiles map[ProviderID]ProviderProfile,
) bool {
	for _, p := range order {
		prof, ok := profiles[p]
		if !ok {
			continue
		}
		if prof.TierMap.IsConfigured(tier) {
			return true
		}
	}
	return false
}

// validateOrder enforces duplicate-free order, providers known, and
// max length = len(known). Called by both validators.
func validateOrder(order []ProviderID, known []ProviderID) error {
	if len(order) > len(known) {
		return &ValidationError{
			Field:   "provider_order",
			Message: fmt.Sprintf("order has %d entries, max %d allowed", len(order), len(known)),
		}
	}
	seen := make(map[ProviderID]struct{}, len(order))
	knownSet := providerSet(known)
	for _, p := range order {
		if p == "" {
			return &ValidationError{Field: "provider_order", Message: "empty provider id"}
		}
		if _, ok := knownSet[p]; !ok {
			return &ValidationError{
				Field:   "provider_order",
				Message: fmt.Sprintf("unknown provider %q", string(p)),
			}
		}
		if _, dup := seen[p]; dup {
			return &ValidationError{
				Field:   "provider_order",
				Message: fmt.Sprintf("duplicate provider %q", string(p)),
			}
		}
		seen[p] = struct{}{}
	}
	return nil
}

// validateTierPerReason rejects unknown wake-reason keys and invalid
// tier values. Empty / nil maps are valid (no policy applied).
func validateTierPerReason(m TierPerReason, field string) error {
	if len(m) == 0 {
		return nil
	}
	knownReasons := wakeReasonSet()
	for reason, tier := range m {
		if _, ok := knownReasons[reason]; !ok {
			return &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("unknown wake reason %q", reason),
			}
		}
		if tier == "" {
			// Empty value means "clear this key"; the caller normally
			// drops it before persisting, but tolerate it here.
			continue
		}
		if err := validateTier(tier); err != nil {
			return &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("%s: %s", reason, err.Error()),
			}
		}
	}
	return nil
}

// wakeReasonSet returns the lookup set used by validateTierPerReason.
// Built from AllWakeReasons so the source of truth stays single.
func wakeReasonSet() map[string]struct{} {
	out := make(map[string]struct{}, len(AllWakeReasons))
	for _, r := range AllWakeReasons {
		out[r] = struct{}{}
	}
	return out
}

// validateTier returns an error when t is not in AllTiers. Empty tier
// is rejected because every config that reaches the validator should
// already have a default applied.
func validateTier(t Tier) error {
	for _, v := range AllTiers {
		if v == t {
			return nil
		}
	}
	return &ValidationError{
		Field:   "default_tier",
		Message: fmt.Sprintf("invalid tier %q", string(t)),
	}
}

// validateEnabledRules is the Enabled=true block of
// ValidateWorkspaceConfig. Extracted to keep the entry point under the
// linter's complexity ceiling.
func validateEnabledRules(cfg WorkspaceConfig) error {
	if len(cfg.ProviderOrder) == 0 {
		return &ValidationError{
			Field:   "provider_order",
			Message: "routing is enabled but no providers are configured",
		}
	}
	var missingProfile []ValidationDetail
	var missingDefault []ValidationDetail
	for _, p := range cfg.ProviderOrder {
		prof, ok := cfg.ProviderProfiles[p]
		if !ok {
			missingProfile = append(missingProfile, ValidationDetail{
				ProviderID: p,
				Message:    "provider has no profile configured",
			})
			continue
		}
		if !prof.TierMap.IsConfigured(cfg.DefaultTier) {
			missingDefault = append(missingDefault, ValidationDetail{
				ProviderID: p,
				Field:      "tier_map." + string(cfg.DefaultTier),
				Message:    "default tier has no model mapping",
			})
		}
	}
	if len(missingProfile) > 0 {
		return &ValidationError{
			Field:   "provider_profiles",
			Message: "providers in order have no profile",
			Details: missingProfile,
		}
	}
	if len(missingDefault) > 0 {
		return &ValidationError{
			Field:   "provider_profiles",
			Message: "providers missing default-tier model mapping",
			Details: missingDefault,
		}
	}
	return nil
}

// providerSet builds a lookup set from a known-provider slice. Used by
// validateOrder to avoid O(n*m) scans on every entry.
func providerSet(known []ProviderID) map[ProviderID]struct{} {
	out := make(map[ProviderID]struct{}, len(known))
	for _, p := range known {
		out[p] = struct{}{}
	}
	return out
}

// ErrEmptyOrder is returned by callers (resolver, scheduler) when an
// effective provider order is computed to be empty. Validators return
// the richer *ValidationError; this sentinel is for downstream callers
// that want a stable comparison target.
var ErrEmptyOrder = errors.New("routing: effective provider order is empty")
