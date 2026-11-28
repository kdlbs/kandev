package backendapp

import (
	"context"
	"fmt"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"strings"
	"time"
)

const limitDimensionUnknown = "unknown"

// When the OpenRouter key-counter client is wired, a free-model daily
// exhaustion must be model-scoped so a paid OpenRouter model on the same
// credential binding is not blocked until 00:00 UTC.
func (s *providerTierLimitService) resolveFailure(ctx context.Context, candidate dynamicruntime.Candidate, failure *routingerr.Error) *routingerr.Error {
	if candidate.RemoteExecution || dynamicruntime.ProviderOf(candidate.ModelID) != "openrouter" || failure.Code != routingerr.CodeRateLimited {
		return failure
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	counter, err := s.refreshOpenRouterCounter(ctx, candidate)
	if err != nil || counter == nil {
		return failure
	}
	copy := *failure
	now := time.Now()
	until := now.Add(time.Minute)
	if reset, ok := headerResetFromFailure(failure, now); ok {
		until = reset
	}
	copy.ClassifierRule = "account_limit.minute"
	if counter.Remaining == 0 {
		copy.ClassifierRule = "account_limit.daily"
		until = now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	}
	copy.ResetHint = &until
	return &copy
}

func (s *providerTierLimitService) refreshOpenRouterCounter(ctx context.Context, candidate dynamicruntime.Candidate) (*agentusage.FreeModelCounter, error) {
	model := ""
	if candidate.UsagePolicy != nil && candidate.UsagePolicy.ProfileID == candidate.ID && candidate.ModelID == candidate.UsagePolicy.ExpectedModel && agentusage.MatchesSelectedUsage(ctx, candidate.UsagePolicy) {
		model = candidate.ModelID
	}
	_, _, binding, _, err := s.tierProfileForModel(ctx, candidate.ID, model)
	if err != nil {
		return nil, nil
	}
	free := candidate.CatalogFree || dynamicruntime.IsFreeModel(candidate.ModelID)
	if !free && binding.class != nil {
		free = binding.class(ctx) == agentusage.ModelClassFree
	}
	if !free {
		return nil, nil
	}
	if binding.client == nil {
		return nil, nil
	}
	s.usage.svc.Register(candidate.ID, binding.client, binding.cacheKey)
	observed, err := s.usage.svc.RefreshUsage(ctx, candidate.ID)
	if err != nil || observed == nil {
		return nil, err
	}
	return observed.FreeModelDailyRequests, nil
}

// classifyDimension never guesses a long window from an ambiguous plan notice.
func classifyLimitDimension(c dynamicruntime.Candidate, failure *routingerr.Error, observed *agentusage.ProviderUsage) string {
	switch dynamicruntime.FailureCauseFor(c, failure) {
	case dynamicruntime.CauseAccountMinuteLimit:
		return "minute"
	case dynamicruntime.CauseFreeDailyLimit:
		return limitDimensionModelDaily
	case dynamicruntime.CauseFreeAccountDailyLimit:
		return "daily"
	}
	if observed != nil {
		for _, w := range observed.Windows {
			if w.Exhausted() {
				if period := providerWindowPeriod(w); period != "" {
					return period
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(failure.RawExcerpt), "concurrent requests") {
		return "concurrency"
	}
	return limitDimensionUnknown
}

// recordObservation enriches and stores one observation under the learning
// lock, so a near-simultaneous block of another model sees this one when it
// looks for a same-minute partner.
func (s *providerTierLimitService) recordObservation(ctx context.Context, candidate dynamicruntime.Candidate, failure *routingerr.Error, observation *sqliterepo.UsageLimitObservation) error {
	s.learningMu.Lock()
	defer s.learningMu.Unlock()
	if err := s.enrichObservation(ctx, candidate, failure, observation); err != nil {
		return err
	}
	return s.store.InsertUsageLimitObservation(ctx, *observation)
}

// enrichObservation runs under learningMu (see recordObservation).
func (s *providerTierLimitService) enrichObservation(ctx context.Context, candidate dynamicruntime.Candidate, failure *routingerr.Error, observation *sqliterepo.UsageLimitObservation) error {
	if candidate.RemoteExecution {
		return nil
	}
	c, err := s.observationContext(ctx, candidate)
	if err != nil {
		return err
	}
	used, err := s.readUsage(ctx, c, "", observation.ObservedAt)
	if err != nil {
		return err
	}
	dimension := classifyLimitDimension(candidate, failure, c.observed)
	observation.Tier = string(c.tier)
	observation.Dimension = dimension
	observation.ModelID = c.profile.Model
	observation.RequestsMinute = used.RequestsMinute
	observation.RequestsDay = used.RequestsDay
	observation.ActiveTurns = used.ActiveTurns
	if c.learned.Tiers == nil {
		c.learned.Tiers = make(map[agentusage.BillingTier]agentusage.TierLimits)
	}

	windows, err := s.snapshotLimitWindows(ctx, c, observation.ObservedAt, observation)
	if err != nil {
		return err
	}
	if dimension == limitDimensionModelDaily {
		return s.learnModelScope(ctx, c, observation.ObservedAt)
	}
	used.Windows = learningTokenWindows(windows)
	base := agentusage.MergeTierLimits(c.defaults, c.learned.Tiers[c.tier])
	learned := agentusage.LearnTierLimits(base, learningDimensionFor(c.tier, dimension), used)

	if !learned.Learned {
		return nil
	}
	c.learned.Tiers[c.tier] = learnedOnly(c.defaults, c.learned.Tiers[c.tier], learned)

	return s.store.SaveAccountTierLearned(ctx, c.key, c.learned)
}

// observationContext resolves the allowance account a limit observation
// belongs to and verifies it matches the execution's selected usage binding.
func (s *providerTierLimitService) observationContext(ctx context.Context, candidate dynamicruntime.Candidate) (tierLimitContext, error) {
	policy := candidate.UsagePolicy
	if candidate.RequireExecutionUsage && policy == nil {
		return tierLimitContext{}, fmt.Errorf("execution allowance account is unknown")
	}
	if policy != nil && policy.ProfileID != candidate.ID {
		return tierLimitContext{}, fmt.Errorf("execution allowance profile does not match candidate")
	}
	model := ""
	if policy != nil && agentusage.MatchesSelectedUsage(ctx, policy) {
		model = policy.ExpectedModel
	}
	c, err := s.contextForModel(ctx, candidate.ID, model)
	if err != nil {
		return tierLimitContext{}, err
	}
	if policy != nil && !tierContextMatchesSelection(c, policy) {
		return tierLimitContext{}, fmt.Errorf("configured allowance account does not match execution selection")
	}
	return c, nil
}

func learnedOnly(defaults, prior, learned agentusage.TierLimits) agentusage.TierLimits {
	if defaults.RequestsPerMinute == nil {
		prior.RequestsPerMinute = learned.RequestsPerMinute
	}
	if defaults.RequestsPerDay == nil {
		prior.RequestsPerDay = learned.RequestsPerDay
		prior.DailyReset = learned.DailyReset
	}
	if defaults.MaxConcurrentTurns == nil {
		prior.MaxConcurrentTurns = learned.MaxConcurrentTurns
	}
	known := make(map[string]bool)
	for _, budget := range defaults.Windows {
		known[budget.Key()] = true
	}
	for _, budget := range learned.Windows {
		if !known[budget.Key()] {
			prior = agentusage.MergeTierLimits(prior, agentusage.TierLimits{Windows: []agentusage.LimitBudget{budget}})
		}
	}
	prior.Learned = true
	return prior
}
