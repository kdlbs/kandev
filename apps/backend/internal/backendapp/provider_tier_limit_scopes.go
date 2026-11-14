package backendapp

import (
	"context"
	"fmt"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// limitDimensionModelDaily is a per-model daily allowance error. It learns the
// model's request scope, never the account-wide daily allowance.
const limitDimensionModelDaily = "model_daily"

// readScopeUsage counts each covering scope's requests since its daily reset
// by the model recorded for each request across the tier's account profiles,
// so a profile's current default model never attributes another model's use.
func (s *providerTierLimitService) readScopeUsage(ctx context.Context, c tierLimitContext, now time.Time, used *agentusage.LimitUsage) error {
	if len(c.effective.Scopes) == 0 {
		return nil
	}
	used.Scopes = make(map[string]float64, len(c.effective.Scopes))
	for _, scope := range c.effective.Scopes {
		if !scope.Covers(c.profile.Model) {
			continue
		}
		start, reset, err := agentusage.DailyLimitWindow(now, scope.DailyReset)
		if err != nil {
			return err
		}
		used.Resets[agentusage.ScopeUsageKey(scope.ID)] = reset
		count, err := s.readModelRequestCount(ctx, c.ids, scope.Models, start, now)
		if err != nil {
			return fmt.Errorf("read model scope: %w", err)
		}
		used.Scopes[scope.ID] = count
	}
	return nil
}

// sameMinuteWindow bounds how far apart two per-model daily blocks may be to
// count as one shared group allowance running out.
const sameMinuteWindow = time.Minute

// learnModelScope records the requests counted in the scope covering the
// profile's model as that scope's learned value. A model without a scope gets
// a learned model scope of its own; same-minute blocks of unconfigured models
// form a provisional group instead.
func (s *providerTierLimitService) learnModelScope(ctx context.Context, c tierLimitContext, at time.Time) error {
	scope, counted, err := s.scopeToLearn(ctx, c, at)
	if err != nil || counted <= 0 {
		return err
	}
	tier := c.learned.Tiers[c.tier]
	if scope.Provisional {
		tier.Scopes = agentusage.WithoutLearnedModelScopes(tier.Scopes, scope.Models, ownerScopeIDs(c))
	}
	tier.Scopes = agentusage.LearnScope(tier.Scopes, scope, counted, at)
	c.learned.Tiers[c.tier] = tier
	return s.store.SaveAccountTierLearned(ctx, c.key, c.learned)
}

// scopeToLearn counts the learned scope's requests by recorded model, like
// readScopeUsage, so the learned value matches what admission counts.
func (s *providerTierLimitService) scopeToLearn(ctx context.Context, c tierLimitContext, at time.Time) (agentusage.RequestScope, float64, error) {
	scope, ok := s.sameMinuteGroup(ctx, c, at)
	if !ok {
		scope, ok = agentusage.CoveringScope(c.effective.Scopes, c.profile.Model)
	}
	if !ok {
		model := agentusage.ScopeModelID(c.profile.Model)
		scope = agentusage.RequestScope{ID: agentusage.ModelScopeID(model), Kind: agentusage.ScopeKindModel, Models: []string{model}}
	}
	start, _, err := agentusage.DailyLimitWindow(at, scope.DailyReset)
	if err != nil {
		return scope, 0, err
	}
	counted, err := s.readModelRequestCount(ctx, c.ids, scope.Models, start, at)
	return scope, counted, err
}

// sameMinuteGroup finds other unconfigured models of the account whose
// per-model daily block was observed within a minute of this one. Observation
// read errors fall back to model learning.
func (s *providerTierLimitService) sameMinuteGroup(ctx context.Context, c tierLimitContext, at time.Time) (agentusage.RequestScope, bool) {
	configured := configuredScopes(c)
	if agentusage.ConfiguredScopeCovers(configured, c.profile.Model) {
		return agentusage.RequestScope{}, false
	}
	observations, err := s.store.ListUsageLimitObservationsSince(ctx, at.Add(-sameMinuteWindow))
	if err != nil {
		return agentusage.RequestScope{}, false
	}
	var partners []string
	for _, o := range observations {
		if o.AccountKey != c.key || o.Dimension != limitDimensionModelDaily || o.ObservedAt.After(at.Add(sameMinuteWindow)) {
			continue
		}
		if o.ModelID != "" && !agentusage.ConfiguredScopeCovers(configured, o.ModelID) {
			partners = append(partners, o.ModelID)
		}
	}
	return agentusage.ProvisionalGroupFor(c.learned.Tiers[c.tier].Scopes, c.profile.Model, partners)
}

// configuredScopes are catalog defaults and every scope the owner set. An
// owner entry that carries only an ID (the card's value or reset edit of a
// learned scope) resolves to the current scope definition, so its models stay
// configured and keep their owner value instead of being grouped.
func configuredScopes(c tierLimitContext) []agentusage.RequestScope {
	configured := append([]agentusage.RequestScope(nil), c.defaults.Scopes...)
	for _, scope := range c.owner.Tiers[c.tier].Scopes {
		if scope.Kind != "" && len(scope.Models) > 0 {
			configured = append(configured, scope)
			continue
		}
		for _, effective := range c.effective.Scopes {
			if effective.ID == scope.ID && len(effective.Models) > 0 {
				configured = append(configured, effective)
				break
			}
		}
	}
	return configured
}

// ownerScopeIDs are the scope IDs the owner set; their learned definitions
// must survive so the owner entry keeps covering its models.
func ownerScopeIDs(c tierLimitContext) map[string]bool {
	ids := make(map[string]bool)
	for _, scope := range c.owner.Tiers[c.tier].Scopes {
		ids[scope.ID] = true
	}
	return ids
}
