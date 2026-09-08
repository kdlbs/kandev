// Package routing provides typed routing config for the office provider-
// routing feature: tiers, providers, workspace + agent overrides, and the
// validators that the HTTP/repo layers call before persisting changes.
//
// The package has no behavior of its own in Phase 0 — it ships the types
// and validators that downstream phases (resolver, scheduler, classifier)
// will consume.
package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
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

// Tier-resolution level names for the widened, four-value tier_source
// contract computed by effectiveTier and carried on Resolution and
// RouteAttempt: wake_reason | override | role | workspace. This is a
// different contract from AgentOverrides.TierSource above (still
// inherit | override | "" — a persisted "did the user override"
// marker, not a resolution level) even though both are spelled
// "tier_source" and share the "override" string. See spec AC-18a.
const (
	TierSourceWakeReason = "wake_reason"
	TierSourceRole       = "role"
	TierSourceWorkspace  = "workspace"
)

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

// AllAgentRoles enumerates the seven role_tiers keys, in the
// settingsmodels.AgentRole constants' declaration order. This order is
// also what the frontend role list follows, so it lives here as the
// single source of truth rather than being duplicated as a literal list.
var AllAgentRoles = []settingsmodels.AgentRole{
	settingsmodels.AgentRoleCEO,
	settingsmodels.AgentRoleWorker,
	settingsmodels.AgentRoleSpecialist,
	settingsmodels.AgentRoleAssistant,
	settingsmodels.AgentRoleSecurity,
	settingsmodels.AgentRoleQA,
	settingsmodels.AgentRoleDevOps,
}

// RoleTierMap maps an agent role to the tier that role should run at.
// Keys are restricted to AllAgentRoles; values to AllTiers. Empty /
// missing keys mean "no role policy — fall through to the workspace
// default_tier." Stored as JSON on the workspace routing row.
type RoleTierMap map[string]Tier

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

