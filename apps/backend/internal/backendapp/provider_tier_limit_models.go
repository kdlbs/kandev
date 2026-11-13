package backendapp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type modelWindowUsageReader interface {
	GetManualModelWindowUsageForProfiles(context.Context, []string, time.Time, time.Time) ([]sqliterepo.ManualModelWindowUsage, error)
}

type selectedModelWindowUsageReader interface {
	GetManualSelectedModelWindowUsage(context.Context, []string, time.Time, time.Time, func(string, string) (bool, error)) (sqliterepo.ManualWindowUsage, error)
}

type modelProfileSnapshot struct {
	profile   settingsmodels.AgentProfile
	agentType string
}

// readModelBudgetUsage keeps each recorded model paired with its profile. A
// turn spanning multiple selected models cannot be summed without double count.
func (s *providerTierLimitService) readModelBudgetUsage(ctx context.Context, c tierLimitContext, budget agentusage.LimitBudget, start, end time.Time) (float64, bool, error) {
	if budget.Unit == "turns" {
		return s.readDistinctModelBudgetTurns(ctx, c, budget, start, end)
	}
	reader, ok := s.store.(modelWindowUsageReader)
	if !ok {
		return 0, false, errModelAttributionUnavailable
	}
	groups, err := reader.GetManualModelWindowUsageForProfiles(ctx, c.ids, start, end)
	if err != nil {
		return 0, false, err
	}
	selected := make(map[string]int)
	var amount float64
	for _, group := range groups {
		if strings.TrimSpace(group.Model) == "" && group.Usage.EventCount > 0 {
			return 0, false, errModelAttributionUnavailable
		}
		matches, err := s.recordedModelMatchesScope(ctx, c, group.ProfileID, group.Model, budget.Scope)
		if err != nil {
			return 0, false, err
		}
		if !matches {
			continue
		}
		selected[group.ProfileID]++
		amount += budgetAmount(group.Usage, budget.Unit)
	}
	return amount, len(selected) > 0, nil
}

func (s *providerTierLimitService) readDistinctModelBudgetTurns(ctx context.Context, c tierLimitContext, budget agentusage.LimitBudget, start, end time.Time) (float64, bool, error) {
	reader, ok := s.store.(selectedModelWindowUsageReader)
	if !ok {
		return 0, false, errModelAttributionUnavailable
	}
	profiles := make(map[string]modelProfileSnapshot, len(c.ids))
	for _, id := range c.ids {
		profile, agentType, found := s.usage.loadProfile(ctx, id)
		if !found || profile == nil {
			return 0, false, errModelAttributionUnavailable
		}
		profiles[id] = modelProfileSnapshot{profile: *profile, agentType: agentType}
	}
	selected := false
	usage, err := reader.GetManualSelectedModelWindowUsage(ctx, c.ids, start, end, func(id, model string) (bool, error) {
		profile, found := profiles[id]
		if !found || strings.TrimSpace(model) == "" {
			return false, errModelAttributionUnavailable
		}
		matches, matchErr := s.modelSnapshotMatchesScope(ctx, c, profile, model, budget.Scope)
		if matchErr != nil {
			return false, matchErr
		}
		selected = selected || matches
		return matches, nil
	})
	return float64(usage.TurnCount), selected, err
}

func (s *providerTierLimitService) recordedModelMatchesScope(ctx context.Context, c tierLimitContext, profileID, model, scope string) (bool, error) {
	profile, agentType, ok := s.usage.loadProfile(ctx, profileID)
	if !ok || profile == nil {
		return false, errModelAttributionUnavailable
	}
	return s.modelSnapshotMatchesScope(ctx, c, modelProfileSnapshot{profile: *profile, agentType: agentType}, model, scope)
}

func (s *providerTierLimitService) modelSnapshotMatchesScope(ctx context.Context, c tierLimitContext, snapshot modelProfileSnapshot, model, scope string) (bool, error) {
	copy := snapshot.profile
	copy.Model = model
	bound, ok := s.usage.bindingFor(&copy, snapshot.agentType)
	if !ok || bound.accountKey != c.key {
		return false, errModelAttributionUnavailable
	}
	class := modelClassForOverride(ctx, profileUsage{Usage: c.observed}, bound)
	free := class == agentusage.ModelClassFree || dynamicruntime.IsFreeModel(model)
	if s.freeModels != nil {
		if catalogFree, known := s.freeModels.IsFreeModel(ctx, model); known && catalogFree {
			free = true
		}
	}
	return matchesRecordedModelScope(scope, free, class == agentusage.ModelClassPremium), nil
}

func matchesRecordedModelScope(scope string, free, premium bool) bool {
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

var errModelAttributionUnavailable = errors.New("model-attributed usage is unavailable")

// provenAccountLimit uses only account-wide evidence already read before an
// attribution failure. It never treats an unknown model scope as exhausted.
func provenAccountLimit(limits agentusage.TierLimits, used agentusage.LimitUsage, now time.Time) agentusage.LimitAdmission {
	limits.Scopes = nil
	if limits.RequestsPerModel {
		limits.RequestsPerMinute = nil
		limits.RequestsPerWindow = nil
	}
	windows := make([]agentusage.LimitBudget, 0, len(limits.Windows))
	for _, budget := range limits.Windows {
		if budget.Scope == "" {
			windows = append(windows, budget)
		}
	}
	limits.Windows = windows
	return agentusage.EvaluateTierLimits(limits, used, now)
}

// readModelRequestCount never assigns a missing model to the profile's
// current default. Recorded and live observations remain separate lower bounds.
func (s *providerTierLimitService) readModelRequestCount(ctx context.Context, ids, models []string, start, end time.Time) (float64, error) {
	reader, ok := s.store.(modelWindowUsageReader)
	if !ok {
		return 0, errModelAttributionUnavailable
	}
	if len(ids) == 0 {
		return 0, nil
	}
	groups, err := reader.GetManualModelWindowUsageForProfiles(ctx, ids, start, end)
	if err != nil {
		return 0, err
	}
	var recorded float64
	for _, group := range groups {
		if strings.TrimSpace(group.Model) == "" && group.Usage.EventCount > 0 {
			return 0, fmt.Errorf("%w: recorded request model is unknown", errModelAttributionUnavailable)
		}
		for _, wanted := range models {
			if strings.EqualFold(agentusage.ScopeModelID(group.Model), agentusage.ScopeModelID(wanted)) {
				recorded += float64(group.Usage.EventCount)
				break
			}
		}
	}
	live, known := s.requests.countModels(ids, models, start, end)
	if !known {
		return 0, fmt.Errorf("%w: live request model is unknown", errModelAttributionUnavailable)
	}
	return math.Max(recorded, live), nil
}
