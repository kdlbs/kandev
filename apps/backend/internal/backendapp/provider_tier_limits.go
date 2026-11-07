package backendapp

import (
	"context"
	"errors"
	"fmt"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"slices"
	"strings"
	"sync"
	"time"
)

// tierResetProvider marks a budget whose reset follows the provider's own
// reported window instead of a local calendar rule.
const tierResetProvider = "provider"

type tierLimitStore interface {
	LoadAccountTierConfig(context.Context, string) (agentusage.AccountTierConfig, agentusage.AccountTierConfig, error)
	SaveAccountTierOverrides(context.Context, string, agentusage.AccountTierConfig) error
	SaveAccountTierLearned(context.Context, string, agentusage.AccountTierConfig) error
	CountLimitActiveTurns(context.Context, []string, string) (int64, error)
	GetManualWindowUsageForProfiles(context.Context, []string, time.Time, time.Time) (sqliterepo.ManualWindowUsage, error)
	InsertUsageLimitObservation(context.Context, sqliterepo.UsageLimitObservation) error
	ListUsageLimitObservationsSince(context.Context, time.Time) ([]sqliterepo.UsageLimitObservation, error)
}

type providerTierLimitService struct {
	freeModels interface {
		IsFreeModel(context.Context, string) (bool, bool)
	}
	store      tierLimitStore
	usage      *usageProviderAdapter
	learningMu sync.Mutex
	requests   requestLimitObservations
}

type tierLimitContext struct {
	modelOverride       bool
	profile             *settingsmodels.AgentProfile
	provider            string
	key                 string
	identity            accountIdentity
	tier                agentusage.BillingTier
	owner, learned      agentusage.AccountTierConfig
	observed            *agentusage.ProviderUsage
	modelClass          agentusage.ModelClass
	defaults, effective agentusage.TierLimits
	ids                 []string
}

var billingTiers = []agentusage.BillingTier{agentusage.TierFree, agentusage.TierSubscription, agentusage.TierPaid}

type catalogFreeModels struct{}

func (catalogFreeModels) IsFreeModel(_ context.Context, model string) (bool, bool) {
	return agentusage.CatalogFreeModel(model)
}

func newProviderTierLimitService(usage *usageProviderAdapter, store tierLimitStore) *providerTierLimitService {
	return &providerTierLimitService{store: store, usage: usage, freeModels: catalogFreeModels{}}
}

func (s *providerTierLimitService) contextFor(ctx context.Context, id string) (tierLimitContext, error) {
	return s.contextForModel(ctx, id, "")
}

func (s *providerTierLimitService) contextForModel(ctx context.Context, id, model string) (tierLimitContext, error) {
	var c tierLimitContext
	profile, agentType, bound, overridden, err := s.tierProfileForModel(ctx, id, model)
	if err != nil {
		return c, err
	}
	c.modelOverride = overridden
	c.profile = profile
	c.key = bound.accountKey
	c.identity = accountIdentityFor(c.key)
	c.provider = dynamicruntime.ProviderOf(profile.Model)
	if c.provider == "" {
		c.provider = strings.SplitN(c.key, ":", 2)[0]
	}
	c.owner, c.learned, err = s.store.LoadAccountTierConfig(ctx, c.key)
	if err != nil {
		return c, err
	}
	observed := s.usage.ProfileUsage(ctx, id)
	if overridden {
		observed.ModelClass = modelClassForOverride(ctx, observed, bound)
	}
	c.observed = observed.Usage
	c.modelClass = observed.ModelClass
	if s.freeModels != nil {
		if free, known := s.freeModels.IsFreeModel(ctx, profile.Model); known && free {
			c.modelClass = agentusage.ModelClassFree
		}
	}
	if dynamicruntime.IsFreeModel(profile.Model) {
		c.modelClass = agentusage.ModelClassFree
	}
	c.tier = s.effectiveBillingTier(ctx, profile, agentType, observed, c.owner)
	c.defaults = s.defaultLimits(c)
	c.effective = agentusage.MergeTierLimits(agentusage.MergeTierLimits(c.defaults, c.learned.Tiers[c.tier]), c.owner.Tiers[c.tier])
	c.ids = s.accountTierProfiles(ctx, c)
	if overridden && !slices.Contains(c.ids, id) {
		c.ids = append(c.ids, id)
	}
	return c, nil
}

