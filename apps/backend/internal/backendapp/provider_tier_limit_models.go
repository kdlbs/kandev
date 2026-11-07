package backendapp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type modelWindowUsageReader interface {
	GetManualModelWindowUsageForProfiles(context.Context, []string, time.Time, time.Time) ([]sqliterepo.ManualModelWindowUsage, error)
}

// readModelBudgetUsage keeps each recorded model paired with its profile. A
// turn spanning multiple selected models cannot be summed without double count.
func (s *providerTierLimitService) readModelBudgetUsage(ctx context.Context, c tierLimitContext, budget agentusage.LimitBudget, start, end time.Time) (float64, bool, error) {
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
		if budget.Unit == "turns" && selected[group.ProfileID] > 1 {
			return 0, false, errModelAttributionUnavailable
		}
		amount += budgetAmount(group.Usage, budget.Unit)
	}
	return amount, len(selected) > 0, nil
}

func (s *providerTierLimitService) recordedModelMatchesScope(ctx context.Context, c tierLimitContext, profileID, model, scope string) (bool, error) {
	profile, agentType, ok := s.usage.loadProfile(ctx, profileID)
	if !ok || profile == nil {
		return false, errModelAttributionUnavailable
	}
	copy := *profile
	copy.Model = model
	bound, ok := s.usage.bindingFor(&copy, agentType)
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
