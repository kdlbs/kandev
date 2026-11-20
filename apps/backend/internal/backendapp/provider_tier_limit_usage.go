package backendapp

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func (s *providerTierLimitService) readUsage(ctx context.Context, c tierLimitContext, exclude string, now time.Time) (agentusage.LimitUsage, error) {
	used := agentusage.LimitUsage{Windows: make(map[string]float64), Resets: make(map[string]time.Time), Estimated: true}
	active, err := s.store.CountLimitActiveTurns(ctx, c.ids, exclude)
	if err != nil {
		return used, err
	}
	used.ActiveTurns = active
	// A model-attribution failure is returned only after the account-wide
	// usage is read, so provenAccountLimit can still apply that evidence.
	var attribution error
	if err := s.readRequestUsage(ctx, c, now, &used, &attribution); err != nil {
		return used, err
	}
	for _, budget := range c.effective.Windows {
		amount, reset, known, err := s.readBudgetUsage(ctx, c, budget, now)
		if err != nil {
			if err := deferAttribution(&attribution, err); err != nil {
				return used, err
			}
			continue
		}
		if known {
			used.Windows[budget.Key()] = amount
			used.Resets[budget.Key()] = reset
		}
	}
	if err := deferAttribution(&attribution, s.readScopeUsage(ctx, c, now, &used)); err != nil {
		return used, err
	}
	return used, attribution
}

// deferAttribution keeps the first model-attribution failure in first and
// returns every other error unchanged.
func deferAttribution(first *error, err error) error {
	if !errors.Is(err, errModelAttributionUnavailable) {
		return err
	}
	if *first == nil {
		*first = err
	}
	return nil
}

func (s *providerTierLimitService) readRequestUsage(ctx context.Context, c tierLimitContext, now time.Time, used *agentusage.LimitUsage, attribution *error) error {
	var err error
	used.RequestsMinute, err = s.readMinuteRequestCount(ctx, c, now.Add(-time.Minute), now)
	if err := deferAttribution(attribution, err); err != nil {
		return err
	}
	start, reset, err := agentusage.DailyLimitWindow(now, c.effective.DailyReset)
	if err != nil {
		return err
	}
	day, err := s.store.GetManualWindowUsageForProfiles(ctx, c.ids, start, now)
	if err != nil {
		return err
	}
	used.RequestsDay = math.Max(float64(day.EventCount), s.requests.count(c.ids, start, now))
	used.TurnsDay = day.TurnCount
	used.TokensDay = day.TokensTotal
	used.Resets["day"] = reset
	if c.effective.RequestWindowSeconds > 0 {
		used.RequestsWindow, err = s.readMinuteRequestCount(ctx, c, now.Add(-time.Duration(c.effective.RequestWindowSeconds)*time.Second), now)
		if err := deferAttribution(attribution, err); err != nil {
			return err
		}
	}
	if c.tier == agentusage.TierFree && c.observed != nil && c.observed.FreeModelDailyRequests != nil && !c.observed.FetchedAt.Before(start) {
		counter := c.observed.FreeModelDailyRequests
		used.RequestsDay = counter.Used
		if counter.Remaining == 0 && c.effective.RequestsPerDay != nil {
			used.RequestsDay = math.Max(used.RequestsDay, *c.effective.RequestsPerDay)
		}
	}
	return nil
}

// readMinuteRequestCount counts a per-model request limit from the recorded
// model of each request across the account's profiles, never from a
// profile's current default model.
func (s *providerTierLimitService) readMinuteRequestCount(ctx context.Context, c tierLimitContext, start, end time.Time) (float64, error) {
	if c.effective.RequestsPerModel {
		return s.readModelRequestCount(ctx, c.ids, []string{c.profile.Model}, start, end)
	}
	return s.readRequestCount(ctx, c.ids, start, end)
}