func (s *providerTierLimitService) tierProfileForModel(ctx context.Context, id, model string) (*settingsmodels.AgentProfile, string, usageBinding, bool, error) {
	profile, agentType, ok := s.usage.loadProfile(ctx, id)
	if !ok || profile == nil {
		return nil, "", usageBinding{}, false, fmt.Errorf("profile unavailable")
	}
	original, ok := s.usage.bindingFor(profile, agentType)
	if !ok {
		return nil, "", usageBinding{}, false, fmt.Errorf("account unavailable")
	}
	overridden := model != "" && model != profile.Model
	if overridden {
		copy := *profile
		copy.Model = model
		profile = &copy
	}
	bound := original
	if overridden {
		bound, ok = s.usage.bindingFor(profile, agentType)
		if !ok || bound.accountKey != original.accountKey {
			return nil, "", usageBinding{}, false, fmt.Errorf("execution model account binding is unavailable")
		}
	}
	return profile, agentType, bound, overridden, nil
}

func modelClassForOverride(ctx context.Context, observed profileUsage, bound usageBinding) agentusage.ModelClass {
	class := agentusage.ModelClassUnknown
	if observed.Usage != nil && strings.EqualFold(observed.Usage.Plan, "free") {
		class = agentusage.ModelClassFree
	}
	if bound.class != nil {
		if resolved := bound.class(ctx); resolved != agentusage.ModelClassUnknown {
			class = resolved
		}
	}
	return class
}

func (s *providerTierLimitService) effectiveBillingTier(ctx context.Context, p *settingsmodels.AgentProfile, a string, observed profileUsage, owner agentusage.AccountTierConfig) agentusage.BillingTier {
	if override := owner.ProfileTiers[p.ID]; validBillingTier(override) {
		return override
	}
	return s.billingTier(ctx, p, a, observed)
}

func (s *providerTierLimitService) defaultLimits(c tierLimitContext) agentusage.TierLimits {
	purchased := c.owner.PurchasedCredits != nil && *c.owner.PurchasedCredits
	plan := ""
	if c.observed != nil {
		plan = c.observed.Plan
		if counter := c.observed.FreeModelDailyRequests; counter != nil && counter.Limit >= 1000 {
			purchased = true
		}
	}
	if c.provider == accountPrefixOpenCodeGo {
		plan = c.owner.SubscriptionPlan
	}
	defaults := agentusage.DefaultTierLimits(c.provider, c.tier, plan, purchased)
	if c.tier == agentusage.TierFree && c.observed != nil && c.observed.FreeModelDailyRequests != nil {
		v := c.observed.FreeModelDailyRequests.Limit
		defaults.RequestsPerDay = &v
		defaults.Provenance = withProvenance(defaults.Provenance, "requests_per_day", agentusage.ProvenanceProvider)
	}
	if c.tier == agentusage.TierSubscription {
		defaults.Windows = appendProviderBudgets(defaults.Windows, c.observed)
	}
	return defaults
}

func withProvenance(labels map[string]string, field, provenance string) map[string]string {
	result := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		result[key] = value
	}
	result[field] = provenance
	return result
}

