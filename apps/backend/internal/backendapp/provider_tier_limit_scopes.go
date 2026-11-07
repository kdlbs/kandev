package backendapp

import (
	"context"
	"sort"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// limitDimensionModelDaily is a per-model daily allowance error. It learns the
// model's request scope, never the account-wide daily allowance.
const limitDimensionModelDaily = "model_daily"

// readScopeUsage counts each scope's requests since its daily reset over the
// tier's account profiles whose model the scope covers.
func (s *providerTierLimitService) readScopeUsage(ctx context.Context, c tierLimitContext, now time.Time, used *agentusage.LimitUsage) error {
	if len(c.effective.Scopes) == 0 {
		return nil
	}
	used.Scopes = make(map[string]float64, len(c.effective.Scopes))
	models := s.profileModels(ctx, c.ids)
	for _, scope := range c.effective.Scopes {
		start, reset, err := agentusage.DailyLimitWindow(now, scope.DailyReset)
		if err != nil {
			return err
		}
		used.Resets[agentusage.ScopeUsageKey(scope.ID)] = reset
		ids := scopeProfileIDs(models, scope)
		if len(ids) == 0 {
			used.Scopes[scope.ID] = 0
			continue
		}
		count, err := s.readRequestCount(ctx, ids, start, now)
		if err != nil {
			return err
		}
		used.Scopes[scope.ID] = count
	}
	return nil
}

func (s *providerTierLimitService) profileModels(ctx context.Context, ids []string) map[string]string {
	models := make(map[string]string, len(ids))
	for _, id := range ids {
		if p, _, ok := s.usage.loadProfile(ctx, id); ok {
			models[id] = p.Model
		}
	}
	return models
}

func scopeProfileIDs(models map[string]string, scope agentusage.RequestScope) []string {
	var ids []string
	for id, model := range models {
		if scope.Covers(model) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// sameMinuteWindow bounds how far apart two per-model daily blocks may be to
// count as one shared group allowance running out.
const sameMinuteWindow = time.Minute

// learnModelScope records the requests counted in the scope covering the
// profile's model as that scope's learned value. A model without a scope gets
// a learned model scope of its own; same-minute blocks of unconfigured models
// form a provisional group instead.
func (s *providerTierLimitService) learnModelScope(ctx context.Context, c tierLimitContext, used agentusage.LimitUsage, at time.Time) error {
	models := s.profileModels(ctx, c.ids)
	scope, counted, err := s.scopeToLearn(ctx, c, models, used, at)
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

func (s *providerTierLimitService) scopeToLearn(ctx context.Context, c tierLimitContext, models map[string]string, used agentusage.LimitUsage, at time.Time) (agentusage.RequestScope, float64, error) {
	if group, ok := s.sameMinuteGroup(ctx, c, models, at); ok {
		counted, err := s.countScope(ctx, models, group, at)
		return group, counted, err
	}
	if scope, ok := agentusage.CoveringScope(c.effective.Scopes, c.profile.Model); ok {
		return scope, used.Scopes[scope.ID], nil
	}
	model := agentusage.ScopeModelID(c.profile.Model)
	scope := agentusage.RequestScope{ID: agentusage.ModelScopeID(model), Kind: agentusage.ScopeKindModel, Models: []string{model}}
	counted, err := s.countScope(ctx, models, scope, at)
	return scope, counted, err
}

func (s *providerTierLimitService) countScope(ctx context.Context, models map[string]string, scope agentusage.RequestScope, at time.Time) (float64, error) {
	start, _, err := agentusage.DailyLimitWindow(at, scope.DailyReset)
	if err != nil {
		return 0, err
	}
	return s.readRequestCount(ctx, scopeProfileIDs(models, scope), start, at)
}

// sameMinuteGroup finds other unconfigured models of the account whose
// per-model daily block was observed within a minute of this one. Observation
// read errors fall back to model learning.
func (s *providerTierLimitService) sameMinuteGroup(ctx context.Context, c tierLimitContext, models map[string]string, at time.Time) (agentusage.RequestScope, bool) {
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
		if o.AccountKey != c.key || o.Dimension != limitDimensionModelDaily || o.ExecutionProfileID == c.profile.ID || o.ObservedAt.After(at.Add(sameMinuteWindow)) {
			continue
		}
		if model, ok := models[o.ExecutionProfileID]; ok && !agentusage.ConfiguredScopeCovers(configured, model) {
			partners = append(partners, model)
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
