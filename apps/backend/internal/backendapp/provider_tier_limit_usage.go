package backendapp

import (
	"context"
	"math"
	"strings"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
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
	if err := s.readRequestUsage(ctx, c, now, &used); err != nil {
		return used, err
	}
	for _, budget := range c.effective.Windows {
		amount, reset, known, err := s.readBudgetUsage(ctx, c, budget, now)
		if err != nil {
			return used, err
		}
		if known {
			used.Windows[budget.Key()] = amount
			used.Resets[budget.Key()] = reset
		}
	}
	if err := s.readScopeUsage(ctx, c, now, &used); err != nil {
		return used, err
	}
	return used, nil
}

func (s *providerTierLimitService) readRequestUsage(ctx context.Context, c tierLimitContext, now time.Time, used *agentusage.LimitUsage) error {
	ids := s.minuteProfiles(ctx, c)
	var err error
	used.RequestsMinute, err = s.readMinuteRequestCount(ctx, c, ids, now.Add(-time.Minute), now)
	if err != nil {
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
		used.RequestsWindow, err = s.readMinuteRequestCount(ctx, c, ids, now.Add(-time.Duration(c.effective.RequestWindowSeconds)*time.Second), now)
		if err != nil {
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

func (s *providerTierLimitService) readMinuteRequestCount(ctx context.Context, c tierLimitContext, ids []string, start, end time.Time) (float64, error) {
	if c.modelOverride && c.effective.RequestsPerModel {
		return s.readModelRequestCount(ctx, c.ids, []string{c.profile.Model}, start, end)
	}
	return s.readRequestCount(ctx, ids, start, end)
}

func (s *providerTierLimitService) minuteProfiles(ctx context.Context, c tierLimitContext) []string {
	if !c.effective.RequestsPerModel {
		return c.ids
	}
	var ids []string
	for _, id := range c.ids {
		p, _, ok := s.usage.loadProfile(ctx, id)
		if ok && p.Model == c.profile.Model {
			ids = append(ids, id)
		}
	}
	return ids
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
	if c.modelOverride && budget.Scope != "" {
		amount, known, err := s.readModelBudgetUsage(ctx, c, budget, start, now)
		return amount, reset, known, err
	}
	ids := s.budgetProfileIDs(ctx, c, budget)
	if len(ids) == 0 {
		return 0, time.Time{}, false, nil
	}
	total, err := s.store.GetManualWindowUsageForProfiles(ctx, ids, start, now)
	return budgetAmount(total, budget.Unit), reset, true, err
}

// readPercentBudget returns a provider-reported utilization and the reset its
// owner anchor selects, when one is configured.
func (s *providerTierLimitService) readPercentBudget(c tierLimitContext, budget agentusage.LimitBudget, now time.Time) (float64, time.Time, bool, error) {
	window, ok := findProviderBudget(c.observed, budget, c.profile.Model)
	if !ok {
		return 0, time.Time{}, false, nil
	}
	if !window.ResetAt.IsZero() && !window.ResetAt.After(now) {
		return 0, time.Time{}, false, nil
	}
	amount := window.UtilizationPct
	if window.LimitReached {
		amount = math.Max(100, amount)
	}
	reset := window.ResetAt
	if budget.Reset == "anchor" || (budget.Reset == tierResetProvider && budget.ResetAt != nil) {
		if _, ownerReset, ok := agentusage.LimitWindowBounds(now, budget); ok && ownerReset.After(now) {
			reset = ownerReset
		}
	}
	return amount, reset, true, nil
}

func findProviderBudget(observed *agentusage.ProviderUsage, b agentusage.LimitBudget, model string) (agentusage.UtilizationWindow, bool) {
	if observed == nil {
		return agentusage.UtilizationWindow{}, false
	}
	scopeModel := agentusage.ScopeModelID(model)
	var accountWide *agentusage.UtilizationWindow
	for index := range observed.Windows {
		window := observed.Windows[index]
		if providerWindowPeriod(window) != b.Window || string(window.Scope) != b.Scope {
			continue
		}
		if window.ModelID != "" {
			if scopeModel != "" && strings.EqualFold(agentusage.ScopeModelID(window.ModelID), scopeModel) {
				return window, true
			}
			continue
		}
		if accountWide == nil {
			windowCopy := window
			accountWide = &windowCopy
		}
	}
	if accountWide != nil {
		return *accountWide, true
	}
	return agentusage.UtilizationWindow{}, false
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

func (s *providerTierLimitService) budgetProfileIDs(ctx context.Context, c tierLimitContext, b agentusage.LimitBudget) []string {
	if b.Scope == "" {
		return c.ids
	}
	var ids []string
	for _, id := range c.ids {
		if s.profileMatchesScope(ctx, id, b.Scope) {
			ids = append(ids, id)
		}
	}
	return ids
}

// profileMatchesScope keeps a model-class budget's usage to the profiles of
// that class, so one class cannot consume another class's allowance.
func (s *providerTierLimitService) profileMatchesScope(ctx context.Context, id, scope string) bool {
	class := s.usage.ProfileUsage(ctx, id).ModelClass
	model := ""
	if profile, _, ok := s.usage.loadProfile(ctx, id); ok && profile != nil {
		model = profile.Model
	}
	free := class == agentusage.ModelClassFree || dynamicruntime.IsFreeModel(model)
	if s.freeModels != nil {
		if catalogFree, known := s.freeModels.IsFreeModel(ctx, model); known && catalogFree {
			free = true
		}
	}
	premium := class == agentusage.ModelClassPremium
	switch scope {
	case string(agentusage.WindowScopeFreeModels):
		return free
	case string(agentusage.WindowScopePremiumModels):
		return premium
	case string(agentusage.WindowScopePaidModels):
		return !free && !premium
	default:
		return true
	}
}

func (s *providerTierLimitService) windowReset(ctx context.Context, id, period string, now time.Time, exhaustedOnly bool) (time.Time, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := s.contextFor(ctx, id)
	if err != nil {
		return time.Time{}, false
	}
	used, err := s.readUsage(ctx, c, "", now)
	if err != nil {
		return time.Time{}, false
	}
	var latest time.Time
	for _, budget := range c.effective.Windows {
		if period != "" && budget.Window != period {
			continue
		}
		if exhaustedOnly && (budget.Value == nil || *budget.Value <= 0 || used.Windows[budget.Key()] < *budget.Value) {
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