func (s *providerTierLimitService) readRequestCount(ctx context.Context, ids []string, start, end time.Time) (float64, error) {
	totals, err := s.store.GetManualWindowUsageForProfiles(ctx, ids, start, end)
	return math.Max(float64(totals.EventCount), s.requests.count(ids, start, end)), err
}

func (s *providerTierLimitService) readBudgetUsage(ctx context.Context, c tierLimitContext, budget agentusage.LimitBudget, now time.Time) (float64, time.Time, bool, error) {
	if budget.Unit == "percent" {
		return s.readPercentBudget(c, budget, now)
	}
	start, reset, ok := agentusage.LimitWindowBounds(now, budget)
	if budget.Reset == tierResetProvider {
		if window, found := findProviderBudget(c.observed, budget, c.profile.Model); found && !window.StartAt.IsZero() && window.ResetAt.After(now) {
			start, reset, ok = window.StartAt, window.ResetAt, true
		}
	}
	if !ok {
		return 0, time.Time{}, false, nil
	}
	if budget.Scope != "" {
		amount, known, err := s.readModelBudgetUsage(ctx, c, budget, start, now)
		return amount, reset, known, err
	}
	total, err := s.store.GetManualWindowUsageForProfiles(ctx, c.ids, start, now)
	return budgetAmount(total, budget.Unit), reset, true, err
}

// readPercentBudget returns the highest provider-reported utilization among the
// shared account window and the windows of the selected model, and the reset
// its owner anchor selects, when one is configured. When several applicable
// windows are exhausted, the latest of their resets applies.
func (s *providerTierLimitService) readPercentBudget(c tierLimitContext, budget agentusage.LimitBudget, now time.Time) (float64, time.Time, bool, error) {
	amount, reset, known := readProviderPercentWindows(c, budget, now)
	if !known {
		return 0, time.Time{}, false, nil
	}
	if budget.Reset == "anchor" || (budget.Reset == tierResetProvider && budget.ResetAt != nil) {
		if _, ownerReset, ok := agentusage.LimitWindowBounds(now, budget); ok && ownerReset.After(now) {
			reset = ownerReset
		}
	}
	return amount, reset, true, nil
}

func readProviderPercentWindows(c tierLimitContext, budget agentusage.LimitBudget, now time.Time) (float64, time.Time, bool) {
	if c.observed == nil {
		return 0, time.Time{}, false
	}
	var amount float64
	var reset, exhaustedUntil time.Time
	known := false
	for _, window := range c.observed.Windows {
		if !providerPercentWindowApplies(window, budget, c.profile.Model, now) {
			continue
		}
		pct := window.UtilizationPct
		if window.LimitReached {
			pct = math.Max(100, pct)
		}
		if !known || pct > amount || (pct == amount && window.ResetAt.After(reset)) {
			amount, reset, known = pct, window.ResetAt, true
		}
		if c.effective.WindowBudgetExhausted(budget, pct) && window.ResetAt.After(exhaustedUntil) {
			exhaustedUntil = window.ResetAt
		}
	}
	if !exhaustedUntil.IsZero() {
		reset = exhaustedUntil
	}
	return amount, reset, known
}

// providerPercentWindowApplies accepts an unexpired window of the budget's
// period and scope that is shared, or scoped to the selected model or its
// recognized family.
func providerPercentWindowApplies(w agentusage.UtilizationWindow, b agentusage.LimitBudget, model string, now time.Time) bool {
	if providerWindowPeriod(w) != b.Window || string(w.Scope) != b.Scope ||
		(!w.ResetAt.IsZero() && !w.ResetAt.After(now)) {
		return false
	}
	if w.ModelID != "" {
		return exactProviderBudgetModel(w.ModelID, model)
	}
	return w.Model == "" || exactProviderBudgetModel(w.Model, model) || knownProviderBudgetFamily(w, model)
}