func (s *providerTierLimitService) accountTierProfiles(ctx context.Context, c tierLimitContext) []string {
	ids, err := s.usage.AccountProfileIDs(ctx, c.profile.ID)
	if err != nil {
		return []string{c.profile.ID}
	}
	var result []string
	for _, id := range ids {
		p, a, ok := s.usage.loadProfile(ctx, id)
		if !ok {
			continue
		}
		observed := s.usage.ProfileUsage(ctx, id)
		if s.effectiveBillingTier(ctx, p, a, observed, c.owner) == c.tier {
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return []string{c.profile.ID}
	}
	return result
}

func inferBillingTier(profile *settingsmodels.AgentProfile, agentType string, observed profileUsage) agentusage.BillingTier {
	if dynamicruntime.IsFreeModel(profile.Model) || observed.ModelClass == agentusage.ModelClassFree {
		return agentusage.TierFree
	}
	if subscriptionProvider(profile.Model, observed) {
		return agentusage.TierSubscription
	}
	if profile.BillingType == string(agentusage.BillingTypeSubscription) {
		return agentusage.TierSubscription
	}
	if profile.ProviderKind != "" || profile.BillingType == "api_key" {
		return agentusage.TierPaid
	}
	switch agentType {
	case claudeACPAgentID, codexACPAgentID, codexAppServerAgentID, antigravityAgentID:
		return agentusage.TierSubscription
	}
	if strings.Contains(agentType, "gemini") {
		return agentusage.TierSubscription
	}
	return agentusage.TierPaid
}

func subscriptionProvider(model string, observed profileUsage) bool {
	provider := dynamicruntime.ProviderOf(model)
	if provider == accountPrefixOpenCodeGo {
		return true
	}
	return provider == accountPrefixLLMGateway && observed.Usage != nil && observed.Usage.Plan != "" && observed.Usage.Plan != "none"
}

func validBillingTier(tier agentusage.BillingTier) bool {
	return tier == agentusage.TierFree || tier == agentusage.TierSubscription || tier == agentusage.TierPaid
}

func (s *providerTierLimitService) billingTier(ctx context.Context, p *settingsmodels.AgentProfile, a string, observed profileUsage) agentusage.BillingTier {
	if s.freeModels != nil {
		if free, known := s.freeModels.IsFreeModel(ctx, p.Model); known && free {
			observed.ModelClass = agentusage.ModelClassFree
		}
	}
	if bound, ok := s.usage.bindingFor(p, a); ok {
		if credentials, ok := bound.client.(interface{ HasSubscriptionCredentials() bool }); ok && !credentials.HasSubscriptionCredentials() && observed.Usage == nil && p.BillingType == "" && p.ProviderKind == "" {
			if a == claudeACPAgentID || a == codexACPAgentID || a == codexAppServerAgentID {
				return agentusage.TierPaid
			}
		}
	}
	return inferBillingTier(p, a, observed)
}

func appendProviderBudgets(budgets []agentusage.LimitBudget, observed *agentusage.ProviderUsage) []agentusage.LimitBudget {
	if observed == nil {
		return budgets
	}
	for _, w := range observed.Windows {
		period := providerWindowPeriod(w)
		if period == "" {
			continue
		}
		found := false
		for _, b := range budgets {
			if b.Window == period && b.Scope == string(w.Scope) {
				found = true
			}
		}
		if found {
			continue
		}
		v := 100.0
		b := agentusage.LimitBudget{Window: period, Unit: "percent", Value: &v, Reset: tierResetProvider, Scope: string(w.Scope)}
		if !w.ResetAt.IsZero() {
			at := w.ResetAt
			b.ResetAt = &at
		}
		budgets = append(budgets, b)
	}
	return budgets
}

func providerWindowPeriod(w agentusage.UtilizationWindow) string {
	label := strings.ToLower(w.Label)
	switch {
	case strings.Contains(label, "month") || w.DurationSeconds >= 27*24*3600:
		return "month"
	case strings.Contains(label, "week") || strings.Contains(label, "7-day") || w.DurationSeconds >= 7*24*3600:
		return "week"
	case strings.Contains(label, "5-hour") || strings.Contains(label, "five_hour") || w.DurationSeconds == 5*3600:
		return "5h"
	case strings.Contains(label, "day") || w.DurationSeconds == 24*3600:
		return "day"
	}
	return ""
}

func (s *providerTierLimitService) Check(ctx context.Context, id, exclude string, now time.Time) (agentusage.LimitAdmission, error) {
	return s.check(ctx, id, exclude, now, nil)
}

func (s *providerTierLimitService) CheckCandidate(ctx context.Context, candidate dynamicruntime.Candidate, exclude string, now time.Time) (agentusage.LimitAdmission, error) {
	if candidate.RequireExecutionUsage && candidate.UsagePolicy == nil {
		return agentusage.LimitAdmission{}, fmt.Errorf("execution allowance account is unknown")
	}
	model := ""
	if candidate.UsagePolicy != nil && candidate.UsagePolicy.ProfileID == candidate.ID && agentusage.MatchesSelectedUsage(ctx, candidate.UsagePolicy) {
		model = candidate.UsagePolicy.ExpectedModel
	}
	return s.checkModel(ctx, candidate.ID, exclude, now, candidate.UsagePolicy, model)
}

func (s *providerTierLimitService) check(ctx context.Context, id, exclude string, now time.Time, selected *agentusage.ExecutionUsagePolicy) (agentusage.LimitAdmission, error) {
	return s.checkModel(ctx, id, exclude, now, selected, "")
}

func (s *providerTierLimitService) checkModel(ctx context.Context, id, exclude string, now time.Time, selected *agentusage.ExecutionUsagePolicy, model string) (agentusage.LimitAdmission, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := s.contextForModel(ctx, id, model)
	if err != nil {
		return agentusage.LimitAdmission{}, err
	}
	if selected != nil && !tierContextMatchesSelection(c, selected) {
		return agentusage.LimitAdmission{}, fmt.Errorf("configured allowance account does not match execution selection")
	}
	used, err := s.readUsage(ctx, c, exclude, now)
	if err != nil {
		if errors.Is(err, errModelAttributionUnavailable) {
			if admission := provenAccountLimit(c.effective, used, now); admission.Reason != "" {
				return admission, nil
			}
		}
		return agentusage.LimitAdmission{}, err
	}
	effective := c.effective
	effective.Windows = applicableBudgets(c.effective.Windows, c.profile.Model, c.tier, c.modelClass)
	if admission := agentusage.EvaluateTierLimits(effective, used, now); admission.Reason != "" {
		return admission, nil
	}
	return agentusage.EvaluateScopeLimits(c.effective.Scopes, c.profile.Model, used, now), nil
}

func tierContextMatchesSelection(c tierLimitContext, selected *agentusage.ExecutionUsagePolicy) bool {
	if selected == nil || c.profile == nil || c.observed == nil || selected.ExpectedAccountRef == "" || selected.ExpectedProvider == "" {
		return false
	}
	return strings.EqualFold(c.observed.Provider, selected.ExpectedProvider) &&
		c.observed.AccountRef == selected.ExpectedAccountRef &&
		c.observed.OrganizationRef == selected.ExpectedOrganizationRef &&
		(selected.ExpectedModel == "" || c.profile.Model == selected.ExpectedModel)
}

func (s *providerTierLimitService) matchesSelectedAccount(ctx context.Context, candidate dynamicruntime.Candidate) bool {
	if candidate.UsagePolicy == nil {
		return !candidate.RequireExecutionUsage
	}
	if candidate.UsagePolicy.ProfileID != candidate.ID {
		return false
	}
	model := ""
	if candidate.ModelID == candidate.UsagePolicy.ExpectedModel && agentusage.MatchesSelectedUsage(ctx, candidate.UsagePolicy) {
		model = candidate.ModelID
	}
	c, err := s.contextForModel(ctx, candidate.ID, model)
	return err == nil && tierContextMatchesSelection(c, candidate.UsagePolicy)
}

// applicableBudgets keeps account-wide budgets and the model-class windows that
// cover this profile's model, so one class window cannot block another.
func applicableBudgets(budgets []agentusage.LimitBudget, model string, tier agentusage.BillingTier, modelClass agentusage.ModelClass) []agentusage.LimitBudget {
	if len(budgets) == 0 {
		return budgets
	}
	free := tier == agentusage.TierFree || dynamicruntime.IsFreeModel(model) || modelClass == agentusage.ModelClassFree
	premium := modelClass == agentusage.ModelClassPremium
	applicable := make([]agentusage.LimitBudget, 0, len(budgets))
	for _, budget := range budgets {
		switch budget.Scope {
		case "":
			applicable = append(applicable, budget)
		case string(agentusage.WindowScopeFreeModels):
			if free {
				applicable = append(applicable, budget)
			}
		case string(agentusage.WindowScopePremiumModels):
			if premium {
				applicable = append(applicable, budget)
			}
		case string(agentusage.WindowScopePaidModels):
			if !free && !premium {
				applicable = append(applicable, budget)
			}
		default:
			applicable = append(applicable, budget)
		}
	}
	return applicable
}

func (s *providerTierLimitService) ListAccounts(ctx context.Context) (map[string][]controller.ProviderAccountLimitDTO, error) {
	profiles, err := s.usage.boundProfiles(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]controller.ProviderAccountLimitDTO)
	byKey := make(map[string]*controller.ProviderAccountLimitDTO)
	providers := make(map[string]string)
	for _, bound := range profiles {
		c, err := s.contextFor(ctx, bound.profile.ID)
		if err != nil {
			continue
		}
		account := byKey[c.key]
		if account == nil {
			account = &controller.ProviderAccountLimitDTO{AccountID: c.identity.ID, AccountKind: c.identity.Kind, Overrides: c.owner, Tiers: make(map[agentusage.BillingTier]controller.ProviderTierLimitDTO)}
			byKey[c.key] = account
			providers[c.key] = c.provider
		}
		account.Profiles = append(account.Profiles, controller.ProviderTierProfileDTO{ID: c.profile.ID, Name: c.profile.Name, Tier: c.tier, Model: agentusage.ScopeModelID(c.profile.Model)})
		used, _ := s.readUsage(ctx, c, "", time.Now())
		account.Tiers[c.tier] = controller.ProviderTierLimitDTO{Defaults: c.defaults, Learned: c.learned.Tiers[c.tier], Override: c.owner.Tiers[c.tier], Effective: c.effective, Usage: used}
		for _, tier := range billingTiers {
			if _, ok := account.Tiers[tier]; !ok {
				defaults := agentusage.DefaultTierLimits(c.provider, tier, "", c.owner.PurchasedCredits != nil && *c.owner.PurchasedCredits)
				effective := agentusage.MergeTierLimits(agentusage.MergeTierLimits(defaults, c.learned.Tiers[tier]), c.owner.Tiers[tier])
				account.Tiers[tier] = controller.ProviderTierLimitDTO{Defaults: defaults, Learned: c.learned.Tiers[tier], Override: c.owner.Tiers[tier], Effective: effective}
			}
		}
	}
	for key, account := range byKey {
		result[providers[key]] = append(result[providers[key]], *account)
	}
	return result, nil
}

