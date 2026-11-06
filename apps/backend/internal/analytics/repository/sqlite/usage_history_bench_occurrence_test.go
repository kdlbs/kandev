//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Dense external observations need distinct coverage identities. Four fixed
// times per day collide even though source IDs and record revisions differ.
func TestUsageHistoryBenchmarkSourceIdentities(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	const count = 1464
	seen := map[string]bool{}
	previous := start.Add(-time.Microsecond)
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{})
	for index := range count {
		at := usageBenchOccurrence(start, end, index, count)
		require.False(t, at.Before(start))
		require.True(t, at.Before(end))
		require.True(t, at.After(previous))
		subject := fixture.subjects[4]
		measurement := usageBenchMeasurement(subject, "external-"+strconv.Itoa(index), at, "plugin:bench")
		row := fixture.seedRecord(t, subject, measurement).observation
		state := usageSegmentReduction{Choice: usageSegmentChoice{Tokens: row.TokenBundle, Cost: row.CostBundle}, Group: row.GroupKey}
		state.add(row)
		fact, keep := state.fact()
		require.True(t, keep)
		require.False(t, seen[fact.Key], "distinct external observations must not collide")
		seen[fact.Key] = true
		previous = at
	}
	require.Len(t, seen, count)
}