// findProviderBudget selects one window as a reset anchor: the exact model,
// then a recognized model family, then the shared account window.
func findProviderBudget(observed *agentusage.ProviderUsage, b agentusage.LimitBudget, model string) (agentusage.UtilizationWindow, bool) {
	if observed == nil {
		return agentusage.UtilizationWindow{}, false
	}
	var shared, family agentusage.UtilizationWindow
	foundShared, foundFamily := false, false
	for _, w := range observed.Windows {
		if providerWindowPeriod(w) != b.Window || string(w.Scope) != b.Scope {
			continue
		}
		if w.ModelID != "" {
			if exactProviderBudgetModel(w.ModelID, model) {
				return w, true
			}
			continue
		}
		if w.Model != "" {
			if exactProviderBudgetModel(w.Model, model) {
				return w, true
			}
			if !foundFamily && knownProviderBudgetFamily(w, model) {
				family, foundFamily = w, true
			}
			continue
		}
		if !foundShared {
			shared, foundShared = w, true
		}
	}
	if foundFamily {
		return family, true
	}
	return shared, foundShared
}

func knownProviderBudgetFamily(window agentusage.UtilizationWindow, model string) bool {
	switch strings.ToLower(strings.TrimSpace(window.Model)) {
	case "opus", "sonnet", "haiku":
		return model != "" && agentusage.ExecutionUsageWindowApplies(window, model)
	default:
		return false
	}
}

func exactProviderBudgetModel(recorded, model string) bool {
	if model == "" {
		return false
	}
	recordedProvider, _, recordedQualified := strings.Cut(recorded, "/")
	modelProvider, _, modelQualified := strings.Cut(model, "/")
	if recordedQualified && modelQualified && !strings.EqualFold(recordedProvider, modelProvider) {
		return false
	}
	return strings.EqualFold(agentusage.ScopeModelID(recorded), agentusage.ScopeModelID(model))
}

func budgetAmount(total sqliterepo.ManualWindowUsage, unit string) float64 {
	switch unit {
	case "usd":
		return float64(total.CostSubcents) / subcentsPerDollar
	case "tokens":
		return float64(total.TokensTotal)
	case "turns":
		return float64(total.TurnCount)
	default:
		return float64(total.EventCount)
	}
}

func (s *providerTierLimitService) windowReset(ctx context.Context, id, period string, now time.Time, exhaustedOnly bool) (time.Time, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := s.contextFor(ctx, id)
	if err != nil {
		return time.Time{}, false
	}
	if exhaustedOnly {
		c.effective.Windows = applicableBudgets(c.effective.Windows, c.profile.Model, c.tier, c.modelClass)
	}
	// An attribution failure leaves only that model-scoped usage unknown; the
	// windows that were read still give their resets.
	used, err := s.readUsage(ctx, c, "", now)
	unknownModel := errors.Is(err, errModelAttributionUnavailable)
	if err != nil && !unknownModel {
		return time.Time{}, false
	}
	var latest time.Time
	for _, budget := range c.effective.Windows {
		if !budgetContributesReset(c.effective, budget, used, period, exhaustedOnly, unknownModel) {
			continue
		}
		reset := used.Resets[budget.Key()]
		if !reset.After(now) {
			_, reset, _ = agentusage.LimitWindowBounds(now, budget)
		}
		if reset.After(latest) {
			latest = reset
		}
	}
	return latest, latest.After(now)
}

// budgetContributesReset selects the budgets whose reset bounds a suspension.
// An exhausted-only reset needs known usage at the budget's threshold; without
// model attribution a model-scoped budget cannot supply a reset.
func budgetContributesReset(limits agentusage.TierLimits, budget agentusage.LimitBudget, used agentusage.LimitUsage, period string, exhaustedOnly, unknownModel bool) bool {
	if period != "" && budget.Window != period {
		return false
	}
	if exhaustedOnly {
		amount, known := used.Windows[budget.Key()]
		return known && limits.WindowBudgetExhausted(budget, amount)
	}
	return !unknownModel || budget.Scope == ""
}
