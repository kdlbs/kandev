package costs

import (
	"context"
	"errors"
	"time"
)

// ErrInvalidCostRange identifies unsupported composed-breakdown range values.
var ErrInvalidCostRange = errors.New("invalid cost range: expected mtd or 30d")

const (
	costRangeMTD    = "mtd"
	costRange30Days = "30d"
)

type costWindow struct{ start, before time.Time }

func validCostRange(value string) bool { return value == costRangeMTD || value == costRange30Days }

func costRangeWindow(value string, now time.Time) (*costWindow, error) {
	if value == "" {
		return nil, nil
	}
	if !validCostRange(value) {
		return nil, ErrInvalidCostRange
	}
	now = now.UTC()
	start := now.Add(-30 * 24 * time.Hour)
	if value == costRangeMTD {
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return &costWindow{start: start, before: now}, nil
}

// GetCostsBreakdownForRange resolves one event-time interval for all five reads.
// An empty range retains the lifetime contract of GetCostsBreakdown.
func (s *CostService) GetCostsBreakdownForRange(
	ctx context.Context, wsID, value string,
) (total int64, byAgent, byProject, byModel, byProvider []*CostBreakdown, err error) {
	window, err := costRangeWindow(value, time.Now())
	if err != nil {
		return 0, nil, nil, nil, nil, err
	}
	return s.getCostsBreakdown(ctx, wsID, window)
}

func (s *CostService) breakdownReaders(wsID string, window *costWindow) (
	func(context.Context) (int64, error), []func(context.Context) ([]*CostBreakdown, error),
) {
	if window == nil {
		return func(ctx context.Context) (int64, error) { return s.repo.SumCosts(ctx, wsID) },
			[]func(context.Context) ([]*CostBreakdown, error){
				func(ctx context.Context) ([]*CostBreakdown, error) { return s.repo.GetCostsByAgent(ctx, wsID) },
				func(ctx context.Context) ([]*CostBreakdown, error) { return s.repo.GetCostsByProject(ctx, wsID) },
				func(ctx context.Context) ([]*CostBreakdown, error) { return s.repo.GetCostsByModel(ctx, wsID) },
				func(ctx context.Context) ([]*CostBreakdown, error) { return s.repo.GetCostsByProvider(ctx, wsID) },
			}
	}
	return func(ctx context.Context) (int64, error) {
			return s.repo.SumCostsInWindow(ctx, wsID, window.start, window.before)
		},
		[]func(context.Context) ([]*CostBreakdown, error){
			func(ctx context.Context) ([]*CostBreakdown, error) {
				return s.repo.GetCostsByAgentInWindow(ctx, wsID, window.start, window.before)
			},
			func(ctx context.Context) ([]*CostBreakdown, error) {
				return s.repo.GetCostsByProjectInWindow(ctx, wsID, window.start, window.before)
			},
			func(ctx context.Context) ([]*CostBreakdown, error) {
				return s.repo.GetCostsByModelInWindow(ctx, wsID, window.start, window.before)
			},
			func(ctx context.Context) ([]*CostBreakdown, error) {
				return s.repo.GetCostsByProviderInWindow(ctx, wsID, window.start, window.before)
			},
		}
}
