package backendapp

import (
	"context"
	"encoding/json"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"math"
	"time"
)

type observedTierWindow struct {
	Requests     float64 `json:"requests"`
	Turns        int64   `json:"turns"`
	Tokens       int64   `json:"tokens"`
	CostSubcents int64   `json:"cost_subcents"`
}

func (s *providerTierLimitService) snapshotLimitWindows(ctx context.Context, c tierLimitContext, at time.Time, observation *sqliterepo.UsageLimitObservation) (map[string]observedTierWindow, error) {
	windows := make(map[string]observedTierWindow)
	for _, period := range []string{"5h", "day", "week", "month"} {
		start, _, _ := agentusage.LimitWindowBounds(at, agentusage.LimitBudget{Window: period, Reset: "rolling"})
		if period == "day" {
			start, _, _ = agentusage.DailyLimitWindow(at, c.effective.DailyReset)
		}
		if c.observed != nil {
			for _, window := range c.observed.Windows {
				if providerWindowPeriod(window) == period && !window.StartAt.IsZero() && window.ResetAt.After(at) {
					start = window.StartAt
					break
				}
			}
		}
		total, err := s.store.GetManualWindowUsageForProfiles(ctx, c.ids, start, at)
		if err != nil {
			return nil, err
		}
		windows[period] = observedTierWindow{Requests: math.Max(float64(total.EventCount), s.requests.count(c.ids, start, at)), Turns: total.TurnCount, Tokens: total.TokensTotal, CostSubcents: total.CostSubcents}
	}
	observation.Turns5h, observation.Tokens5h = windows["5h"].Turns, windows["5h"].Tokens
	observation.TurnsDay, observation.TokensDay, observation.CostSubcentsDay = windows["day"].Turns, windows["day"].Tokens, windows["day"].CostSubcents
	observation.TurnsWeek, observation.TokensWeek = windows["week"].Turns, windows["week"].Tokens
	encoded, err := json.Marshal(windows)
	observation.WindowUsageJSON = string(encoded)
	return windows, err
}

func learningDimensionFor(tier agentusage.BillingTier, dimension string) string {
	if tier == agentusage.TierSubscription && dimension == limitDimensionUnknown {
		return "unknown_subscription"
	}
	return dimension
}

func learningTokenWindows(windows map[string]observedTierWindow) map[string]float64 {
	values := make(map[string]float64)
	for period, w := range windows {
		values[period] = float64(w.Tokens)
	}
	return values
}
