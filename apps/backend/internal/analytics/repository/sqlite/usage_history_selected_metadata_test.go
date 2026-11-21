package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryUnsegmentedSelectedCostMetadata(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, costStale := range []bool{true, false} {
			t.Run(fmt.Sprintf("postgres=%t/selected_cost_stale=%t", postgres, costStale), func(t *testing.T) {
				testUnsegmentedSelectedCostMetadata(t, postgres, costStale)
			})
		}
	}
}

func testUnsegmentedSelectedCostMetadata(t *testing.T, postgres, costStale bool) {
	t.Helper()
	repo := segmentContractFixture(t, postgres)
	retainSegmentNative(t, repo, 100, func(native *usagehistory.NativeUsage) {
		native.ReportedTotalTokens, native.CostPresent, native.CostSubcents = models.Int64(100), true, 10
		native.CostSource, native.CostBasis, native.Estimated = usageCostSourceEstimated, models.UsageCostBasisEstimated, true
	})
	item := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-a", "session-1"), "fresh-reported-price", "Sonnet", "anthropic", 100, false)
	item.Segment, item.WorkIdentity = nil, &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: "segment-native-event"}
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr("2026-01-02T12:00:00Z")}
	item.Cost.Amount, item.Cost.Basis, item.Quality.Stale = models.Int64(20), pluginsdk.UsageCostBasisReported, costStale
	sealContractObservation(t, &item)
	result := reportContractObservation(t, repo, "collector-a", item)
	require.Equal(t, "accepted", result.Status)
	accepted, err := time.Parse(time.RFC3339Nano, result.AcceptedAt)
	require.NoError(t, err)
	discarded := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-z", "session-1"), "discarded-price", "Sonnet", "anthropic", 999, false)
	discarded.Segment, discarded.WorkIdentity, discarded.Coverage = nil, item.WorkIdentity, item.Coverage
	discarded.Quality.Stale = true
	sealContractObservation(t, &discarded)
	discardedResult := reportContractObservation(t, repo, "collector-z", discarded)
	require.Equal(t, "accepted", discardedResult.Status)
	discardedAccepted, err := time.Parse(time.RFC3339Nano, discardedResult.AcceptedAt)
	require.NoError(t, err)
	require.True(t, discardedAccepted.After(accepted), "the unselected stale candidate is newer than the selected price")
	selected := selectContractPartition(t, repo, item.ItemID)
	require.Len(t, selected.Facts, 1)
	fact := selected.Facts[0]
	require.NotNil(t, fact.TokenSource)
	require.NotNil(t, fact.CostSource)
	require.Equal(t, usageSourceNative, fact.TokenSource.Source)
	require.Equal(t, "plugin:collector-a", fact.CostSource.Source)
	m := fact.Measurement
	assert.Equal(t, costStale, m.Stale)
	require.False(t, m.Estimated)
	assert.True(t, accepted.Equal(m.CollectedAt), "collection time comes from the selected price, not the older token or newer discarded source")
	require.Equal(t, "2026-01-02T12:00:00Z", m.ObservedAt.UTC().Format(time.RFC3339))
	require.Equal(t, "2026-01-02T12:00:00Z", m.CoverageStart.UTC().Format(time.RFC3339))
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	report := requireSegmentTotals(t, repo, 100, 20)
	require.NotNil(t, report.LastUpdated)
	assert.True(t, accepted.Equal(*report.LastUpdated))
	assert.Equal(t, costStale, report.Rows[0].Stale)
	rows, _, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, costStale, rows[0].Stale)
	assert.True(t, accepted.Equal(rows[0].CollectedAt))
}
