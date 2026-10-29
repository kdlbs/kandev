package sqlite

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySegmentsUnknownLifetimeCostPreservesKnownDay(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, knownZero := range []bool{false, true} {
			t.Run(fmt.Sprintf("postgres=%t/zero=%t", postgres, knownZero), func(t *testing.T) {
				repo := segmentContractFixture(t, postgres)
				binding := segmentBinding(t, repo, "collector-a", "session-1")
				lifetime := multiModelAdapterObservation(t, binding, "unpriced-lifetime", "Sonnet", "anthropic", 100, false)
				lifetime.Cost = &pluginsdk.UsageCostV1{Basis: pluginsdk.UsageCostBasisUnknown}
				lifetime.Quality.CostCoverageComplete = false
				want := int64(20)
				if knownZero {
					lifetime.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(0), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
					lifetime.Quality.CostCoverageComplete, want = true, 0
				}
				sealContractObservation(t, &lifetime)
				day := multiModelAdapterObservation(t, binding, "known-day", "Sonnet", "anthropic", 20, true)
				day.Segment = lifetime.Segment
				sealContractObservation(t, &day)
				reportSegmentBatch(t, repo, "collector-a", lifetime, day)
				finishFixtureRetention(t, repo)
				completeFixtureProjection(t, repo)
				report := requireSegmentTotals(t, repo, 100, want)
				if !knownZero {
					require.True(t, report.Totals.Partial)
					require.Nil(t, report.Totals.EffectiveCostPerMillion)
				}
			})
		}
	}
}

func TestUsageHistorySegmentsReportedCostDoesNotInheritDiscardedEstimate(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprintf("oversized=%t", oversized), func(t *testing.T) {
			repo := segmentContractFixture(t, false)
			retainSegmentNative(t, repo, 100, func(native *usagehistory.NativeUsage) {
				native.ReportedTotalTokens, native.CostPresent = models.Int64(100), true
				native.CostSource, native.CostBasis, native.CostSubcents, native.Estimated = usageCostSourceEstimated, models.UsageCostBasisEstimated, 10, true
			})
			item := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-a", "session-1"), "reported-event-cost", "Sonnet", "anthropic", 100, false)
			item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr("2026-01-02T12:00:00Z")}
			item.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: "segment-native-event"}
			item.Cost.Amount, item.Cost.Basis = models.Int64(20), pluginsdk.UsageCostBasisReported
			sealContractObservation(t, &item)
			reportSegmentBatch(t, repo, "collector-a", item)
			if oversized {
				var copies []pluginsdk.UsageObservationV1
				for i := range 221 {
					copy := item
					copy.ItemID = fmt.Sprintf("reported-copy-%03d", i)
					sealContractObservation(t, &copy)
					copies = append(copies, copy)
				}
				reportSegmentBatch(t, repo, "collector-a", copies...)
				require.True(t, selectContractPartition(t, repo, item.ItemID).Preparing)
			}
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			report := requireSegmentTotals(t, repo, 100, 20)
			require.Equal(t, models.UsageCostBasisReported, report.Totals.CostBasis)
			require.False(t, report.Totals.Estimated)
			facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			for _, fact := range facts {
				require.False(t, fact.Measurement.Estimated)
				if fact.TokenSource != nil {
					require.Equal(t, "native", fact.TokenSource.Source)
				}
			}
		})
	}
}