func (s *providerTierLimitService) UpdateAccount(ctx context.Context, provider, id string, config agentusage.AccountTierConfig) error {
	profiles, err := s.usage.boundProfiles(ctx)
	if err != nil {
		return err
	}
	for tier, limits := range config.Tiers {
		limits = ownerTierLimits(limits)
		config.Tiers[tier] = limits
		if !validBillingTier(tier) {
			return fmt.Errorf("%w: invalid billing tier", controller.ErrInvalidProviderLimit)
		}
		if err := agentusage.ValidateTierLimits(limits); err != nil {
			return fmt.Errorf("%w: %v", controller.ErrInvalidProviderLimit, err)
		}
	}
	for _, tier := range config.ProfileTiers {
		if !validBillingTier(tier) {
			return fmt.Errorf("%w: invalid profile tier", controller.ErrInvalidProviderLimit)
		}
	}
	for _, bound := range profiles {
		identity := accountIdentityFor(bound.binding.accountKey)
		p := dynamicruntime.ProviderOf(bound.profile.Model)
		if p == "" {
			p = strings.SplitN(bound.binding.accountKey, ":", 2)[0]
		}
		if identity.ID != id || p != provider {
			continue
		}
		allowed := make(map[string]bool)
		for _, b := range profiles {
			if b.binding.accountKey == bound.binding.accountKey {
				allowed[b.profile.ID] = true
			}
		}
		for profileID := range config.ProfileTiers {
			if !allowed[profileID] {
				return fmt.Errorf("%w: profile is not on this account", controller.ErrInvalidProviderLimit)
			}
		}
		return s.store.SaveAccountTierOverrides(ctx, bound.binding.accountKey, config)
	}
	return fmt.Errorf("%w: account unavailable", controller.ErrInvalidProviderLimit)
}

func ownerTierLimits(limits agentusage.TierLimits) agentusage.TierLimits {
	limits.Learned = false
	for i := range limits.Windows {
		limits.Windows[i].Learned = false
		limits.Windows[i].WindowEstimated = false
	}
	limits.Scopes = agentusage.OwnerScopes(limits.Scopes)
	limits.Provenance = nil
	return limits
}
