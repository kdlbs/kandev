package dynamic

import (
	"context"
	"errors"
)

func supportedTierMode(mode string, usageAvailable bool) bool {
	if mode == orderedTierMode || mode == tierModeCost || mode == tierModeRandom || mode == tierModeRoundRobin {
		return true
	}
	return usageAvailable && usageTierMode(mode)
}

// The engine mutex encloses this read, ranking and the existing decision
// transaction. Other backend processes may independently rank the same history.
func (e *Engine) tierSelectionHistory(ctx context.Context, profileID string, tiers []orderedTier) (map[string]int64, error) {
	for _, tier := range tiers {
		if tier.mode != tierModeRoundRobin {
			continue
		}
		reader, ok := e.persistence.(RouteSelectionHistoryReader)
		if !ok {
			return nil, errors.New("round-robin selection history unavailable")
		}
		return reader.LastRouteSelectionOrdinals(ctx, profileID)
	}
	return nil, nil
}

// Ordinals select the last current member, including a now-disabled member.
// Rotation follows saved adjacency; eligibility and preferred retry stay intact.
func rotateRoundRobinRows(rows []rankedTierRow, history map[string]int64) []rankedTierRow {
	last, ordinal := -1, int64(0)
	for index, row := range rows {
		if value := history[row.candidate.ID]; value > ordinal {
			last, ordinal = index, value
		}
	}
	if last < 0 || last == len(rows)-1 {
		return rows
	}
	rotated := make([]rankedTierRow, 0, len(rows))
	rotated = append(rotated, rows[last+1:]...)
	return append(rotated, rows[:last+1]...)
}
