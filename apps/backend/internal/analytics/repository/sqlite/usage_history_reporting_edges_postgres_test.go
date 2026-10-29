package sqlite_test

import (
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryPostgresReportingAmountBoundaries(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "explicit-int64-overflow"
		if mixed {
			name = "distinct-currencies-have-no-monetary-sum"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newProjectionDatabaseFixture(t, true)
			for index := range 2 {
				item := fixture.item
				point := time.Date(2026, 3, 1+index, 1, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
				item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
				amount := int64(math.MaxInt64)
				revision := int64(2)
				if index == 1 {
					item.ItemID, item.SourceRecordID, revision, amount = "second", "second", 1, 1
				}
				if mixed {
					currency := []string{"USD", "EUR"}[index]
					item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(math.MaxInt64), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}
					item.Quality = &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true, CostCoverageComplete: true}
					amount = 1
				}
				projectionReport(t, fixture.first, sealProjectionItem(t, item, revision, amount))
			}
			completeExternalReport(t, fixture)
			report, err := fixture.first.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "day"})
			if !mixed {
				require.ErrorIs(t, err, analytics.ErrUsageAmountOverflow)
				require.Nil(t, report)
				return
			}
			require.NoError(t, err)
			require.Len(t, report.Rows, 2)
			require.Nil(t, report.Totals.CostSubcents)
			require.EqualValues(t, 2, *report.Totals.TotalTokens)
		})
	}
}

func TestUsageHistoryPostgresReportingSourceOwnershipAndUndated(t *testing.T) {
	fixture := newProjectionDatabaseFixture(t, true)
	completeExternalReport(t, fixture)
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	filter := models.SessionUsageFilter{WorkspaceID: "projection-ws", GroupBy: "model", IncludeUndated: true, Start: &start, End: &end}
	report, err := fixture.first.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.True(t, report.Rows[0].Undated)
	require.True(t, report.Totals.Partial)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
	filter.Source = "plugin:collector"
	rows, err := fixture.first.ListSessionUsageMeasurements(t.Context(), filter)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = fixture.database.Exec(`UPDATE workspaces SET org_id='transferred-org' WHERE id='projection-ws'`)
	require.NoError(t, err)
	rows, err = fixture.first.ListSessionUsageMeasurements(t.Context(), filter)
	require.NoError(t, err)
	require.Empty(t, rows, "current ownership cannot reveal former-organization source observations")
}

func completeExternalReport(t *testing.T, fixture projectionDatabaseFixture) {
	t.Helper()
	complete := false
	for range 100 {
		done, err := fixture.first.BackfillUsageHistory(t.Context(), 200)
		require.NoError(t, err)
		if done {
			complete = true
			break
		}
	}
	require.True(t, complete, "all retention watermarks must complete")
	generation, err := fixture.first.StartUsageHistoryRebuild(t.Context(), fixture.scope)
	require.NoError(t, err)
	finishExternalProjection(t, fixture.first, fixture.scope, generation)
}