// ProviderProfile maps each tier to the complete execution profile used for
// that launch. TierMap, Mode, Flags, and Env are retained as legacy display
// snapshots during migration; execution profile references are authoritative.
type ProviderProfile struct {
	TierMap             TierMap             `json:"tier_map"`
	ExecutionProfileIDs ExecutionProfileIDs `json:"-"`
	// TierProfileIDs is retained as a source-compatibility shim while callers
	// migrate. JSON decoding normalizes the legacy key into
	// ExecutionProfileIDs and JSON encoding always writes the canonical key.
	TierProfileIDs TierProfileIDs    `json:"-"`
	Mode           string            `json:"mode,omitempty"`
	Flags          []string          `json:"flags,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
}

// MarshalJSON writes the canonical execution_profile_ids key while accepting
// TierProfileIDs as the in-memory compatibility field during migration.
func (p ProviderProfile) MarshalJSON() ([]byte, error) {
	type providerProfileJSON struct {
		TierMap             TierMap              `json:"tier_map"`
		ExecutionProfileIDs *ExecutionProfileIDs `json:"execution_profile_ids,omitempty"`
		Mode                string               `json:"mode,omitempty"`
		Flags               []string             `json:"flags,omitempty"`
		Env                 map[string]string    `json:"env,omitempty"`
	}
	out := providerProfileJSON{
		TierMap: p.TierMap,
		Mode:    p.Mode,
		Flags:   p.Flags,
		Env:     p.Env,
	}
	ids := p.effectiveExecutionProfileIDs()
	if !ids.IsZero() {
		out.ExecutionProfileIDs = &ids
	}
	return json.Marshal(out)
}

// UnmarshalJSON accepts the canonical execution_profile_ids field and the
// legacy tier_profile_ids field. When both are present the canonical field is
// authoritative, including when it is explicitly empty.
func (p *ProviderProfile) UnmarshalJSON(data []byte) error {
	type providerProfileJSON struct {
		TierMap             TierMap              `json:"tier_map"`
		ExecutionProfileIDs *ExecutionProfileIDs `json:"execution_profile_ids"`
		TierProfileIDs      *ExecutionProfileIDs `json:"tier_profile_ids"`
		Mode                string               `json:"mode,omitempty"`
		Flags               []string             `json:"flags,omitempty"`
		Env                 map[string]string    `json:"env,omitempty"`
	}
	var raw providerProfileJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = ProviderProfile{
		TierMap: raw.TierMap,
		Mode:    raw.Mode,
		Flags:   raw.Flags,
		Env:     raw.Env,
	}
	if raw.ExecutionProfileIDs != nil {
		p.ExecutionProfileIDs = *raw.ExecutionProfileIDs
		p.TierProfileIDs = *raw.ExecutionProfileIDs
	} else if raw.TierProfileIDs != nil {
		p.ExecutionProfileIDs = *raw.TierProfileIDs
		p.TierProfileIDs = *raw.TierProfileIDs
	}
	return nil
}

// ExecutionProfileIDs maps each tier to the complete agent profile used to
// launch that candidate.
type ExecutionProfileIDs struct {
	Frontier string `json:"frontier,omitempty"`
	Balanced string `json:"balanced,omitempty"`
	Economy  string `json:"economy,omitempty"`
}

// TierProfileIDs is the legacy source name kept while call sites migrate.
type TierProfileIDs = ExecutionProfileIDs

// ProfileID returns the execution profile configured for tier.
func (ids ExecutionProfileIDs) ProfileID(tier Tier) string {
	switch tier {
	case TierFrontier:
		return ids.Frontier
	case TierBalanced:
		return ids.Balanced
	case TierEconomy:
		return ids.Economy
	default:
		return ""
	}
}

// IsZero reports whether no tier has an execution profile.
func (ids ExecutionProfileIDs) IsZero() bool {
	return ids.Frontier == "" && ids.Balanced == "" && ids.Economy == ""
}

// ExecutionProfileID returns the complete runtime profile for tier. The
// deprecated source field is consulted only when no canonical mapping exists.
func (p ProviderProfile) ExecutionProfileID(tier Tier) string {
	return p.effectiveExecutionProfileIDs().ProfileID(tier)
}

func (p ProviderProfile) effectiveExecutionProfileIDs() ExecutionProfileIDs {
	if !p.ExecutionProfileIDs.IsZero() {
		return p.ExecutionProfileIDs
	}
	return p.TierProfileIDs
}

// WorkspaceConfig is the persisted routing config for one workspace. Enabled
// controls automatic fallback; tier-to-execution-profile selection applies to
// every Office launch.
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
	// RoleTiers maps an agent role onto the tier that role should run
	// at, applied below TierPerReason and the per-agent override but
	// above DefaultTier. An empty / missing map means "no role policy"
	// — the default for every workspace.
	RoleTiers RoleTierMap `json:"role_tiers,omitempty"`
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
	if err := validateRoleTiers(cfg.RoleTiers, "role_tiers"); err != nil {
		return err
	}
	if err := checkRoleTiersMapped(cfg.RoleTiers, &cfg); err != nil {
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
// order has a launchable execution profile for the tier. Helper shared
// by the two override checks above.
func tierMappedOnAnyProvider(
	tier Tier, order []ProviderID,
	profiles map[ProviderID]ProviderProfile,
) bool {
	for _, p := range order {
		prof, ok := profiles[p]
		if !ok {
			continue
		}
		if prof.ExecutionProfileID(tier) != "" {
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

// validateRoleTiers rejects role_tiers keys outside the seven
// AllAgentRoles values and values outside AllTiers. Unlike
// validateTierPerReason (fail-fast on the first bad entry),
// role_tiers accumulates one ValidationDetail per offending key —
// role_tiers is a map that may carry several bad entries in a single
// write, and a single flat error would collapse them into one message
// and hide which of the seven roles is broken. Messages are built
// directly here rather than through validateTier, whose Field is
// hardcoded to "default_tier" and whose Error() text is prefixed with
// "routing config invalid:" — either leaking into a role_tiers message
// would misattribute the offending field.
func validateRoleTiers(m RoleTierMap, field string) error {
	if len(m) == 0 {
		return nil
	}
	knownRoles := agentRoleSet()
	var details []ValidationDetail
	for role, tier := range m {
		if _, ok := knownRoles[role]; !ok {
			details = append(details, ValidationDetail{
				Field:   role,
				Message: fmt.Sprintf("unknown role %q", role),
			})
			continue
		}
		if tier == "" {
			// AC-11: empty value means "absent"; dropped before
			// persistence, not rejected.
			continue
		}
		if !isValidTierValue(tier) {
			details = append(details, ValidationDetail{
				Field:   role,
				Message: fmt.Sprintf("invalid tier %q", string(tier)),
			})
		}
	}
	if len(details) == 0 {
		return nil
	}
	return &ValidationError{
		Field:   field,
		Message: "role_tiers references unknown roles or invalid tiers",
		Details: details,
	}
}

// isValidTierValue reports whether t is one of AllTiers. Unlike
// validateTier, it carries no field/message opinion — callers that need
// a role_tiers-scoped error build it themselves (see validateRoleTiers).
func isValidTierValue(t Tier) bool {
	for _, v := range AllTiers {
		if v == t {
			return true
		}
	}
	return false
}

// agentRoleSet returns the lookup set used by validateRoleTiers. Built
// from AllAgentRoles so the source of truth stays single.
func agentRoleSet() map[string]struct{} {
	out := make(map[string]struct{}, len(AllAgentRoles))
	for _, r := range AllAgentRoles {
		out[string(r)] = struct{}{}
	}
	return out
}

// checkRoleTiersMapped returns nil when every non-empty tier in
// role_tiers is mapped on at least one provider in cfg.ProviderOrder.
// Mirrors checkTierPerReasonMapped's accumulation shape rather than
// checkTierMapped's single-value shape, for the same reason
// validateRoleTiers does: role_tiers may hold several bad entries at
// once. Checked unconditionally (not gated on cfg.Enabled) because a
// role tier can supply the effective tier for a launch even while
// automatic fallback is disabled (AC-12/AC-12a).
func checkRoleTiersMapped(m RoleTierMap, cfg *WorkspaceConfig) error {
	var details []ValidationDetail
	for role, tier := range m {
		if tier == "" {
			continue
		}
		if tierMappedOnAnyProvider(tier, cfg.ProviderOrder, cfg.ProviderProfiles) {
			continue
		}
		details = append(details, ValidationDetail{
			Field: role,
			Message: fmt.Sprintf(
				"tier %q is not mapped on any provider in the workspace provider order",
				string(tier)),
		})
	}
	if len(details) == 0 {
		return nil
	}
	return &ValidationError{
		Field:   "role_tiers",
		Message: "role tier overrides reference unmapped tiers",
		Details: details,
	}
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
		if prof.ExecutionProfileID(cfg.DefaultTier) == "" {
			missingDefault = append(missingDefault, ValidationDetail{
				ProviderID: p,
				Field:      "execution_profile_ids." + string(cfg.DefaultTier),
				Message:    "default tier has no execution profile mapping",
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
			Message: "providers missing default-tier execution profile mapping",
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
